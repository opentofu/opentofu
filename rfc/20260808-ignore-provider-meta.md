# Silently ignore `provider_meta` blocks

The OpenTofu language currently has an infrequently-used feature called `provider_meta` which allows a module author to collaborate with a provider author to covertly collect telemetry information about usage of a module.

For example, modules maintained by the Google Cloud Platform team typically use the `hashicorp/google` or `hashicorp/google-beta` provider to indirectly gather usage statistics for the modules, by including declarations like this:

```hcl
terraform {
  provider_meta "google" {
    module_name = "namespace/name"
  }
}
```

Each time OpenTofu makes a request to the `hashicorp/google` provider that's related to a resource declared in that module, OpenTofu currently sends the content of this block to the provider and then the provider forwards it to the Google Cloud API where it's presumably recorded and used to drive internal analytics (although we cannot actually see what the API server does with this information).

At the time of writing this RFC, we know of the following providers are currently supporting this mechanism to collect information about usage of modules:

- `equinix/equinix`
- `hashicorp/aws`
- `hashicorp/google`
- `hashicorp/google-beta`
- `hashicorp/hcp`

In the earlier RFC [Miscellaneous Configuration Settings in Modules](./20250730-module-misc-settings.md) we discussed whether and how OpenTofu would introduce alternatives to the settings currently specified in `terraform` blocks that we inherited from our predecessor. [The section on "Provider Metadata"](./20250730-module-misc-settings.md#provider-metadata) declined to make any immediate changes for `provider_meta` in particular, ending with the following statement:

> We do not have any intention of breaking existing uses of this, but it's also not clear at this time whether this mechanism is a good fit for OpenTofu in particular and whether it would be supported by future provider protocol versions at all. Therefore this can continue using `provider_meta` blocks inside `terraform` blocks primarily for backward-compatibility, and will defer introducing any new syntax for it for now.

That previous RFC intentionally left this undecided, and so now _this_ RFC is making a more direct statement: this sort of covert telemetry is not something we would have chosen to support had it not already been present in the codebase we forked from, since we typically reject features used primarily or exclusively to allow third-parties (including the OpenTofu project itself) to quietly collect usage information without explicit opt-in.

[Our work on a new implementation of the language runtime](./20251001-eval-plan-apply-architecture.md) has transformed this from a question about whether to just keep some code that is already present into a question about whether to intentionally re-implement this feature in the new runtime. This is therefore a good prompt to make a final decision about the future of `provider_meta` in OpenTofu.

## Proposed Solution

The proposal is for OpenTofu to just silently ignore `provider_meta` blocks, behaving as if they are not present at all.

Since there are existing public modules that already include `provider_meta` blocks, removing support altogether (and therefore generating an error when one is present) would be a breaking change for anyone already using those modules. Silently ignoring the blocks instead means that the provider would get the same requests it would otherwise have seen except that the metadata field in the requests will always be unpopulated.

Accepting this proposal means both that we would skip adding any support for `provider_meta` to the new language runtime _and_ that a future minor version of OpenTofu would begin ignoring `provider_meta` in the traditional runtime too, for consistency. The OpenTofu project would then proceed as if this language feature doesn't exist at all when considering other related requests, unless we explicitly decide to reintroduce support in some way based on new information we learn in future.

If this proposal is accepted, we will also change the OpenTofu language server (tofu-ls) to have the following behavior:

- Code completion inside a `terraform` block will not propose `provider_meta` as an available completion.
- Any `provider_meta` block already in the source code will have a diagnostic reported for it that has the severity "Hint" and has the diagnostic tag "Unnecessary", which editors typically illustrate by using "faded out" text to render the affected code.

    The message of the diagnostic will briefly state that `provider_meta` is ignored by OpenTofu, to provide some additional context for those encountering this behavior for the first time.

## Technical Notes

From a provider protocol perspective, this means that the `provider_meta` field of the protobuf request messages will always describe a null value, such as in [`PlanResourceChange.Request.provider_meta`](https://github.com/opentofu/opentofu/blob/d9edfa1a0362dee72569f52b9c3e01c8332da8fb/docs/plugin-protocol/tfplugin6.9.proto#L547). This will exactly match the content the field would have for a module which lacks any `provider_meta` block for the respective provider, thereby encouraging the provider to handle it in the same way it would for a module that does not attempt to covertly collect usage information.

Since our `opentofu/provider-client` codebase is intended to represent the conceptual provider protocol that OpenTofu uses as opposed to any specific wire protocol (so that we could reimplement its API against other protocols in future), we would also remove the fields related to the `provider_meta` feature from that library and have it just populate a suitable placeholder null value in its implementation of the protocols we inherited from Terraform. Any OpenTofu-specific protocol designs in future would not include this feature at all.

## Risks

We have inferred from the design and current public usage of `provider_meta` that its intended usage is covert collection of module usage information, but of course any provider protocol feature can potentially be used by an inventive provider developer to do something other than what it was intended for.

Therefore accepting this proposal means taking a risk that there might be some future provider that uses `provider_meta` for something other than collection of usage information, and modules running in OpenTofu would not be able to use whatever provider features are driven by that configuration block. Furthermore, because we'd be silently ignoring `provider_meta` blocks instead of returning an error the operator would not get any direct feedback about that part of the configuration being ineffective.

This document proposes that we accept that risk, and minimally account for it by making OpenTofu generate a `WARN`-level internal log line whenever a module contains an explicit `provider_meta` block so that anyone referring to the `TF_LOG=warn` output (including OpenTofu maintainers reviewing a reported issue that included log information) would see a clue that the module and provider together may have been relying on this language feature.

If we learn of a future provider that uses `provider_meta` in a compelling way beyond just covert usage statistics then a future RFC could consider adding an explicit opt-in to allow sending `provider_meta` either in a specific provider or in a specific module, depending on what makes most sense for the feature this would enable. We would nonetheless discourage provider authors from relying on this language and protocol feature at all moving forward, and prefer to use other language and protocol features instead.
