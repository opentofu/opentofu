#!/usr/bin/env node

import {
  MiddlewareServer,
  StdioTransport,
  type OnPlanCompletedParams,
  getResourcesByType,
} from "@opentofu/middleware";

console.error("[BAN-UNVERSIONED-S3] Starting middleware...");

const server = new MiddlewareServer({
  name: "ban-unversioned-s3-buckets",
  version: "0.1.0",
});

server.onPlanCompleted(async (params: OnPlanCompletedParams) => {
  console.error("[BAN-UNVERSIONED-S3] Plan completed, checking for unversioned S3 buckets...");

  const plan = params.plan_json;

  // Get all S3 bucket resources being created or updated
  const allS3Buckets = getResourcesByType(plan, "aws_s3_bucket");
  const allS3BucketVersionResources = getResourcesByType(plan, "aws_s3_bucket_versioning");

  console.error(
    `[BAN-UNVERSIONED-S3] Found ${allS3Buckets.length} S3 buckets and ${allS3BucketVersionResources.length} versioning resources`,
  );

  // Create a map of bucket names/IDs that have versioning
  const versionedBucketIds = new Set<string>();

  // For each versioning resource, extract the bucket it references
  for (const versioningResource of allS3BucketVersionResources) {
    // Check if it's being created or not deleted
    const actions = versioningResource.change?.actions || [];
    if (actions.includes("delete") && !actions.includes("create")) {
      // Versioning is being removed, don't count it
      continue;
    }

    // The bucket reference could be in different places depending on the state
    const bucketRef =
      versioningResource.change?.after?.bucket || versioningResource.change?.before?.bucket;

    if (bucketRef) {
      versionedBucketIds.add(bucketRef);
    }
  }

  // Check each S3 bucket to see if it has versioning
  const unversionedBuckets = allS3Buckets.filter((bucket) => {
    // Skip buckets being deleted
    const actions = bucket.change?.actions || [];
    if (actions.includes("delete") && !actions.includes("create")) {
      return false;
    }

    // Get the bucket name/ID from the planned state
    const bucketName = bucket.change?.after?.bucket || bucket.name;
    const bucketId = bucket.change?.after?.id || bucket.change?.after?.bucket;

    // Check if this bucket has a versioning resource
    // Match by either the bucket attribute value or by resource name pattern
    const hasVersioning =
      versionedBucketIds.has(bucketId) ||
      versionedBucketIds.has(bucketName) ||
      allS3BucketVersionResources.some((v) => v.name === bucket.name);

    if (!hasVersioning) {
      console.error(
        `[BAN-UNVERSIONED-S3] Bucket ${bucket.address} does not have versioning enabled`,
      );
    }

    return !hasVersioning;
  });

  if (unversionedBuckets.length > 0) {
    return {
      status: "fail",
      message: `Found ${unversionedBuckets.length} S3 bucket(s) without versioning enabled`,
      metadata: {
        middleware: "ban-unversioned-s3-buckets",
        timestamp: new Date().toISOString(),
        unversioned_buckets: unversionedBuckets.map((b) => b.address),
      },
    };
  }

  return {
    status: "pass",
    message: "All S3 buckets have versioning enabled",
    metadata: {
      middleware: "ban-unversioned-s3-buckets",
      timestamp: new Date().toISOString(),
      checked_buckets: allS3Buckets.length,
    },
  };
});

new StdioTransport({
  logger: (msg) => console.error(`[TRANSPORT] ${msg}`),
})
  .connect(server)
  .then(() => {
    console.error("[BAN-UNVERSIONED-S3] Middleware running");
  })
  .catch((error) => {
    console.error(`[BAN-UNVERSIONED-S3] Failed to start: ${error}`);
    process.exit(1);
  });
