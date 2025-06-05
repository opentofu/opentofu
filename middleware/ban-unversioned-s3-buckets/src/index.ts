#!/usr/bin/env node

import {
  MiddlewareServer,
  StdioTransport,
  type OnPlanCompletedParams,
  getResourcesByType,
  FileLogger,
} from "@opentofu/middleware";

// Set up logging
const fileLogger = new FileLogger("/tmp/ban-unversioned-s3.log");
const log = (message: string) => {
  console.error(message);
  fileLogger.log(message);
};

log("[BAN-UNVERSIONED-S3] Starting middleware...");

const server = new MiddlewareServer({
  name: "ban-unversioned-s3-buckets",
  version: "0.1.0",
});

server.onPlanCompleted(async (params: OnPlanCompletedParams) => {
  log("[BAN-UNVERSIONED-S3] Plan completed, checking for unversioned S3 buckets...");

  const plan = params.plan_json;

  // Get all S3 bucket resources being created or updated
  const allS3Buckets = getResourcesByType(plan, "aws_s3_bucket");
  const allS3BucketVersioningResources = getResourcesByType(plan, "aws_s3_bucket_versioning");

  // Filter to only buckets being created or updated (ignore deletions)
  const relevantBuckets = allS3Buckets.filter((bucket) => {
    const actions = bucket.change?.actions || [];
    return actions.includes("create") || actions.includes("update");
  });

  log(
    `[BAN-UNVERSIONED-S3] Found ${relevantBuckets.length} S3 buckets being created/updated and ${allS3BucketVersioningResources.length} versioning resources`,
  );

  // Build a set of bucket IDs that have versioning enabled
  const versionedBucketIds = new Set<string>();

  // Process versioning resources that are active (not being deleted)
  const activeVersioningResources = allS3BucketVersioningResources.filter((resource) => {
    const actions = resource.change?.actions || [];
    // Keep if creating, updating, or no-op (existing)
    return !actions.includes("delete") || actions.includes("create");
  });

  // Extract bucket references from versioning resources
  for (const versioningResource of activeVersioningResources) {
    const bucketRef =
      versioningResource.change?.after?.bucket || versioningResource.change?.before?.bucket;

    if (bucketRef) {
      versionedBucketIds.add(bucketRef);
      log(`[BAN-UNVERSIONED-S3] Found versioning for bucket: ${bucketRef}`);
    }
  }

  // Find unversioned buckets
  const unversionedBuckets = relevantBuckets.filter((bucket) => {
    const bucketName = bucket.change?.after?.bucket || bucket.name;
    const bucketId = bucket.change?.after?.id || bucket.change?.after?.bucket;

    // Check multiple ways to match bucket with versioning
    const hasVersioning =
      versionedBucketIds.has(bucketId) ||
      versionedBucketIds.has(bucketName) ||
      // Check if versioning resource has same name suffix
      activeVersioningResources.some((v) => v.name === bucket.name) ||
      // Check if the bucket ID will be referenced (for new buckets)
      activeVersioningResources.some((v) => {
        const ref = v.change?.after?.bucket;
        return ref && (ref.includes(bucket.name) || ref === `\${aws_s3_bucket.${bucket.name}.id}`);
      });

    if (!hasVersioning) {
      log(`[BAN-UNVERSIONED-S3] ❌ Bucket ${bucket.address} does not have versioning enabled`);
    } else {
      log(`[BAN-UNVERSIONED-S3] ✅ Bucket ${bucket.address} has versioning enabled`);
    }

    return !hasVersioning;
  });

  // Return result
  if (unversionedBuckets.length > 0) {
    const message = `Found ${unversionedBuckets.length} S3 bucket(s) without versioning enabled: ${unversionedBuckets.map((b) => b.address).join(", ")}`;

    log(`[BAN-UNVERSIONED-S3] FAIL: ${message}`);

    return {
      status: "fail",
      message,
      metadata: {
        middleware: "ban-unversioned-s3-buckets",
        timestamp: new Date().toISOString(),
        unversioned_buckets: unversionedBuckets.map((b) => ({
          address: b.address,
          name: b.name,
          bucket: b.change?.after?.bucket,
        })),
        total_buckets_checked: relevantBuckets.length,
        versioning_resources_found: activeVersioningResources.length,
      },
    };
  }

  log("[BAN-UNVERSIONED-S3] PASS: All S3 buckets have versioning enabled");

  return {
    status: "pass",
    message: `All ${relevantBuckets.length} S3 bucket(s) have versioning enabled`,
    metadata: {
      middleware: "ban-unversioned-s3-buckets",
      timestamp: new Date().toISOString(),
      buckets_checked: relevantBuckets.length,
      versioning_resources_found: activeVersioningResources.length,
    },
  };
});

// Set up transport and start server
new StdioTransport({
  logger: (msg) => log(`[TRANSPORT] ${msg}`),
})
  .connect(server)
  .then(() => {
    log("[BAN-UNVERSIONED-S3] Middleware running and ready");
  })
  .catch((error) => {
    log(`[BAN-UNVERSIONED-S3] Failed to start: ${error}`);
    process.exit(1);
  });
