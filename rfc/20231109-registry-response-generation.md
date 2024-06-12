# Pre-generating OpenTofu Registry v1 API Responses

Issue: https://github.com/opentofu/opentofu/issues/841

> [!NOTE]  
> This RFC was originally written by @Yantrio and was ported from the old RFC process. It should not be used as a reference for current RFC best practices.

> [!NOTE]
> This proposal is part of the [Homebrew-like Registry Design](https://github.com/opentofu/opentofu/issues/741).

This RFC introduces a strategy for pre-generating API responses for the OpenTofu registry using predefined JSON files and GPG keys as per the folder structure specified in RFC 827. The objective is to create a directory structure that is aligned with the expected responses of the OpenTofu Registry v1 API, which will subsequently be served from an S3-compatible storage solution. This approach is centered around the idea of static generation, which removes the requirement for any computation and allows for the direct exposure of the storage bucket to handle API requests.

## Proposed Solution

The implementation process involves a GitHub Action designed to consolidate information from the JSON files and GPG keys into a prepared format that adheres to the API's response specifications. This transformed data will then be synchronized to the chosen storage backend utilizing rclone or a comparable synchronization tool.

### Benefits

Pre-generating API responses and serving them as static files yields several key benefits:

- **Reduced Latency**: Static files can be delivered with lower latency compared to dynamically generated responses, as there is no server-side processing involved.
- **Cost Efficiency**: The absence of server-side computation diminishes resource utilization, translating to potential cost savings on infrastructure. If we chose to use Cloudflare R2 we also do not encur any data egress costs.
- **Simplicity**: With a static file-based approach, the complexity of the serving infrastructure is significantly decreased.
- **Scalability**: The use of a content delivery network (CDN) becomes more practical with static files, enhancing the scalability and availability of the registry's services.
- **Automation**: Automated generation and synchronization ensure that the OpenTofu registry remains accurate and up-to-date with current data without manual intervention.

### User Documentation

How would a OpenTofu developer interact with and debug this process?

### Technical Approach

#### GitHub Action Workflow

To facilitate the pre-generating of API responses, a GitHub Action will be created using Golang for the OpenTofu registry. This action will be triggered on pushes to the main branch. Upon activation, it will execute a series of steps designed to checkout the relevant repository, generate the JSON files into the OpenTofu v1 API format for both providers and modules, and finally sync the generated output to a preconfigured S3 bucket.
Workflow Steps

The GitHub Action workflow will consist of the following steps:

- **Checkout**: The repository will be checked out. This ensures that the most recent version of the codebase is used for generating the v1 API responses.
- **Generate**: A custom Golang-based script or binary will be executed to transform the JSON files and GPG key information into the directory structure and file format required by the v1 API.
This step will convert the details defined in the JSON files and combine them with the appropriate GPG key data to form the complete API responses expected by clients of the OpenTofu registry.
- **Sync**: The generated files will then be synchronized to an S3 bucket using `rclone`, a command-line program to manage files on cloud storage. This step will be handled by a bash script or a direct invocation of the `rclone` command:

This script will handle the necessary authentication and file transfer operations to ensure that the latest generated data is available in the S3 bucket. By using `rclone` we can ensure that we are only uploading new information and not re-uploading the entire registry again.

Each of these steps will be encapsulated within the GitHub Action workflow file, allowing for automated execution and deployment upon code pushes to the main branch.

#### Generation Application Design

##### Overview

The generation application is a critical component within the OpenTofu registry infrastructure. Its primary function is to process JSON files and GPG keys, and to generate a directory structure that mirrors the OpenTofu v1 API endpoints for providers and modules. This Golang-based application will reside within the registry repository, and it will be built and potentially cached to expedite the generation process during each run.

##### Application Workflow

Upon triggering by a push to the main branch, the application will go through the following flow:

- Processing: The application will read the JSON files and key material to collate the necessary information for pre-generating responses. This includes details about provider versions, module versions, download URLs, SHA sums, and corresponding GPG public keys.

- Generation: The application will create a directory structure that corresponds to the OpenTofu v1 API's URL paths for listing versions.

For example:

- `/v1/providers/{namespace}/{type}/versions` For listing versions
- `/v1/providers/{namespace}/{type}/{version}/download/{os}/{arch}` for downloading specific versions for specific architectures.

##### Example: Null Provider Generation

Taking the example of the "null" provider, the generation application would produce a directory structure similar to this:

```text
.
└── providers
    └── opentofu
        └── terraform-provider-null
            ├── 0.1.0
            │   └── download
            │       ├── darwin
            │       │   └── amd64
            │       ├── linux
            │       │   ├── 386
            │       │   └── amd64
            │       └── windows
            │           ├── 386
            │           └── amd64
            ...
            ├── 3.2.1
            │   └── download
            │       ├── darwin
            │       │   ├── amd64
            │       │   └── arm64
            │       ├── linux
            │       │   ├── 386
            │       │   ├── amd64
            │       │   ├── arm
            │       │   └── arm64
            │       └── windows
            │           ├── 386
            │           ├── amd64
            │           ├── arm
            │           └── arm64
            └── versions
```

Each subdirectory under the version number (e.g., 0.1.0, 3.2.1, etc.) will contain other directories named after OSes and their corresponding architectures. Within these directories, JSON files will contain metadata for downloading the providers, including URLs, SHAs, and signature information.

By matching the directory structure to the v1 api paths, we can expose the bucket directly and serve the filesystem as if it was an API.

> [!NOTE]
> For information about the exact format of the files stored. See the documentation for the [Provider Registry Protocol](https://opentofu.org/docs/internals/provider-registry-protocol/) and the [Module Registry Protocol](https://opentofu.org/docs/internals/module-registry-protocol)

#### Storage and Delivery Options

##### Overview

A crucial aspect of the OpenTofu registry is the choice of storage and delivery solutions to host the pre-generated API responses. The chosen solution must be robust, scalable, and performant, ensuring that users can access the registry data with minimal latency and high availability. This section explores different storage options and their compatibility with various CDN (Content Delivery Network) services, with a focus on their potential use in the OpenTofu infrastructure.

##### S3 + Fastly/Cloudflare/Other

**Pros**:
    - Proven reliability and widespread use in the industry.
    - High configurability and fine-grain control over caching policies.
    - Robust security features and DDoS protection.

**Cons**:
    - Potential for higher costs compared to other CDN solutions.

##### Cloudflare R2

**Pros**:
    - Avoids egress fees typically associated with data transfer out of S3.
    - Integrated with Cloudflare's existing CDN and security services.
    - Provides a potential cost-effective solution for high read volumes.
    - One single product to configure, setup and maintain to satisfy the requirements.
    
**Cons**:
    - Lack of global replication might be an issue for the OpenTofu registry's use case.

### Open Questions

While each of these storage options presents viable possibilities for the OpenTofu registry, careful investigation and testing are required to determine the optimal choice. A key concern to address is how to handle global replication, especially considering the limitation currently evident with Cloudflare R2.

Answered by @cube2222:

Fwiw I did write an email to CloudFlare asking how they'd propose doing multi-region static file hosting with R2. I suppose we'd just rclone to multiple regional buckets and then put a load balancer in front of that.

Alright, it's confirmed that the single-regionality of R2 buckets isn't the common meaning of the word, and those are hosted in a geographically distributed way. The region indicates where the primary requesting users are (and is vague, like "Europe"). Which means we can go ahead and use a plain public R2 bucket for this!

### Future Considerations


## Potential Alternatives

