# S3 Backend: Locking by using the recently released feature of conditional writes

Issue: https://github.com/opentofu/opentofu/issues/599

Considering the request from the ticket above, and the newly AWS released S3 feature, we can now have state locking without relying on DynamoDB.

The main reasons for such a change could be summarized as follows:
* Less resources to maintain.
* Potentially reducing costs by eliminating usage of dynamo db.
* One less point of failure (removing DynamoDB from the state management)
* Easy for other S3 compatible services to implement and enable locking

The most important things that need to be handled during this implementation:
* A simple way to enable/disable locking by using S3 conditional writes.
* A path forward to migrate the locking from DynamoDB to S3.
* The default behavior should be untouched as long as the `backend` block configuration contains no attributes related to the new locking option.

## Proposed Solution

Until recently, most of the approaches that could have been taken for this implementation, could have been prone to data races.
But AWS has released new functionality for S3, supporting conditional writes on objects in any S3 bucket.

For more details on the AWS S3 feature and the way it works, you can read more on the [official docs](https://docs.aws.amazon.com/AmazonS3/latest/userguide/conditional-writes.html).

> By using conditional writes, you can add an additional header to your WRITE requests to specify preconditions for your Amazon S3 operation. To conditionally write objects, add the HTTP `If-None-Match` or `If-Match` header.
> 
> The `If-None-Match` header prevents overwrites of existing data by validating that there's not an object with the same key name already in your bucket.
>
> Alternatively, you can add the `If-Match` header to check an object's entity tag (`ETag`) before writing an object. With this header, Amazon S3 compares the provided `ETag` value with the `ETag` value of the object in S3. If the `ETag` values don't match, the operation fails.


To allow this in OpenTofu, the `backend "s3"` will receive one new attribute:
* `use_lockfile` `bool` (Default: `false`) - Flag to indicate if the locking should be performed by using strictly the S3 bucket.

> [!NOTE]
>
> The name `use_lockfile` was selected this way to keep the [feature parity with Terraform](https://developer.hashicorp.com/terraform/language/backend/s3#state-locking).

### User Documentation

In order to make use of this new feature, the users will have to add the attribute in the `backend` block:
```terraform
terraform {
  backend "s3" {
    bucket = "tofu-state-backend"
    key = "statefile"
    region = "us-east-1"
    dynamodb_table = "tofu_locking"
    use_lockfile = true
  }
}
```

* When the new attribute `use_lockfile` exists and `dynamodb_table` is missing, OpenTofu will try to acquire the lock inside the configured S3 bucket.
* When the new attribute `use_lockfile` will exist **alongside** `dynamodb_table`, OpenTofu will:
  * Acquire the lock in the S3 bucket;
  * Acquire the lock in DynamoDB table;
  * Get the digest of the state object from DynamoDB and ensure the state object content integrity;
  * Perform the requested sub-command;
  * Release the lock from the S3 bucket;
  * Release the lock from DynamoDB table.

The usage of [workspaces](https://opentofu.org/docs/language/state/workspaces/) will not impact this new way of locking. The locking object will always be stored right next to its related state object.

> [!NOTE]
>
> OpenTofu [recommends](https://opentofu.org/docs/language/settings/backends/s3/) to have versioning enabled for the S3 buckets used to store state objects.
>
> Acquiring and releasing locks will add a good amount of writes and reads to the bucket. Therefore, for a versioning-enabled S3 bucket, the number of versions for that object could grow significantly.
> Even though the cost should be negligible for the locking objects, any user using this feature could consider configuring the lifecycle of the S3 bucket to limit the number of versions of an object.

> [!WARNING]
> 
> When OpenTofu S3 backend is used with an S3 compatible provider, it needs to be checked that the provider supports conditional writes in the same way AWS S3 is offering.

#### Migration paths
##### I have no locking enabled
In this case, the user can just add the new `use_lockfile=true` and run `tofu init -reconfigure`.

##### I have DynamoDB locking enabled
In case the user has DynamoDB enabled, there are two paths forward:
1. Add the new attribute `use_lockfile=true` and run `tofu init -reconfigure`
   * Later, after a baking period with both locking mechanisms enabled, if no issues encountered, remove the `dynamodb_table` attribute and run `tofu init -reconfigure` again.
   * Letting both locking mechanisms enabled, ensures that nobody will acquire the lock regardless of having or not the latest configuration.
2. Add the new attribute `use_lockfile=true`, remove the `dynamodb_table` one and run `tofu init -reconfigure` 
   * **Caution:** when the configuration updated is executed from multiple places (multiple machines, pipelines on PRs, etc), you might get into issues where one outdated copy of the configuration is using DynamoDB locking and the one updated is using S3 locking. This could create concurrent access on the same state.
   * Once the state is updated by using this approach, the state digest that OpenTofu was storing in DynamoDB (for data consistency checks) will get stale. If it is wanted to go back to DynamoDB locking, the old digest needs to be cleaned up manually.

OpenTofu recommends to have both locking mechanisms enabled for a limited amount of time and later remove the DynamoDB locking. This is to ensure that the changes pushed upstream will be propagated into all of your places that the configuration could be executed from.
### Technical Approach

In order to achieve and ensure a proper state locking via S3 bucket, we want to attempt to create the locking object only when it is missing. 
In order to do so we need to call `s3client.PutObject` with the property `IfNoneMatch: "*"`.
For more information, please check the [official documentation](https://docs.aws.amazon.com/AmazonS3/latest/userguide/conditional-writes.html#conditional-write-key-names).

But the simplified implementation would look like this:
```go
input := &s3.PutObjectInput{
    Bucket:            aws.String(bucket),
    Key:               aws.String(key),
    Body:              bytes.NewReader([]byte(lockInfo)),
    IfNoneMatch:       aws.String("*"),
}
_, err := actor.S3Client.PutObject(ctx, input)
```

The `err` returned above should be handled accordingly with the [behaviour defined](https://docs.aws.amazon.com/AmazonS3/latest/userguide/conditional-writes.html) in the official docs:
* HTTP 200 (OK) - Means that the locking object was not existing. Therefore, the lock can be considered as acquired.
  * For buckets with versioning enabled, if there's no current object version with the same name, or if the current object version is a delete marker, the write operation succeeds.
* HTTP 412 (Precondition Failed) - Means that the locking object is already there. Therefore, the whole process should exit because the lock couldn't be acquired.
* HTTP 409 (Conflict) - Means that the there was a conflict of concurrent requests. AWS recommends to retry the request in such cases, but we could just handle this similarly to the 412 case.

#### Digest updates
> [!NOTE]
> Right now, when locking is enabled on DynamoDB, at the moment of updating the state object content, OpenTofu also writes an entry in DynamoDB with the MD5 sum of the state object.
> The reason is to be able to check the integrity of the state object from the S3 bucket in a future run. This is done by reading the digest from DynamoDB and comparing it with the ETag attribute of the state object from S3. 

By moving to the S3 based locking, OpenTofu will store no other file for the digest of the state object. This was a mechanism to validate the state object integrity when the lock was stored in DynamoDB.
More info about this topic can be found on the [official documentation](https://docs.aws.amazon.com/AmazonS3/latest/userguide/checking-object-integrity.html).

But if both locks are enabled (`use_lockfile=true` and `dynamodb_table=<actual_table_name>`), the state digest will still be stored in DynamoDB.

> [!WARNING]
> 
> By enabling the S3 locking and disabling the DynamoDB one, the digest from DynamoDB will become stale. This means that if it is desired to go back to the DynamoDB locking, the digest needs to be cleaned up or updated in order to allow the content integrity check to work.

### Open Questions

* Do we want to provide the option to have the lock objects into another bucket? This will break the feature parity.

### Future Considerations
Later, DynamoDB based locking might be considered for deprecation once (at least) the following are true:
* Conditional writes are implemented in the majority of the S3 compatible services.
* The adoption rate for the new S3 based locking will be high enough to not affect the existing users.

## Potential Alternatives
Since this new feature relies on the S3 conditional writes, there is hardly other reliable alternative to implement this. 
