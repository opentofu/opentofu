# Miscellaneous Configuration Settings in Modules

The current OpenTofu language inherited a top-level block type named `terraform` from its predecessor. Blocks of this type contain an assortment of only-tangentially-related settings that seem to have ended up there just because there wasn't any other obvious place to put them.

This document proposes new alternatives to those settings that are intended to be tool-agnostic, while also making some room for other changes we have already discussed including in future versions of the language.

Relevant issues:
- [opentofu/opentofu#1708](https://github.com/opentofu/opentofu/issues/1708): Needs OpenTofu image version compatible with terraform version 1.7.x
- [opentofu/opentofu#3061](https://github.com/opentofu/opentofu/issues/3061): `required_engine = "opentofu"`

The core observation of this proposal is that the current collection of settings that are supported in `terraform` blocks has no specific theme or rationale for being collected together in this way, and so we can and should reconsider each of those settings in how they relate to each other and to the module they are declared within rather than simply replacing the `terraform` block type directly with some other block type name.

The `terraform` block is currently responsible for:

- Specifying which versions of Terraform _or_ OpenTofu the module is expected to be compatible with, using the optional `required_version` argument.

    Because both Terraform and OpenTofu consume this setting and assume it applies to that software, it currently requires weird workarounds (using `.tofu` files that Terraform cannot "see") to declare both a Terraform version requirement and an OpenTofu version requirement in the same module.

- Declaring which providers the module requires and which versions of each provider the module is expected to be compatible with, using a `required_providers` block.

    This block type actually deals with a number of different concerns all at once:

    - Declaring which providers the module depends on using the (assumed-)global provider source address namespace.
    - Declaring which versions of each provider the module is expected to be compatible with.
    - Declaring a module-specific mapping from "local names" to full provider source addresses, such as declaring that in a particular module `aws` is short for `registry.opentofu.org/opentofu/aws`.
    - Declaring which provider configuration addresses the module expects to have populated by its caller, using the `providers` argument in the calling `module` block, instead of by declaring those providers inline in the module.

- Declaring which variant of the OpenTofu language the module was written for, using the `language` and `experiments` arguments.

    Today's OpenTofu does not tend to make use of either of these, because it has only one rolling language edition and does not use language experiments. In principle though, these arguments allow a particular module to opt in both to new editions of the language that might have slight incompatibilities with older editions, and to opt-in to participating in experimental new language features that are not yet subject to compatibility promises.

- Configuring a "backend" in a root module, using either the `backend` or `cloud` block types.

    For modules used as root modules only, these two mutually-exclusive block types can specify where OpenTofu should store state snapshots or even cause OpenTofu CLI to act only as a local terminal to another execution process running on some remote system.

- Specifying "provider metadata", using the `provider_meta` block.

    This rarely-used feature is useful only for situations where a module has been developed by the same entity as a provider that module uses, and that vendor wants to use the provider as a vehicle for collecting usage metrics for the module by adding additional information to every request made to the given provider related to resources declared in the module. It has no other reasonable purpose.

This proposal will provide at least a high-level direction for the future of each of these, although the details of some are intentionally left to later RFCs which might also change exactly what information we need to collect on each of these topics.

## Language and Runtime Versions

Although it's difficult to find a meaningful link between _all_ of the settings currently configured in `terraform` blocks, what several of them have in common is that they describe the related concerns of which version of the language the module is intending to use and what versions of which runtimes (e.g. OpenTofu and Terraform) the module is expected to be compatible with.

Keeping those settings all declared together in a single place is reasonable because they describe different facets of the same concern and so are likely to change together. Therefore we can group these all together in a new top-level block called `language`, which subsumes what we currently handle with the `required_version`, `language` and `experiments` arguments in `terraform` blocks:

```hcl
# The "language" block type essentially describes what OpenTofu language the
# module author was intending to use. Since it's a "living language" we
# don't explicitly version individual changes to it, so talking about which
# versions of OpenTofu the module is compatible with is the main idea.
language {
  # compatible_with declares which runtime software the module is known to be
  # compatible with, intentionally defined generically so that other software
  # can potentially interpret modules written in the OpenTofu language.
  compatible_with {
    # Only the "opentofu" argument would actually be interpreted by OpenTofu,
    # treating it as a version constraint for OpenTofu CLI versions.
    opentofu = ">= 1.15"

    # Other arguments are allowed in here but are completely ignored by
    # OpenTofu. Other software could potentially define its own argument
    # name for use in this block, and define what values are valid for
    # that argument name.
  }

  # OpenTofu does not currently use language editions, but reserving an argument
  # for specifying them means that if we _do_ later introduce a new one then
  # older OpenTofu versions can potentially return a more useful error message
  # about it, rather than simply complaining about an invalid argument.
  edition = OTF2028

  # Again, OpenTofu does not currently use experiments, but defining the argument
  # means that we can return errors saying that any specified experiment is
  # not available in the current OpenTofu version, rather than returning a
  # generic syntax error.
  experiments = []
}
```

These settings can all potentially affect arbitrary details of how OpenTofu
interprets the rest of a module, so all of these settings are required to be
configured with constant values (no early eval). These settings are effectively
describing characteristics of _the module itself_, rather than the environment
where it is being run, and so we accept this compromise to ensure that we could
potentially vary even the _early evaluation behavior itself_ based on these
settings in future versions of OpenTofu.

Modules that use a `language` block should choose carefully where to place it:

- Placing it in a `versions.tf` file (or any other `.tf` file) means that the
  module will not work in Terraform unless something changes in a future
  Terraform version to make this work.
- For modules that intend to be cross-compatible with Terraform, authors can
  create a `versions.tf` file containing a Terraform-style `terraform` block
  with `required_version`, and a `versions.tofu` file containing a `language`
  block which OpenTofu will then use _in preference to_ the settings in
  the `versions.tf` file.

For any module that contains a `language` block, OpenTofu will completely
ignore any of the corresponding arguments within `terraform` blocks assuming
that they are intended only for Terraform's use. We'd recommend, but not
_require_, that `language` blocks be placed in `.tofu` configuration files.

### The existing `required_version` argument in `terraform` blocks

Due to the history of both projects, unfortunately both OpenTofu and Terraform
make use of the `required_version` argument in a `terraform` block, but
OpenTofu interprets it as an OpenTofu version number while Terraform interprets
it as a Terraform version number.

There is no particular correspondance between those version numbers after
the v1.5 series where the projects diverged, and so for a cross-compatible
module that needs to mention a newer version in its constraint we currently
recommend that authors create both a `.tf` file and a `.tofu` file of the
same basename, and place the Terraform version constraint in the `.tf` file
and the OpenTofu constraint in the `.tofu` file.

This document does not propose to change anything about that behavior, so
that all existing uses of that pattern will retain their current meaning.

Module authors that wish to support OpenTofu versions prior to the introduction
of the `language` block type should continue following the existing pattern.
Authors should adopt a `language` block and nested `compatible_with` block
only once the minimum required OpenTofu version is one that supports that
syntax.

Authors of broadly-shared modules might prefer to delay adopting the new syntax
until all currently-supported OpenTofu minor release series support it, so that
anyone trying to use the module with older versions of OpenTofu will recieve
an error message about an incorrect OpenTofu version, rather than a generic
syntax error about the unsupported `language` block.

## Provider Dependencies

The providers needed for a module are a top-level concern of that module and
so we shall introduce a new _top-level_ `required_providers` block type instead
of having it nested inside any other block type.

This proposal intentionally leaves the contents of this new block unspecified
because there are various other ideas under discussion at the time of writing
that would affect its design if accepted:

- We have discussed returning to a model where each provider configuration is
  able to specify a different version of a provider, rather than requiring
  all instances of a particular provider to agree on a single version, in
  which case the `version` argument would appear in individual `provider`
  blocks instead of in the entries within `required_providers`.
- We have discussed allowing provider instances to be passed around as normal
  values of a special new "provider instance" type, instead of the current
  model where provider configurations pass between modules via a special
  "side-channel", in which case the `configuration_aliases` argument would
  likely not exist in its current form within `required_providers`.
- We have discussed alternative ways to specify providers, such as writing
  a command line to execute directly in the configuration or directly specifying
  that a provider should be installed from a particular physical location
  instead of using the source address indirection. If we choose to do this
  then that suggests quite a different structure for describing which
  provider is "required".

If this proposal is accepted then the next minor version of OpenTofu should
include support for recognizing a top-level `required_providers` block and
generating a specialized error message saying that it's reserved for use in
a future version of OpenTofu, so that introducing fully in a later version
of OpenTofu would cause older versions to return an error message that directly
encourages the reader to investigate whether a module they are trying to use
requires a newer version of OpenTofu.

We will continue to rely on the current form of `required_providers` nested
inside `terraform` until we have made more progress on the other discussions
that might affect its structure, so that we can introduce a new design built
with the future ideas in mind rather than just copying the existing design
and then potentially having to accept awkward compromises to make later features
work with it.

## State Storage Configuration

At the time of writing this proposal we are considering various changes to
how OpenTofu thinks about state storage, including:

- Allowing state storage implementations to be offered as part of an OpenTofu
  provider plugin, rather than having to be built in to OpenTofu CLI.
- Having state storage configured somewhere _outside_ of the root module,
  so that the same root module can be instantiated multiple times with
  completely independent state storages.
- Allowing different modules within the same configuration to have different
  state storage settings, rather than requiring everything to be tracked
  together in a single location.
- Using a more granular storage scheme for state so that it's no longer stored
  as just a single huge snapshot that must always be updated as a unit,
  so that it's possible to work on changes to different parts of the
  configuration in separate plan/apply rounds without one necessarily
  invalidating the other.
- Making "remote operations" be something handled by separate tools, rather
  than built in to OpenTofu.

All of these potentially impose new requirements on the configuration syntax we
use for configuring state storage. Therefore this document does not yet propose
any specific replacement for the current `backend` and `cloud` block types
within `terraform` blocks, except to say that if the new design _does_ include
something similar to the current idea of in-root-module backend configuration
then it should appear as a new top-level block type, not nested inside any other
block type. (It's also possible that a new design would not include any
equivalent of this at all.)

Until those discussions have progressed further and we have a better idea of
what requirements we're trying to design for, OpenTofu authors should continue
using `backend` or `cloud` blocks inside `terraform` blocks, and we will keep
that pattern working in some form in future versions to give authors time
to transition gradually to whatever replaces them.

## "Provider Metadata"

The `provider_meta` block is narrowly focused on the relatively unusual case
where a module is maintained by the same vendor that maintains the main provider
it uses. It is not useful in the more common case where a module is written by
a different party than the providers it uses.

Based on a GitHub Code Search, it appears that the only vendors currently making
_public_ use of this mechanism are:

- Equinix, with [the `equinix/equinix` provider](https://search.opentofu.org/provider/equinix/equinix/latest)
  supporting `module_name` metadata that is used by a number of modules
  published in the `equinix-labs` GitHub organization.
- Google Cloud Platform, with [the `hashicorp/google` provider](https://search.opentofu.org/provider/hashicorp/google/latest)
  and [the `hashicorp/google-beta` provider](https://search.opentofu.org/provider/hashicorp/google-beta/latest)
  both supporting `module_name` metdata that is used by various modules in
  the `GoogleCloudPlatform` GitHub organization, and also in forks of those
  modules.
- HashiCorp Cloud Platform, with [the `hashicorp/hcp` provider](https://search.opentofu.org/provider/hashicorp/hcp/latest)
  supporting `module_name` metadata used by various modules in the `hashicorp`
  GitHub organization.

We do not have any intention of breaking existing uses of this, but it's also
not clear at this time whether this mechanism is a good fit for OpenTofu
in particular and whether it would be supported by future provider protocol
versions at all. Therefore this can continue using `provider_meta` blocks
inside `terraform` blocks primarily for backward-compatibility, and will defer
introducing any new syntax for it for now.

## Technical Approach

The initial limited scope described above can be implemented entirely within
OpenTofu's `package configs`, with no impact on the rest of the system.

No changes to the public API of that package are required. Instead, the new
`language` block type introduces a new way to populate existing fields
of [`configs.Module`](https://github.com/opentofu/opentofu/blob/1e755e9a8f77a723b06e22971d79c4bc2c71eace/internal/configs/module.go#L19-L69):

- The `opentofu` argument in a `compatible_with` block populates the
  `CoreVersionConstraints` field. (All other arguments in this block are
  completely ignored by OpenTofu, so that other tools can use them without
  conflict.)
- The `experiments` argument populates the `ActiveExperiments` field.
- The `edition` argument is treated the same as we currently treat the
  `language` argument in a `terraform` block, which is to check whether it's
  been set to some fixed token we consider to represent the current OpenTofu
  language version and if not to return an error saying this module seems to
  be intended for a different version of OpenTofu.

    Because there is only one acceptable edition to select, the selection is
    not currently exposed anywhere in the public API.

The only other change immediately required for this proposal is to recognize
a top-level `required_providers` block and to immediately return a specialized
error message about it. That can be implemented internally within the
configuration decoding logic and so does not require any public API changes.

## Open Questions

- **Is it okay that the new language-related settings would not be immediately usable for many module authors?**

    Introducing an entirely new syntax for describing language-related settings
    means that older versions of OpenTofu will consider any usage of that syntax
    to be a syntax error, returning an error message that does not clearly
    suggest that the module might be intended for a newer version of OpenTofu.

    This proposal asserts that it's okay to lay the groundwork for a nicer
    syntax in future, even if that means that many authors would continue to use
    the existing syntax for some time until they are ready to require a
    sufficiently-new version of OpenTofu.
    
    The author believes this to be justified because we already have _one_
    cross-compatible solution for presenting different version information to
    OpenTofu vs Terraform -- using `.tf` and `.tofu` files with the same
    basename -- and that will continue to work throughout the transition period
    so that authors of existing modules can make their own decision about when
    to use the new syntax.

- **Are we okay with leaving so many design questions unanswered?**

    This proposal mainly focuses on the general idea of moving away from
    using any block type that's named after a particular product, while leaving
    most of the details of that unspecified with the assumption that future
    RFCs will tackle those questions.

    Would we prefer to wait until the other discussions are further along so
    that we can design this all together as a single unit? Might there be
    "unknown unknowns" that would cause us to design differently even the small
    subset that this proposal initially aims to change?

    We don't have any particular urgency to change _anything_ right now. We
    could choose to wait, if we think the risk outweighs the reward.

- **Should we just ignore language editions and experiments for now?**

    OpenTofu has never made any use of either of these mechanisms; they are
    just ideas we inherited from our predecessor. We could decide to leave
    those existing mechanisms unchanged and not introduce any new syntax
    for them for now, until we have a better idea of whether and how we might
    use them in OpenTofu.

    This proposal includes them primarily because thematically they seem to
    belong to the same category of settings as the OpenTofu runtime version
    constraints and so proposing a new location for all of them together
    seemed wise. However, we could choose to introduce the new `language`
    block type with only the `compatible_with` block type to start and
    then add other arguments later once we know what problems we're trying
    to solve, which would give us more freedom to choose to do something
    significantly different than what's stubbed today.
    
    The only slight advantage of doing something for these immediately is that
    -- as with the existing language features for these concepts -- we can
    introduce real uses of them later knowing that at least some older versions
    of OpenTofu recognize them enough to return a specialized error message
    about them. However, we could achieve that a different way by ensuring
    that the version constraints in `compatible_with` are always checked
    before returning any other errors and then expect that module authors
    using whatever hypothetical language-edition-like or experiment-like
    features we add will _also_ use `compatible_with` to exclude OpenTofu
    versions that do not support the new arguments.

    There is also a potential compromise in supporting the `experiments`
    argument as an alias for the existing argument of the same name but not
    including `edition` at all. There is already existing logic in OpenTofu
    to handle `experiments`, but our only handling of language editions today
    is to return an error if the argument is set to anything other than a
    placeholder token representing an older version of the Terraform language.

- **Should we allow modules to specify that they aren't compatible with OpenTofu _at all_?**

    The current proposal focuses on the situation where a module is written
    to support OpenTofu but wants to declare that it's only intended to work
    with a certain subset of OpenTofu versions.

    It does not include any way to assert that the module is not intended to
    be used with _any_ version of OpenTofu, i.e. that it is intended for use
    only with Terraform or with some other hypothetical future tool that
    replaces OpenTofu while still supporting its module format.

    We could potentially support a special extra value assigned to `opentofu`
    in a `compatible_with` block which represents an empty set of compatible
    versions, whereas the default when unspecified is the maximum set containing
    _all_ possible versions. Is this useful enough to be worth the additional
    complexity that implies?

    (Technically we could add such a thing later as long as earlier versions
    of OpenTofu would reject the new syntax as an error, since it would
    still then have the effect of making the module not work with those older
    versions of OpenTofu, but with a worse error message. If that worse
    error message were acceptable then the author could just set the
    version constraint to any invalid version constraint syntax to get the same
    effect.)

## Future Considerations

At the time of writing the following discussions are ongoing, which this
proposal is intentionally aiming to leave room for without blocking on their
conclusion:

- [Backends as plugins](https://github.com/opentofu/opentofu/issues/382) suggests
  that some or all of the functionality of what we currently call "backends" --
  state storage, at least -- would be implemented in a plugin rather than built
  in to OpenTofu.

    There are various ways that could work and each imposes some different
    requirements on the configuration syntax for configuring them.
- [Discussion of a new provider protocol](https://github.com/opentofu/opentofu/pull/3080)
  includes some ideas for different ways to fetch and execute provider plugins,
  which might impose new design requirements on the `required_providers` block.
- [Scalable Root Modules in OpenTofu](https://github.com/opentofu/opentofu/issues/2860)
  is an umbrella issue for various discussion about ways OpenTofu could better
  support describing and maintaining larger infrastructure estates, which
  includes possibilities of changing state storage and possibly introducing
  an additional concept above "root module" to make root modules themselves
  more reusable.
- [Backward-compatible Additions of new Reference Symbols](https://github.com/opentofu/opentofu/pull/2262)
  discusses a way to _avoid_ introducing a new language edition for certain
  kinds of otherwise-breaking changes, but does not solve everything and
  imagines that language editions might still be involved as a way to more
  clearly aggregate collections of new functionality together once after
  they've had some time to "bake" using more complicated backward-compatibility
  mechanisms.
- ["Stack configuration" files](https://github.com/opentofu/opentofu/pull/2893)
  investigates a very different approach to state storage where it's
  configured outside of the root module.
- ["Registry in a file"](https://github.com/opentofu/opentofu/pull/2892)
  considers a new model where authors are able to get an effect similar to
  running a private module registry but using only a file distributed alongside
  the configurations that would make use of it.

## Potential Alternatives

- We have previously considered simply supporting `tofu` as an alias block type
  name for `terraform`, while changing nothing else.

    That is technically feasible and relatively easy to achieve, but arguably
    repeats the mistake of using a specific product's name as part of the
    language, and would squander the opportunity to revisit the design of
    these nested elements to make room for known future ideas.

- [Tofu Version Compatibility](https://github.com/opentofu/opentofu/pull/1716)
  previously discussed a more constrained change that would, along with
  adopting `tofu` as an alias for `terraform` as in the previous item, also
  change the interpretation of the `required_version` block to assume that
  `required_version` in a `terraform` block is more likely to be talking about
  a Terraform version than an OpenTofu version.

    This is a narrower solution that does not significantly change the existing
    texture of the language. However, that makes it potentially harder to
    explain -- having a language feature of the same name across both Terraform
    and OpenTofu which is nonetheless interpreted subtly differently in each --
    and left unanswered the question of how OpenTofu should react (if at all)
    to Terraform version constraints.

    This _new_ proposal instead leaves the existing language features unchanged,
    "warts and all", and introduces something separate that module authors
    can gradually adopt over time.
