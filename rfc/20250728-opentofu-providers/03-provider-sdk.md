# Provider SDK

## Summary

This document proposes a new set of SDKs that enable developers to create OpenTofu providers in multiple programming languages. The SDK abstracts the underlying protocol complexity, providing a CRUD-centric, schema-first development experience that dramatically lowers the barrier to entry for provider development.

> [!NOTE]  
> The SDK design is heavily inspired by the [MCP Server SDKs](https://github.com/modelcontextprotocol/servers), which demonstrate how to create simple, language-idiomatic APIs for protocol-based integrations.

## Design Philosophy

### Core Principles

**1. CRUD-Centric Development**
Developers define simple create, read, update, and delete operations. The SDK should aim to handle the complex mapping to OpenTofu's provider protocol, state management, and resource lifecycle.

**2. Schema-First Approach**
Resource and data source schemas are defined using each language's idiomatic validation libraries (Zod for TypeScript, Pydantic for Python, etc.). The SDK uses these schemas for runtime validation, type safety, and automatic documentation generation. However it may be possible to define another function that can return schema.

**3. Language-Idiomatic Design**
Each SDK follows the conventions and patterns of its target language ecosystem, making provider development feel natural to developers already familiar with that language.

**4. Progressive Disclosure**
Simple use cases require minimal code, while complex scenarios remain possible through advanced SDK features and escape hatches.

**5. Automatic Features**
The SDK automatically should provide managed resources, data sources, documentation generation, capability negotiation, and protocol handling, etc. without requiring developer intervention.

## SDK Architecture

### Transport Abstraction

All SDKs use a transport abstraction that handles the underlying protocol communication. The initial implementation focuses on stdio transport, with future support for other transports:

```typescript
// TypeScript example
import { Provider, StdioTransport } from '@opentofu/provider-sdk';

const provider = new Provider({ name: "custom", version: "1.0.0" });
new StdioTransport().connect(provider);
```

### Protocol Translation Layer

The SDK acts as a translation layer between the developer's simple methods and something that could fulfil the `providers.interface` specification. See [02-provider-client-library.md](./02-provider-client-library.md) for more details.

For example a developer should be able to write the following:
```typescript
provider.resource("s3_bucket", {
  ...
  methods: {
    async create(config) {
      const result = await my_s3_client.createBucket(config);
      return { id: result.id, state: { ...config, id: result.id } };
    }
  }
}
```

And the SDK translates the calls to/from such functions:
- `GetProviderSchema` responses with proper schema definitions
- `PlanResourceChange` handling with unknown value management
- `ApplyResourceChange` execution with error handling
- State persistence and retrieval logic

## Multi-Language Implementation

### TypeScript SDK

> [!NOTE]
> The proposal for typescript may seem more comlpete as it is what I am most familiar with. Other language examples are shown to define the pattern of implementation as an example, and not as something set in stone.

The TypeScript SDK could leverage Zod for schema validation and provides a simple experience of defining items on the provider. For example:

```typescript
import { z } from 'zod';
import { Provider, StdioTransport } from '@opentofu/provider-sdk';

const provider = new Provider({
  name: "my-custom-aws-s3",
  version: "0.1.0",
});

// Schema-first resource definition
const s3BucketSchema = z.object({
  bucket: z.string(),
  region: z.string().default("us-east-1"),
  versioning: z.boolean().default(false),
  tags: z.record(z.string()).optional(),
  // Computed fields
  arn: z.string().optional(),
  id: z.string().optional(),
});

provider.resource("s3_bucket", {
  schema: s3BucketSchema,
  methods: {
    async read(id, config) {
      const bucket = await s3Client.getBucket(id);
      if (!bucket) return null;
      
      return {
        ...config,
        id,
        arn: `arn:aws:s3:::${id}`,
        versioning: bucket.versioningEnabled,
      };
    },
    
    async create(config) {
      const bucketResult = await s3Client.createBucket({
        Bucket: config.bucket,
        Region: config.region,
      });
      
      if (config.versioning) {
        await s3Client.putBucketVersioning({
          Bucket: config.bucket,
          VersioningConfiguration: { Status: 'Enabled' },
        });
      }
      
      return {
        id: config.bucket,
        state: {
          ...config,
          id: config.bucket,
          arn: bucketResult.arn,
        },
      };
    },
    
    async update(id, config) {
      if (config.tags) {
        await s3Client.putBucketTags({
          Bucket: id,
          Tags: config.tags,
        });
      }
      
      return {
        ...config,
        id,
        arn: `arn:aws:s3:::${id}`,
      };
    },
    
    async delete(id) {
      await s3Client.deleteBucket({ Bucket: id });
    },
  },
});

// Data source automatically derived from resource read method, or you can define them yourself explicitly
provider.dataSource("s3_bucket", {
  schema: z.object({
    ...
  }),
  resolve: async (query) => {
    const bucket = await s3Client.getBucket(id);
    if (!bucket) return null;
    
    return {
      ...config,
      id,
      arn: `arn:aws:s3:::${id}`,
      versioning: bucket.versioningEnabled,
    };
  }
});

new StdioTransport()
  .connect(provider)
  .then(() => {
    console.log("AWS S3 Governance Provider ready");
  })
  .catch((error) => {
    console.error(`Failed to start: ${error}`);
    process.exit(1);
  });
```

### Python SDK

The Python SDK could use Pydantic for schema validation and provide both decorator-based and class-based APIs:

```python
from pydantic import BaseModel, Field
from opentofu_provider_sdk import Provider, StdioTransport

provider = Provider(name="custom", version="1.0.0")

class S3BucketSchema(BaseModel):
    bucket: str
    region: str = "us-east-1"
    versioning: bool = False
    tags: dict[str, str] = Field(default_factory=dict)
    # Computed fields
    arn: str | None = None
    id: str | None = None

@provider.resource("s3_bucket", schema=S3BucketSchema)
class S3BucketResource:
    async def read(self, id: str, config: S3BucketSchema) -> S3BucketSchema | None:
        bucket = await s3_client.get_bucket(id)
        if not bucket:
            return None
            
        return S3BucketSchema(
            **config.model_dump(),
            id=id,
            arn=f"arn:aws:s3:::{id}",
            versioning=bucket.versioning_enabled,
        )
    
    async def create(self, config: S3BucketSchema) -> dict:
        await s3_client.create_bucket(
            Bucket=config.bucket,
            Region=config.region,
        )
        
        if config.versioning:
            await s3_client.put_bucket_versioning(
                Bucket=config.bucket,
                VersioningConfiguration={"Status": "Enabled"},
            )
        
        return {
            "id": config.bucket,
            "state": S3BucketSchema(
                **config.model_dump(),
                id=config.bucket,
                arn=f"arn:aws:s3:::{config.bucket}",
            ),
        }
    
    async def update(self, id: str, config: S3BucketSchema) -> S3BucketSchema:
        if config.tags:
            await s3_client.put_bucket_tags(
                Bucket=id,
                Tags=config.tags,
            )
        
        return S3BucketSchema(
            **config.model_dump(),
            id=id,
            arn=f"arn:aws:s3:::{id}",
        )
    
    async def delete(self, id: str) -> None:
        await s3_client.delete_bucket(Bucket=id)

if __name__ == "__main__":
    transport = StdioTransport()
    transport.connect(provider)
```

### Go SDK

The Go SDK provides a familiar interface for Go developers while maintaining the simplified CRUD approach:

```go
package main

import (
    "context"
    "fmt"
    
    "github.com/opentofu/provider-sdk-go"
)

type S3BucketConfig struct {
    Bucket     string            `json:"bucket" validate:"required"`
    Region     string            `json:"region" default:"us-east-1"`
    Versioning bool              `json:"versioning" default:"false"`
    Tags       map[string]string `json:"tags,omitempty"`
    // Computed
    ARN string `json:"arn,omitempty"`
    ID  string `json:"id,omitempty"`
}

func main() {
    provider := sdk.NewProvider("custom", "1.0.0")
    
    provider.Resource("s3_bucket", &sdk.ResourceDefinition{
        Schema: &S3BucketConfig{},
        Methods: &sdk.ResourceMethods{
            ReadFunc: func(ctx context.Context, id string, config interface{}) (interface{}, error) {
                cfg := config.(*S3BucketConfig)
                bucket, err := s3Client.GetBucket(ctx, id)
                if err != nil {
                    return nil, err
                }
                if bucket == nil {
                    return nil, nil
                }
                
                return &S3BucketConfig{
                    Bucket:     cfg.Bucket,
                    Region:     cfg.Region,
                    Versioning: bucket.VersioningEnabled,
                    Tags:       cfg.Tags,
                    ARN:        fmt.Sprintf("arn:aws:s3:::%s", id),
                    ID:         id,
                }, nil
            },
            
            CreateFunc: func(ctx context.Context, config interface{}) (*sdk.CreateResult, error) {
                cfg := config.(*S3BucketConfig)
                
                err := s3Client.CreateBucket(ctx, &s3.CreateBucketInput{
                    Bucket: &cfg.Bucket,
                    Region: &cfg.Region,
                })
                if err != nil {
                    return nil, err
                }
                
                if cfg.Versioning {
                    err = s3Client.PutBucketVersioning(ctx, &s3.PutBucketVersioningInput{
                        Bucket: &cfg.Bucket,
                        VersioningConfiguration: &s3.VersioningConfiguration{
                            Status: "Enabled",
                        },
                    })
                    if err != nil {
                        return nil, err
                    }
                }
                
                return &sdk.CreateResult{
                    ID: cfg.Bucket,
                    State: &S3BucketConfig{
                        Bucket:     cfg.Bucket,
                        Region:     cfg.Region,
                        Versioning: cfg.Versioning,
                        Tags:       cfg.Tags,
                        ARN:        fmt.Sprintf("arn:aws:s3:::%s", cfg.Bucket),
                        ID:         cfg.Bucket,
                    },
                }, nil
            },
            
            UpdateFunc: func(ctx context.Context, id string, config interface{}) (interface{}, error) {
                cfg := config.(*S3BucketConfig)
                
                if len(cfg.Tags) > 0 {
                    err := s3Client.PutBucketTags(ctx, &s3.PutBucketTagsInput{
                        Bucket: &id,
                        Tags:   cfg.Tags,
                    })
                    if err != nil {
                        return nil, err
                    }
                }
                
                return &S3BucketConfig{
                    Bucket:     cfg.Bucket,
                    Region:     cfg.Region,
                    Versioning: cfg.Versioning,
                    Tags:       cfg.Tags,
                    ARN:        fmt.Sprintf("arn:aws:s3:::%s", id),
                    ID:         id,
                }, nil
            },
            
            DeleteFunc: func(ctx context.Context, id string) error {
                return s3Client.DeleteBucket(ctx, &s3.DeleteBucketInput{
                    Bucket: &id,
                })
            },
        },
    })
    
    transport := sdk.NewStdioTransport()
    if err := transport.Connect(provider); err != nil {
        panic(err)
    }
}
```

## Resource Patterns

### Basic Resource Lifecycle

The SDK handles the complete resource lifecycle through four core operations:

**Create**: Provisions a new resource and returns its initial state
```typescript
async create(config) {
  const result = await api.createResource(config);
  return {
    id: result.id,
    state: { ...config, id: result.id, computed_field: result.value }
  };
}
```

**Read**: Refreshes resource state from the remote system
```typescript
async read(id, config) {
  const resource = await api.getResource(id);
  return resource ? { ...config, ...resource } : null;
}
```

**Update**: Modifies an existing resource
```typescript
async update(id, config) {
  await api.updateResource(id, config);
  return { ...config, id, updated_at: new Date().toISOString() };
}
```

**Delete**: Removes the resource
```typescript
async delete(id) {
  await api.deleteResource(id);
  // No return value needed
}
```

### Advanced Resource Patterns

**Conditional Operations**: Handle resources that may not support all operations, either explicitly throw an error or omit the method completely.
```typescript
async update(id, config) {
  if (!api.supportsUpdate()) {
    throw new Error("This resource does not support updates");
  }
  // ... normal update logic
}
```

**Error Handling**: Provide meaningful error messages
```typescript
async create(config) {
  try {
    const result = await api.createResource(config);
    return { id: result.id, state: { ...config, id: result.id } };
  } catch (error) {
    if (error.code === 'ALREADY_EXISTS') {
      throw new Error(`Resource with name '${config.name}' already exists`);
    }
    throw error;
  }
}
```

## Data Sources

### Automatic Data Source Generation

The SDK should be able to automatically generate data sources from resource read methods, this is a change from existing functionality and I am curious what others think heavily here. Authors should be able to define a specific data source method or have it inferred from the resource `read` method.


### Custom Data Sources

For data sources that don't correspond to resources:

```typescript
provider.dataSource("s3_buckets", {
  schema: z.object({
    region: z.string().optional(),
    buckets: z.array(z.object({
      name: z.string(),
      region: z.string(),
      creation_date: z.string(),
    })).optional(),
  }),
  resolve: async (query) => {
    const buckets = await s3Client.listBuckets({
      Region: query.region,
    });
    
    return {
      region: query.region,
      buckets: buckets.map(bucket => ({
        name: bucket.Name,
        region: bucket.Region,
        creation_date: bucket.CreationDate.toISOString(),
      })),
    };
  }
});
```

## Provider-Defined Functions

The SDK supports provider-defined functions for custom computation:

```typescript
provider.function("base64_encode", {
  parameters: [
    { name: "input", type: "string", description: "String to encode" }
  ],
  returnType: "string",
  implementation: async (input: string) => {
    return Buffer.from(input, 'utf8').toString('base64');
  }
});

provider.function("generate_password", {
  parameters: [
    { name: "length", type: "number", description: "Password length" },
    { name: "special_chars", type: "bool", description: "Include special characters", default: true }
  ],
  returnType: "string",
  implementation: async (length: number, specialChars: boolean) => {
    const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789";
    const special = "!@#$%^&*()_+-=[]{}|;:,.<>?";
    const chars = specialChars ? charset + special : charset;
    
    let password = "";
    for (let i = 0; i < length; i++) {
      password += chars.charAt(Math.floor(Math.random() * chars.length));
    }
    return password;
  }
});
```

## Automatic Features

### Documentation Generation

We should attempt to find a way to generate documentation based on what is provided in the source written by the author. Or provide manual documentation to override this. Either way we should ensure that documentation is a first class citizen of this design.

One possible approach is to generate documentation based on the resource objects passed to the server object.

```typescript
// Schema definitions become documentation
const schema = z.object({
  bucket: z.string().describe("The name of the S3 bucket"),
  region: z.string().default("us-east-1").describe("AWS region for the bucket"),
  versioning: z.boolean().default(false).describe("Enable versioning on the bucket"),
});

// Method implementations include examples
provider.resource("s3_bucket", {
  schema,
  examples: [
    {
      title: "Basic S3 bucket",
      config: {
        bucket: "my-app-bucket",
        region: "us-west-2",
      }
    },
    {
      title: "S3 bucket with versioning",
      config: {
        bucket: "my-versioned-bucket",
        versioning: true,
      }
    }
  ],
  methods: { /* ... */ }
});
```

### Capability Negotiation

The SDK automatically handles protocol capability negotiation during initialization, enabling or disabling features based on what the OpenTofu Core version supports.

### Validation and Type Safety

Runtime validation occurs automatically using the defined schemas, providing clear error messages for configuration issues before resources are created or updated.

## Error Handling and Diagnostics

### Error Types

The SDK could provide structured error handling with different error types:

```typescript
import { ProviderError, ResourceError, ValidationError } from '@opentofu/provider-sdk';

async create(config) {
  try {
    // Validation happens automatically via schema
    const result = await api.createResource(config);
    return { id: result.id, state: { ...config, id: result.id } };
  } catch (error) {
    if (error.code === 'INSUFFICIENT_PERMISSIONS') {
      throw new ProviderError(
        'Insufficient permissions to create S3 bucket. Check AWS credentials.',
        { detail: error.message }
      );
    }
    if (error.code === 'BUCKET_ALREADY_EXISTS') {
      throw new ResourceError(
        'Bucket name already exists. S3 bucket names must be globally unique.',
        { attribute: 'bucket' }
      );
    }
    throw error;
  }
}
```

## Implementation Considerations

### SDK Distribution

Each language SDK is distributed through that language's standard package manager:
- **TypeScript/JavaScript**: npm package `@opentofu/provider-sdk`
- **Python**: PyPI package `opentofu-provider-sdk`
- **Go**: Go module `github.com/opentofu/provider-sdk-go`

### Versioning Strategy

SDKs follow semantic versioning with compatibility guarantees:
- Major versions may introduce breaking changes to the developer API
- Minor versions add new features while maintaining backward compatibility
- Patch versions contain bug fixes and performance improvements

The SDK version is independent of the protocol version, allowing SDK improvements without protocol changes.