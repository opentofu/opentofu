# Speed up tofu show -json <planfile> by embedding provider schemas in the planfile

Today, `tofu show -json <planfile>` has become one of the primary mechanisms for other applications to interface with a planfile and with the rise of agentic development, we at OpenTofu should aim to improve the time which this command takes to run.

I believe that this command is currently being run a lot in the following situations:

- TACOS automating tofu runs by splitting the plan and the apply phase
- Generic CI/CD users who execute tofu
- Cost estimation tooling (Infracost/etc)
- Tooling around policy engines (OPA/Conftest/Sentinel-style tools)
- Increasingly, AI/LLM based agents

For all of these, `tofu show -json <planfile>` is on the hot path, called once or many times per execution of `tofu plan`.

This slow execution on the hot path hits end users in different ways, but I would like to focus on 2 primary ways with this RFC:

- **Feedback loop latency** - This is the story for agentic development, allowing an agent to run a tight loop of `tofu plan` -> read the json -> reason, infer, fix, edit -> plan again. The factor that matters here is what I will call "Time to actionable output". Launching even a single provider with a large schema taxes every cycle of this loop. Over time this adds up.
- **Compute and time at scale** - Teams that are running `tofu` at a large scale run plans hundreds or even thousands of times a day, and this adds up to a very non-negligible recurring compute cost over time.

The problem today is that rendering a saved planfile as json requires the schema of the resources from the provider and OpenTofu obtains those schemas in an expensive way, by launching every referenced provider plugin and asking each one for its schema over gRPC. It does this even though the plan was built with those exact schemas moments earlier in the plan execution. So every consumer re-derives from scratch a value that is fully determined by the plan. This issue is what this RFC plans to address.

This RFC proposes storing a **minimal subset of provider schemas** that are required to render the planfile as json inside the plan file itself, so that `tofu show` and other tooling built on it can decode and display the plan without requiring to launch providers.

Beyond this being just a speed improvement, this turns the plan file into a self-describing and portable artifact, which means that the consumer of the planfile for analytics may not always need to be the same machine that created it. Note: you will still need to run `tofu show -json <planfile>` with the same version of OpenTofu that created the planfile.

## Background - Why are schemas needed?

A saved plan stores resource attributes as an opaque msgpack value blob. To turn these blobs back into a typed JSON OpenTofu needs to know the `cty.Type` for each resource's block, plus per-resource metadata such as the schema version and sensitivity marks.

To get this `cty.Type` and metadata, OpenTofu needs to spawn the provider that manages the resource and ask for the schema over gRPC. This means spawning a separate process, and in the case of large providers, transferring around megabytes of schema information.

Because of this, **it is slow and it gets slower as you use more providers**. Each distinct provider in the plan means one more plugin to launch, one more gRPC call, one more teardown. Whilst this work is parallelized across providers, the cost of launching these providers is expensive.

As mentioned above too, this **requires every provider to be installed locally**. If a provider is not present then OpenTofu is unable to determine the schema. There is no graceful degradation, it's a hard fail. So if a plan file is copied from a CI runner to a reviewer's laptop, or an agent sandbox, this cannot display the planfile as json at all unless the environment is populated with providers correctly through `tofu init`.

## Proposed Solution

OpenTofu should persist the provider schemas that are needed by the `show` command **inside the planfile**, written at plan-creation time from the schemas already in memory, and have `show` prefer using those over launching providers.

The plan file today is already an archive (zip), I propose that we introduce one new, optional entry alongside the existing entries. This area of the codebase is extensible and already ignores unknown entries in the zip.

Crucially, and this is possibly an extra bit of work on top of this, we only store what is needed to render the plan, NOT the full schema of every provider. This ensures that the planfile does not balloon in size too much, especially for users of providers with a large amount of resources.

### Technical approach

There are 3 parts that we need to address here to introduce this feature into OpenTofu:

- What to store
- How to store
- How to read

#### What to store

During rendering the json, `jsonplan`, `jsonstate` and `jsonconfig` all access parts of the schema that is stored in memory.

From this we can derive the minimal state of what resource schemas need storing in the planfile. For each provider referenced by the plan, we can store a condensed version of the ProviderSchema that only contains:

- The **provider configuration** schema
- **managed resource type schemas and resource identity schemas** for resource types that are referenced in the config snapshot or in the state
- **data source schemas** that are referenced similarly to resources.

We can store this information in a format that is easily comprehendible by the json marshalling logic to reduce the compute overhead of reading this entry in the archive. If the trimming logic and the render logic ever seem to drift (For example, we are missing a resource identity) then the fallback path of the `show` command should be the existing functionality, by questioning the provider.

> [!NOTE]
> Provisioner Schemas today are built into the opentofu binary and are not part of plugins. For this reason we do not need to store these alongside provider schemas.

#### How to store

Provider schemas already have a protobuf representation as defined by the `GetProviderSchema` response in `tfplugin{5|6}`, and the codebase has existing logic to handle conversion in both directions. Reusing this protobuf representation simplifies the implementation of this feature and avoids inventing a new format that could drift over time.

The new proposed zip entry should be a new protobuf message, a map of provider address to the trimmed per-provider schema.

To store this, we should pass the ProviderSchemas through to the logic that writes the planfile (where `planfile.Create` is called should be a good entrypoint). The trimming of the schemas can happen in here and we can re-use the same mechanism of writing to the archive that the other entries in the archive use.

#### How to read

Today, `tofu show <planfile>` obtains schemas via `Meta.MaybeGetSchemas`, which builds a full `tofu.Context` and launches every provider referenced by the plan to ask it for its schema over gRPC.

With this change, the `show` command should first check the opened plan file for the new schemas entry:

1. `planfile.Reader` gains a method to read the new zip entry and decode the protobuf message back into the in-memory schema representation (`tofu.Schemas` / `providers.ProviderSchema`), reusing the existing protobuf-to-internal conversion logic.
2. If the entry is present, the `show` command uses these schemas directly and **skips launching providers entirely**.
3. Before rendering, `show` validates that the stored schema set covers every resource type and data source referenced by the plan. If the entry is absent (a plan file written by an older version) or the validation finds a gap (e.g. the trimming logic and the render logic have drifted), `show` falls back to the existing behaviour of launching the providers from the start.

The fallback keeps this change safe: the stored schemas are strictly an optimization, and any gap in them degrades to today's behaviour rather than to an error.

### Open Questions

- Should we have this as a default enabled feature? - It's nearly free at write time and there's lots of benefits for this, however we should be aware that some people want to keep their planfiles smaller.
  - Yes, this will be mandatory with no user-facing option. Since the schemas are trimmed to only the resource types present in the plan and the zip container is already compressed, we don't expect a significant size increase. During implementation we will benchmark plan file sizes against realistic configurations; if the increase turns out to be significant, that calls into question whether we should do this at all, rather than justifying the complexity of a flag.
- Encryption - Schemas are not sensitive information, but they live inside the encrypted blob like everything else in the planfile, we should check that that's okay.
- Should we do the same for human readable `tofu show`, or only `-json` ? I think it's a freebie but requires some investigation.

### Potential Alternatives

#### Storing schemas in the state snapshot instead

During review it was suggested that the schemas could instead be stored in the state snapshot (which is itself embedded in the planfile), making state a self-describing artifact too. We decided against this:

- State snapshots are retained indefinitely and some state storage backends impose maximum object sizes, so any bloat there accumulates. We have already seen that some users are sensitive to state snapshot size (see [#1593](https://github.com/opentofu/opentofu/issues/1593), which led to switching to compact JSON). A plan file, by contrast, is effectively disposable: it is invalidated as soon as the state snapshot it was created from is superseded.
- The use cases motivating this RFC are all about rendering plans. No concrete use case has been identified that would benefit from self-describing state snapshots.
- When comparing state against the real remote system, OpenTofu needs to launch the providers anyway, so embedded schemas would rarely save a provider launch in state-oriented workflows.

If concrete use cases for self-describing state snapshots emerge later, this can be revisited as a separate proposal.

### Ideas for the future

- It would be nice to have a global persistent provider schema cache on disk instead of embedding it into the planfile. This could be done alongside existing provider caching.
