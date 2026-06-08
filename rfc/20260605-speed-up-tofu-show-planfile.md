# Speed up tofu show -json <planfile> by embedding provider schemas in the planfile

Today, `tofu show -json <planfile>` has become one of the primary mechanisms for other applications to interface with a planfile and with the rise of agentic development, we at OpenTofu should aim to imrpove the time which this command takes to run.

I believe that this command is currently being run a lot in the following situations

- TACOS automating tofu runs by splitting the plan and the apply phase
- Generic CI/CD users who execute tofu
- Cost estimation tooling (Infracost/etc)
- Tooling around policy engines (OPA/Conftest/Sentinel-style tools)
- Increasingly, AI/LLM based agents

For all of these, `tofu show -json <planfile>` is on the hotpath, called once or many times per execution of `tofu plan`.

This slow execution on the hotpath hits end users in different ways, but I would like to focus on 2 primary ways with this RFC:

- **Feedback loop latency** - This is the story for agentic development, allowing an agent to run a tight loop of `tofu plan` -> read the json -> reason, infer, fix, edit -> plan again`. The factor that matters here is what I will call "Time to actionable output". Launching even a single provider with a large schema taxes every cycle of this loop. Over time this adds up.
- **Compute and time at scale** - Teams that are running `tofu` at a large scale run plans hundreds or even thousands of times a day, and this adds up to a very non-negligable recurring compute cost over time.

The problem today is that rendering a saved planfile as json requires the schema of the resources from the provider and OpenTofu obtains those schemas in an expensive way, by launching every referenced provider plugin and asking each one for it's schema over gRPC. It does this even though the plan was built with those exact schemas moments earlier in the plan execution. So every consumer re-derives from scratch a value that is fully determined by the plan. This issue is what this RFC plans to address.

This RFC proposed storing a **minimal subset of provider schemas** that are required to render the planfile as json inside the plan file itsself, so that `tofu show` and other tooling built on it can decode and display the plan without requiring to launch providers.

Beyond this being just a speed improvement, this turns the plan file in to a self-describing and portable artifact. Which means that the consumer of the planfile for analytics may not always need to be the same machine that created it.

## Background - Why are schemas needed?

A saved plan stores resource attributes as an opaque msgpack value blob. To turn this blobs back into a typed JSON OpenTofu needs to know the `cty.Type` for each resource's block, plus per-resource metadata such as the schema version and sensitivity marks.

To get this `cty.Type` and metadata, OpenTofu needs to spawn the provider that manages the resource and ask for the schema over gRPC. This means spawning a seperate process, and in the case of large providers, transferring around megabytes of schema information.

Because of this, **it is slow and it gets slower as you use more providers**. Each distinct provider in the plan means one more plugin to launch, one more gRPC call, one more teardown. Whilst this work is paralellized across pproviders, the cost of launching these providers is expensive.

As mentioned above too, this **requires every provider to be installed locally**. If a provider is not present then OpenTofu is unable to determine the schema. There is no graceful degredation, its a hard fail. So if a plan file is copied from a CI runner to a reviewer's laptop, or an agent sandbox, this cannot display the planfile as json at all unless the environment is populated with providers correctly through `tofu init`.

## Proposed Solution

OpenTofu should persist the provider schemas that are needed by the `show` command **inside the planfile**, written at plan-creation time from the schemas already in memory, and have `show` prefer using those over launching providers.

The plan file today is already an archive (zip), I propose that we introduce one new, optional entry alongside the existing entries. This area of the codebase is extensible and already ignores unknown entries in the zip.

Crucially, and this is possibly an extra bit of work ontop of this, we only store what is needed to render the plan, NOT the full schema of every provider. This ensures that the planfile does not baloon in size too much, especially for users of providers with a large amount of resources.

### Technical approach

There are 3 parts that we need to address here to introduce this feature into OpenTofu:

- What to store
- How to store
- How to read

#### 1. What to store

During rendering the json, `jsonplan`, `jsonstate` and `jsonconfig` all access parts of the schema that is stored in memory.

From this we can derive the minimal state of what resource schmeas need storing in the planfile. For each provider referenced by the plan, we can store a condensed version of the ProviderSchema that only contains

- The **provider configuration** schema
- **managed resource type schemas and resource identity schemas** for resource types that are referenced in the config snapshot or in the state
- **data source schemas** that are referenced similarly to resources.

We can store this information in a format that is easily comprehendable by the json marshalling logic to reduce the compute overhead of reading this entry in the archive. If the trimming logic and the render logic ever seem to drift (For example, we are missing a resource identity) then the fallback path of the `show` command should be the existing functionality, by questining the provider.

> ![NOTE]
> Provisioner Schemas today are built into the opentofu binary and are not part of plugins. For this reason we do not need to store these alongside provider schemas.

#### How to store

Provider schemas already have a protobuf representation as defined by the `GetProviderSchema` response in `tfplugin{5|6}`, and the codebase has existing logic to handle conversion in both directions. Reusing this protobuf representation simplifies the implementation of this featureand avoids inventing a new format that could drift over time.

The new proposed zip entry should be a new protobuf message, a map of provider address to the trimmed per-provider schema.

To store this, we should pass the ProviderSchemas through to the logic that writes the planfile (where `planfile.Create` is called should be a good entrypoint). The trimming of the schemas can happen in here and we can re-use the same mechanism of writing to the archive that the other entries in the archive use.

#### How to read

... TODO

### Open Questions

- Should we have this as a default enabled feature? - It's nearly free at write time and there's lots of benefits for this, however we should be aware that some people want to keep their planfiles smaller.
- Encryption - Schemas are not sensitive information, but they live inside the encrypted blob like everything else in the planfile, we should check that that's okay.
- Should we do the same for human readable `tofu show`, or only `-json` ? I think it's a freebie but requires some investigation.

### Ideas for the future

- It would be nice to have a global persistent provider schema cache on disk instead of embedding it into the provider. This could be done alongside existing providder caching.
