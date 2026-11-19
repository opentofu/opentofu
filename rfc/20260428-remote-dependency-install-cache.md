# Installation and Caching of Remote Dependencies

This document proposes a new approach to OpenTofu's handling of remote module
packages and provider packages.

This is a broad proposal that potentially interacts with (but does not
necessarily completely solve) at least the following issues:

- [opentofu#1199](https://github.com/opentofu/opentofu/issues/1199): Implement TF_MODULE_CACHE_DIR
- [opentofu#2719](https://github.com/opentofu/opentofu/issues/2719): Use symlinks consistently when "installing" `file:`-schemed modules into the local cache directory
- [opentofu#1086](https://github.com/opentofu/opentofu/issues/1086): Don't make redundant copies of the same module package
- [opentofu#2495](https://github.com/opentofu/opentofu/issues/2495): Module versions should support version constraints for git and more
- [opentofu#1942](https://github.com/opentofu/opentofu/issues/1942): Module version SHA pinning
- [opentofu#1969](https://github.com/opentofu/opentofu/issues/1969): Module version pinning should warn user if a tag has been moved
- [opentofu#3339](https://github.com/opentofu/opentofu/issues/3339): Module Filesystem Mirror

It also includes some related improvements to provider installation and caching,
with the general intention of treating both module and provider dependencies
as similarly as possible, but most of the open feedback has been about module
packages just because the current handling of those is far older and is
constrained by more technical debt.

## User Experience

Overall, the main user-facing changes caused by this proposal are:

- OpenTofu uses central cache directories for all remote module and provider
  packages _by default_, without requiring explicit opt-in in the CLI
  configuration.
- The dependency lock file tracks both module _and_ provider dependency
  selections, including tracking checksums of packages when possible.

The following sections describe those ideas in more detail.

### Remote Package Cache Directories

Today's OpenTofu supports an opt-in global cache directory for provider
packages, and has no option whatsoever for locally caching module packages.
The global provider cache directory is used as a secondary location that
OpenTofu then either copies from or symlinks to when populating a working
directory's own private cache directory.

Under this proposal, OpenTofu would have global cache directories for both
provider and module packages, and OpenTofu would load packages from them
directly at runtime instead of making copies of or symlinks to them.

OpenTofu would choose the locations of these directories automatically based
on platform-specific path conventions. On freedesktop.org-based Unix systems the
selected path would be under the "cache home" directory as defined by the
XDG Base Directory specification, and for other platforms OpenTofu would select
the platform-specific directory that has similar semantics: namely, that it's
excluded from backups by default, and could be cleaned out automatically when
disk space is low, which are both acceptable because OpenTofu expects to be able
to repopulate it from the upstream source if needed.

OpenTofu will append additional directory segments after the platform-specific
cache base directory, following the usual platform naming convention for
separating the cache directories for different applications. For example,
on freedesktop.org-based Unix systems the actual cache base directory would be
`$XDG_CACHE_HOME/opentofu`, followed by `provider-cache` or `module-cache`
depending on the specific cache type. The `-cache` suffix is included because
on some systems the same base directory is used for multiple different XDG
base directory environment variables, and so e.g. `XDG_DATA_HOME` might refer
to the same directory as `XDG_CACHE_HOME`.

### Provider Package Cache Directories

Today's OpenTofu uses a single cache directory shared across all of the
configured provider installation methods, which is straightforward but causes
problems in more complicated setups where a local or network mirror is
intentionally serving different packages than would be served by a provider's
origin registries.

That's defensible when both the cache directory and the installation methods
require explicit configuration because the operator can be responsible for
configuring them in a reasonable way, but having a cache enabled by default
means that we need to make sure that can't lead to misbehavior that would not
be immediately obvious to the operator.

Therefore the automatically-selected provider package cache directory would have
an additional level of heirarchy where the first level splits the cache by
which installation method the package came from:

- For the `direct` installation method, which is the default one that installs
  directly from a provider's origin registry, the first directory is literally
  named `direct`.

    We assume that this installation method always takes the "official" packages
    published by the provider authors and that official packages are immutable
    once published, and so the provider's source address already contains enough
    information to uniquely identify a cached package in this case.
- For a `network_mirror` installation method the first directory is
  named with the fixed prefix `netmirror_` followed by a SHA256 hash of the
  network mirror's base URL.

    This ensures that each distinct network mirror is able to host its own
    versions of provider packages without any cross-pollution between their
    cache directories.
- For an `oci_mirror` installation method the first directory is named with
  the fixed prefix `ocimirror_` followed by a SHA256 hash of the configured
  repository template.

    Again, this ensures that each distinct mirror configuration can host its
    own versions of provider packages without cross-poisoning. Hashing the
    repository template string does unfortunately mean that OpenTofu could
    potentially experience a cache miss even when two different templates
    resolve to the same final OCI repository address, but that should not
    arise often in practice.
- For a `filesystem_mirror` installation method (whether explicitly configured
  or detected implicitly) the first directory is named with the fixed prefix
  `fsmirror_` followed by a SHA256 hash of the absolute base directory.

    As a special case, if the package selected from the mirror is in the
    "unpacked" layout where the package is already extracted into loose files
    in the mirror directory, OpenTofu would not copy it into the provider
    cache directory at all and would instead use the unpacked directory from
    the mirror directly as its own "cache" directory.

Overall then, the idea is to give each configured installation method its own
separate cache directory so that they can potentially offer different packages
for the same provider version without poisoning each other. Beneath these new
top-level directories would be a filesystem tree matching the layout of the
explicitly-configured global cache directory in today's OpenTofu.

The main goal of the separated cache for each method is to allow different
configurations to be used with different provider installation settings, such as
when using the `TF_CLI_CONFIG_FILE` environment variable to locally override the
provider installation strategy for just one configuration on a system. The
content of whichever cache directory is ultimately used to launch the provider
must still match the checksums in the dependency lock file, and so switching an
existing configuration to different installation methods may require running
`tofu init -upgrade` to explicitly switch to expecting a different set of
checksums for the same provider version.

By default OpenTofu would automatically select a suitable cache directory for
each explicitly-configured or implicitly-detected installation method as
described above, but would also allow optionally forcing a different cache
directory as a new setting in the configuration block for each method:

```hcl
provider_installation {
  direct {
    package_cache_dir = "/tmp/example1"
  }
  network_mirror {
    url               = "https://example.com/providers/"
    package_cache_dir = "/tmp/example2"
  }
  # etc...
}
```

Operators may choose to configure the same cache directory for multiple
installation methods if they wish, in which case it's their responsibility
to ensure that all methods that could possibly write into that directory have
agreement about which package represents each distinct provider version so that
they don't poison each other.

Setting the top-level `plugin_cache_dir` argument in the CLI configuration, or
equivalently setting the `TF_PLUGIN_CACHE_DIR` environment variable, activates
the old behavior of sharing a single cache directory across all sources. This is
exactly equivalent to specifying the same path in all of the nested
`package_cache_dir` arguments inside individual installation method blocks. If
both a global and a method-specific cache directory are explicitly configured
then the method-specific cache directory "wins".

If we later adopt a feature similar to
[the "Registry in a File" prototype](https://github.com/opentofu/opentofu/pull/2892)
then any additional configuration-specific provider installation sources
**must not** be allowed to contribute to the shared cache, because they are
inherently specific to the directory containing the dependency specification
file. A later RFC for this kind of feature must specify its own independent
caching system that does not pollute the shared global cache.

### Module Package Cache Directories

Module packages are more challenging to cache globally than provider packages
for a number of annoying historical reasons:

- Module source addresses have not historically been required to refer to
  something immutable.

    For example, `git::https://example.com/foo.git` refers to whatever the
    `HEAD` branch in that Git repository happens to refer to at the time of
    the installation request. If new commits are pushed to that branch later
    then the same request could return different package content.

- Modules have historically been able to modify files in their own source
  directories, and we know that some real-world modules rely on this as
  described later in [Self-modifying Modules](#self-modifying-modules).

    This means that if we just naively started using a shared cache for module
    packages then this kind of module would invalidate the cache directory once
    it's used for the first time.
    
    At the very least we'd need to make sure that this kind of module is more
    likely to fail with an error than to corrupt the cache, such as by making
    the cache directory read-only. We'd probably also need to provide an opt-out
    of caching so that those already relying on this sort of module can easily
    keep it working until they have time to migrate to a better approach.

- Some existing automation relies on the fact that today's OpenTofu caches all
  remote module packages locally under `.terraform/modules` by default, by
  including that directory in a snapshot of the directory after planning to
  send to a different system that will run the apply phase later.

    If we move to using global cache directories exclusively (without also
    copying the package content from the global cache into `.terraform/modules`)
    then that automation would be broken after upgrading to a newer version of
    OpenTofu.

    One specific reason why automation acts in this way is related to the
    previous item about self-modifying modules: a shared module that generates
    a `.zip` archive during its plan phase typically expects that archive
    to still be present on disk during the apply phase. Therefore it might make
    sense for the opt-out described in the previous item to also cause OpenTofu
    to copy the affected modules into `.terraform/modules` as it currently
    does, and then the same workaround would apply to both problems.

- Whereas providers are always identified using consistent provider address
  syntax, module source addresses are considerably more freeform. OpenTofu
  is more likely to encounter cache misses due to there being two different
  ways to refer to effectively the same underlying package.

    We do currently have support for some _basic_ address normalization, it is
    purely syntax-based and can't take into account information from the remote
    system such as which Git commit a particular ref refers to.

    Furthermore, OpenTofu allows module registries to choose between either
    being a direct host of module packages (returning an archive file directly
    using the registry's own credentials) or being just an alias for a physical
    source address such as a commit in a Git repository. Our caching strategy
    must ensure that those two modes can't conflict with one another and cause
    cache poisoning.

- We allow some limited use of input variable values to allow module call source
  addresses and version constraints to vary without modifying the source code.
  In a cache whose key is based on the source address, this would cause a
  separate cache entry to be created each time the source address changes.

    For some uses of non-constant source addresses this is not a huge problem,
    but one of the use-cases folks often talked about when requesting that
    feature was being able to embed specific user credentials directly in a
    Git URL instead of having to configure Git directly, and in a dynamic
    environment where credentials are issued on-demand by a system like OpenBao
    that means that each run would have a different source address and thus
    a different cache key, making the cache ineffective.

- OpenTofu's support for remote sources of module packages is currently based on
  the upstream library [hashicorp/go-getter](https://github.com/hashicorp/go-getter),
  and so we're currently constrained by the operations it supports.

    To achieve a more modern approach to dependency management and caching for
    module packages we may need to move away from this library in favor of
    something we maintain ourselves, so that we can introduce new features
    such as allowing the `git::` source type to transform a given source address
    into an immutable equivalent that's suitable for use as a cache key.

The following proposal currently addresses the above concerns only partially,
because the current draft of this document is mainly focused on finding
consensus about what user experience we want to reach rather than the fine
details of how we'd get there. Once we agree on what the user experience ought
to be for those who are not constrained by these historical quirks, we can then
discuss compromises or workarounds we might need to ensure that there's a way
for existing users to opt out and preserve any historical quirks they've been
relying on until they're ready to adopt the new caching model.

In order to limit the scope of this proposal, this RFC _does not_ propose
immediately introducing support for multiple installation methods for module
packages, such as supporting "mirrors" in the sense that provider installation
uses that term. However, this design _does_ aim to leave room for expanding
it in that way later, by considering the currently-only-available installation
method to be named "direct" by analogy to the `direct` installation method
for providers.

Therefore the default base directory for the module package cache is decided
by appending a `direct` path segment to the base path described earlier, giving
`$XDG_CACHE_HOME/opentofu/module-cache/direct` on platforms that follow the
freedesktop.org specifications and a semantically-equivalent path on other
platforms. If we add support for other installation methods in future then
they can use other sibling directories with a similar naming strategy as
proposed for provider packages above.

Module package addresses don't follow a rigid heirarchical structure as we use
for provider package addresses, so the module package cache structure is based
purely on SHA256 hashes of source addresses. Specifically:

1. Call a source-type-specific function to attempt to translate the
   author-provided source address into an immutable equivalent.

    For example, the `git` source type should translate
    `git::https://example.com/foo.git` by checking what commit the `HEAD` branch
    in that repository currently refers to and then returning a URL with
    the prefix `git::https://example.com/foo.git?ref=` followed by that
    commit.

    For registry-based module addresses, this translation differs depending on
    whether the registry is self-hosting the package or if it's delegating to
    a "go-getter-style" source address:

    1. For direct hosting, we construct a synthetic source address string
       which starts with the fixed prefix `registry::`, followed by the
       `hostname/namespace/name/target-system`-shaped address, then a literal
       `@` followed by the selected version number.

        The `registry::` prefix ensures that this cannot collide with any remote
        source address string as long as we make sure to never use "registry"
        as a supported remote source type in future versions of OpenTofu.

        Note that in this case we're using the logical registry source address
        as the unique identifier, rather than the physical storage location
        it referred to. This means that the module registry is allowed to return
        a different package download URL on future requests as long as the
        package content is unchanged.
    2. For indirection through another remote source address, we run the
       transform for whatever source type the registry selected and store the
       result of that transform. For example, if the registry returns a
       `git::`-prefixed address then we'd use the same Git-specific transform
       described above.

        In this case, if the module registry begins returning a different source
        address in future then that would be stored under a different cache key.
        That's important because unfortunately switching source types may cause
        the checksum of the package directory to change, e.g. because `git::`
        source address installation retains the `.git` directory in the
        installed work tree and modules from Git sources sometimes rely on
        that for techniques such as using the `external` data source from
        `hashicorp/external` to run commands like `git describe` to learn
        which version of a module is being used. If a module registry switches
        to using a different source address type them the hashes may change due
        to these additional source-type-specific directory entries.

    Not all source types will be able to perform this operation. Addresses
    that cannot support this "immutability transform" are ineligible for global
    caching and will always be fetched from their source and cached only locally
    under `.terraform/modules` as in today's OpenTofu. In that case, the rest of
    this section does not apply to those source addresses.

2. Take the SHA256 hash of the immutable form of the package source address, in
   lowercase hexadecimal format.

3. Take the first two hex digits of the hash and concatenate the
   fixed suffix `-s256` to produce the name of the _bucket directory_
   that the package will be placed into. For example this name would be
   `ab-s256` if the first two digits of the hash are "ab".

    (The suffix is to leave room for switching to a different hashing scheme in
    future if SHA256 is found to be flawed, without causing problems for
    those temporarily running both old and new versions of OpenTofu against the
    same cache directory. This sort of "crypto-agility" is important in this
    case because module source addresses are directly controlled by the author
    of a parent module and so could be tailored by an attacker to intentionally
    collide with another commonly-used module. This is less important for our
    other use of SHA256 in the provider cache directory described earlier
    because those inputs are controlled exclusively by the operator in their CLI
    configuration file, rather than being specified in third-party source code.)

4. The final cache directory is `$BASE/<bucketdir>/<hash>`, thereby ensuring
   that the top-level directory has a maximum of 256 entries.
   
   This sort of bucketing is common for hash-based caching strategies like this
   to avoid problems when folks attempt commands like `ls *` or open the cache
   directory in a GUI file explorer that struggles with a large number of
   directory entries. The nested directories could still end up quite large
   over time, but this assumes that bulk operations like listing or deleting
   many cache entries together are more likely to happen at the toplevel, given
   that there's no meaningful relationship between the subdirectories in a
   single bucket directory.

Once a working directory has been initialized with `tofu init`, the remote
packages containing each of the modules the configuration refers to will be
available on local disk either in the global cache directory or in a
subdirectory under `.terraform/modules`. `.terraform/modules` is used both for
source address types that cannot support an "immutability transform" and for
any modules that are subject to the (as-yet-undesigned) optout that would force
OpenTofu to treat some or all dependencies of a particular configuration as
local for backward-compatibility purposes.

When loading the source code for a remote module, OpenTofu would first check
`.terraform/modules` and use a package cached there if present, and would then
fall back to the global cache otherwise. If no cached package is available in
either location, OpenTofu would prompt the user to run `tofu init` to populate
one of these two locations.

The directory representing a remote module package contains a directory
structure that matches the content of the remote package in at least the
following ways (this represents a minimum contract that all source types are
expected to support, but others may preserve more):

- All file and directory names from the package must be preserved exactly.

    In situations where that's impossible -- for example, if the package
    contains a filename which has a character that Windows does not permit and
    we're installing on Windows, or if we're installing to a case-insensitive
    filesystem and there are two filenames that differ only in case -- then
    installation must fail and leave the cache directory completely absent,
    rather than appearing to succeed but leaving an incomplete or mismatching
    directory structure in the cache directory.

- For source package formats that are able to represent the concept of a file
  being "executable" by its owner, when installing a package on an operating
  system which also has that concept (i.e. Unix systems) the executable mode
  must be transferred to files in the cache directory.

    If that's impossible -- for example, if the chmod call fails or if the
    call succeeds but yet the file does not have the executable mode afterwards --
    then installation must fail with an error and leave the cache directory
    completely absent.

    If the source file from the package is _not_ executable then it's okay for
    it to be marked as executable in the cache directory if the cache is on
    a filesystem that just forces all entries to be executable. Likewise, on
    a platform like Windows where "executable" is not a meaningful concept
    OpenTofu will ignore the executable mode in the source package.
    
    This rule is intended to narrowly ensure that executable files from the
    source package will be treated as executable when relevant, but to be
    liberal otherwise. Being able to execute a particular file can be important
    for correct behavior of a module (e.g. using `local-exec` provisioner or
    `hashicorp/external`'s `external` data source) but modules rarely rely on
    a file _not_ being executable.

    No other mode bits other than the "owner executable" flag will necessarily
    be preserved, so module authors should not rely on them. (Note that this
    focus on preserving only the "executable" mode matches Git's strategy, so
    in the common case of using a Git repository as a module package this is
    aligned with what the Git repository is able to represent.)

- Files and directories written into the global cache from a source package
  are always marked as read only when the OS and filesystem can support that,
  to make it more likely that a self-modifying module will fail at runtime
  rather than silently corrupting its shared cache directory. This happens
  regardless of whether "readable" or "read-only" flags are set in the source
  package.

    Packages that contain modules that self-modify can only be used by opting
    out of the global cache, so that there will be a private copy of the module
    package in `.terraform/modules` that can therefore be written into.

If we later adopt a feature similar to
[the "Registry in a File" prototype](https://github.com/opentofu/opentofu/pull/2892)
then any additional configuration-specific module installation sources
**must not** be allowed to contribute to the shared cache, because they are
inherently specific to the directory containing the dependency specification
file. A later RFC for this kind of feature must specify its own independent
caching system that does not pollute the shared global cache.

### Modules in the Dependency Lock File

For provider packages, OpenTofu uses the dependency lock file to "remember"
which version of each provider was selected and what hashes it was supposed to
have, and that's useful in conjunction with a global cache because it means
that OpenTofu only needs to consult a remote registry (or similar) when asked
to "upgrade" the dependencies. If just using a package that was previously
selected then OpenTofu can just immediately use the cache directory if there's
a valid package there that matches the expected hashes.

To complete the story for the module package cache, we'd also introduce tracking
of module package selections in the dependency lock file. At a high level this
serves the same purpose as the tracking of provider packages: to "remember"
exactly which package was most recently selected, so that OpenTofu can use the
local cache when possible instead of going back to the remote source.

Modules have a few notable differences to providers that are relevant to the
dependency lock file, though:

- Module packages can be specified either as direct remote source addresses or
  as registry-style addresses. Registry-style addresses can be used along with
  version constraints to select from one of many available versions of the
  module package. This means the lock file entries need to be able to
  accommodate both styles.
- When installing from a module registry, multiple `module` blocks can refer
  to the same module package but make different version selections. This means
  we need to track selections separately for each `module` block, rather than
  just once per distinct source address.
- As discussed earlier, not all source types are suitable for locking because
  they have no way to describe an immutable package selection. This means that
  certain lock file entries will need to remain "floating", rather than having
  a selection and checksums captured into the lock file.

With all of that in mind, a module entry for an immutability-capable source
type would be shaped like this:

```hcl
module "foo.bar.baz" { # foo.bar.baz is the path to the "module" block this entry applies to

  # "source" is the source address as it was written in the calling module's
  # source code, aside from performing the simple purely-syntax-based
  # normalization we typically apply to those addresses.
  #
  # If this doesn't match the address written in the module source code
  # (normalization notwithstanding) then the operator must run `tofu init` to
  # resynchronize it.
  source = "git::https://example.com/foo.git//subdir"

  # When "source" contains a registry-style address and a `version` argument
  # was also present in the call, `constraints` records the configured version
  # constraints from that argument. As with the equivalent argument in provider
  # lock file entries, this is here primarily just so that it's obvious in a
  # diff of the lock file if the entry was updated in response to changes in
  # the calling module's constraints, rather than just that a new version
  # became available.
  # (This would not be present for a git:: source address like shown above,
  # in practice, because those do not currently allow the `version` argument
  # to be specified at all.)
  constraints = "~> 1.0.0"

  # "package" is the immutable-transformed version of the package address part
  # of the source address, representing the specific package that was selected
  # and so whose checksums appear below and which address to use when consulting
  # the global cache to see if a cached package is already present. Notice that,
  # because this is a package source address rather than a module source address,
  # the "//subdir" portion of the source address has been discarded here.
  #
  # If "source" is a registry-shaped address and the registry chose to host the
  # package itself instead of delegating to another source then this contains
  # the special "registry::"-prefixed string used to identify that registry's
  # result in the cache. That address includes the specific version number that
  # was selected.
  #
  # If "source" is a registry-shaped address and the registry chose to delegate
  # to a non-registry address then this reflects the immutable-transformed
  # version of that address, which will then _not_ have a "registry::" prefix.
  package = "git::https://example.com/foo.git?ref=5632352e8042c13b2da684ad5895525f2aeb0f98"

  # As with "provider" blocks, "hashes" is a list of strings representing
  # different kinds of hash that the package is expected to conform to.
  # Because module packages are not platform-specific, there should initially
  # be just one "h1:"-prefixed checksum in here derived from the content of
  # the package cache directory after the first successful installation. New
  # hashes would appear here only if OpenTofu adopts new hashing schemes in
  # future versions.
  # We don't need "zh:"-schemed hashes here; those are used for providers only
  # because provider developers provide separate signed files containing SHA256
  # hashes, which we then use for cross-platform package verification.
  hashes = [
    # ...
  ]
}
```

In commands like `tofu plan` which expect all packages to already be installed
and want to just load them, these lock file entries contain the information
needed to consult the two cache locations:

1. The `foo.bar.baz` address in the block label is the directory name we'd
   expect to find under `.terraform/modules` for a locally-cached package, which
   we'd prefer to use when it's present.
2. The package address in the `package` argument is the string that we'd take
   a SHA256 hash of in order to determine the path where we'd expect to find
   a globally-cached package.

If neither of those locations contains a valid package that matches at least one
entry in `hashes`, module loading fails and the operator must run `tofu init`
to try to repair the situation. `tofu init` can also follow this lookup process
to determine whether it needs to download a package or if it can just use a
cached package that's already installed.

If a module call uses a source address that cannot support the "immutability
transform" operation to produce a package address we can rely on
not to change, or if it's a registry source address that resolves to a source
address that can't be made immutable, the dependency lock file entry is instead
a "floating" one written like this:

```hcl
module "foo.bar.baz" {
  source   = "https://example.com/package.tar.gz//subdir"
  floating = true
}
```

Creating an explicit entry for "floating" calls serves to make the lack of
global-cachability and hash-checking visible in a diff of the dependency lock
file during code review, and allows commands like `tofu plan` to confirm that
`tofu init` has been run against the current form of the configuration. Since a
"floating" entry doesn't include a `package` argument, these can be resolved
_only_ by looking for `foo.bar.baz` in the `.terraform/modules` directory; no
global cache entry is possible. No hashes are recorded in this case because
we're not expecting the package content to be immutable.

If we include a mechanism to force some or all packages to not be included in
the global cache because they are self-modifying, those packages would always
be recorded as "floating" in the dependency lock file, even if their source
addresses could otherwise be transformed into something immutable.

The `module` blocks in the dependency lock file subsume all of the information
that today's OpenTofu records in `.terraform/modules/modules.json`, and so
OpenTofu would no longer read or write that file. This mirrors how OpenTofu
already uses the dependency lock file to "remember" the results of provider
installation after `tofu init`, without tracking that information redundantly
anywhere else. As in today's OpenTofu, operators can choose for themselves
whether to include the lock file in their version control system (recommended
for consistency between environments) or just to treat it as a temporary local
file created separately for each working directory by `tofu init`.

The addition of module tracking will make the dependency lock file incompatible
with Terraform's dependency lock file format, so at the same time as introducing
this we should switch to using files named `.opentofu.lock.hcl` to avoid
creating something that Terraform would not consider to be valid. `tofu init`
would then check first for `.opentofu.lock.hcl` and fall back to
`.terraform.lock.hcl` only when the preferred name is not present. If
`tofu init` makes any changes to the lock file then it would always write the
new content to `.opentofu.lock.hcl`, regardless of which filename the lock
entries were originally loaded from.

### Cache-cleaning Subcommand

The global provider package and module package cache directories could get
pretty large over time. That's hopefully less of a concern than in today's
OpenTofu where the cache content often gets copied to multiple places throughout
the filesystem, but it'll still eventually add up on systems that have the
cache directory on persistent storage rather than just ephemeral storage.

OpenTofu will therefore get a new `tofu clean` subcommand, which initially
accepts the following options:

- `-provider-cache` causes the provider package cache directory to be completely
  removed.
- `-module-cache` causes the module package cache directory to be completely
  removed.

    This command must first undo the write-protection imposed on the cached
    package directories during installation so that it can remove them without
    encountering spurious permissions-related errors.

At least one of these two options must be provided, or the subcommand will
return usage information and exit with unsuccessful status.

The general name `clean` is intended to allow it to be extended with other
transient-state-cleanup commands in future. For example, if we implement
something like
[Explicit mechanism for working with Temporary Files](https://github.com/opentofu/opentofu/pull/2049)
then this subcommand might then support an additional option for cleaning the
directory where temporary files are written, in cases where that directory is
not automatically cleaned due to an error.

## Implementation Details

(For now this section is intentionally not filled out because early discussion
is focusing only on what user experience we want to enable. The author has
thought about at least one way to achieve all of the high-level goals described
above to make sure they are reasonable to propose, but future drafts of this
document could potentially change those design goals and even if not there are
probably several different ways we could implement them.)

## Open Questions

### Registry Module Package Immutability

This proposal is making the assumption that it's valid to treat packages fetched
through a module registry as immutable if the registry either hosts the packages
itself or delegates to a remote source address that can be transformed into
an immutable package address.

That is a valid assumption for OpenTofu's own registry where we're expecting to
be linking to immutable tags in the underlying GitHub repositories, but this is
not a requirement previously imposed by OpenTofu's module registry client code
and so there might be registries out there which intentionally return "floating"
source addresses that e.g. select whatever is the latest commit on some branch
in a Git repository at the time of installation, rather than selecting a single
commit that's fixed in the source address.

Are we willing to retroactively treat the package indicated by a module registry
response as being immutable even though that wasn't required before? Is it okay
that a module package found through a registry could either be treated as
immutable or floating depending on what shape of source address the registry
chooses to use?

### "Sticky" Module Version Selections

Even in the case of version-constrained module packages taken from a module
registry, today's OpenTofu does not track the selected version in any durable
way and so unless the `version` argument specifies a single exact version it's
possible that `tofu init` (without `-upgrade`) will choose a different version
than was previously selected.

Introducing module tracking to the dependency lock file will make module
packages be treated more like provider packages: for any source type that
provides an "immutability transform" function, `tofu init` will just keep
reinstalling the same version of the module until the operator either runs
`tofu init -upgrade` or manually removes the relevant entry from the dependency
lock file.

Although this new behavior would be desirable for those who _want_ the guarantee
of only getting a new version when they asked for it, others might consider this
to be a regression if they are accustomed to just always getting the latest
version of all of their dependencies.

Do we want to make any additional changes to better support those who want
something more like the old behavior? For anyone who just always wants latest
verisons of _everything_ they could either just always run `tofu init -upgrade`
or avoid adding the dependency lock file to their version control system so
that OpenTofu always thinks it's making fresh selections. But that solution
would not work for someone who, for whatever reason, wants to have the "sticky"
behavior for provider version selections while retaining the non-sticky behavior
for modules.

### Self-modifying Modules

We know that there are some modules that intentionally modify their own source
directory during execution, such as when creating `.zip` archives for AWS Lambda
using the badly-behaved `archive_file` data source from the `hashicorp/archive`
provider that modifies the filesystem even though data sources are expected
not to materially modify the world they are interacting with.

```hcl
data "archive_file" "example" {
  # Note that this specifies a filename under path.module, which is the
  # current module's source directory.
  output_path = "${path.module}/package.zip"
  type        = "zip"

  source {
    # ...
  }
  # ...
}
```

This proposal is assuming that although such modules exist they make up a
relatively small percentage of all module usage, and so we'd be willing to
make them be broken by default under this new scheme as long as we're providing
both a way for users of such modules to opt out of the new treatment (so that
they can use such modules unmodified) _and_ an alternative solution for
temporary files that causes them to be placed somewhere other than the module's
source directory.

Are we willing to accept some friction for those relying on these problematic
modules in order to get better default behavior, as long as we're documenting
a clear way that folks can easily opt out of the new behavior when needed?

(The exact details of how someone would opt out of the new behavior and what
exactly that opt-out would cause to happen remain to be decided after we've
decided if that's something we're even willing to accept at all, but let's
assume while making the decision that the opt-in mechanism requires only
a trivial modification to part of the calling configuration and that, when set,
it would cause at least the affected module to have its own private source
directory that is writable.)

### Using the XDG Cache Home, or equivalent

The proposal currently asserts that we'll place the provider package and module
cache directories by default in some place that the host platform treats as
having cache semantics, so that we have the best possible chance of the cache
being excluded from backups by default, cleaned up automatically when the OS
deems that necessary, etc.

However, there are some other parts of what we've proposed that might not make
that appropriate in practice:

- Provider packages contain executable programs that OpenTofu must be able to
  launch directly. Are there platforms where the cache root is mounted on a
  `noexec` filesystem, which would therefore make the provider plugin fail to
  launch?
- The proposal calls for marking the module package cache directories as
  read-only so that attempting to use a module that expects to modify its source
  directory is more likely to fail rather than silently corrupting the cache
  directory. But could doing that interfere with the operating system's
  automatic cache cleanup behavior?

### Module Source Addresses in Error Messages

In today's OpenTofu, if a diagnostic is raised about part of a module that was
installed from a remote source then the diagnostic message reveals the path
to the local cache directory containing that module:

```
Error: Something went wrong

  with module.foo.module.bar.module.baz.bar.baz,
  on .terraform/modules/foo.bar.baz/main.tf line 2, in resource "foo" "bar":
   2:   thingy = "WRONG!"

...
```

If we switch to also having a global module cache directory without changing
anything else then this these error messages would sometimes include long and
non-human-understandable paths, such as this:

```
Error: Something went wrong

  with module.foo.module.bar.module.baz.bar.baz,
  on /home/username/.cache/opentofu/module-cache/03-s256/0396e1f8343aadf8b250e5a44a107af7162b1fd65fa14a42a9fbc2612d8efce5/main.tf line 2, in resource "foo" "bar":
   2:   thingy = "WRONG!"

...
```

We could potentially take this opportunity to (arguably) improve the UI in both
of these cases by using the information from the dependency lock file to
notice when the source address belongs to a file beneath a directory recognized
as being a cached package for one of the known remote module packages and
render the source location using OpenTofu's normal remote source address syntax
instead of as a local path, like this:

```
Error: Something went wrong

  with module.foo.module.bar.module.baz.bar.baz,
  on git::http://example.com/foo.git//main.tf line 2, in resource "bar" "baz":
   2:   thingy = "WRONG!"

...
```

While this is arguably better for the human-readable UI because it describes
the source location in the same way the author provided it, it'd be potentially
problematic in machine-readable output where software might use it to e.g.
open the affected line in a text editor, where the physical path on local disk
is what we'd actually want.

We could compromise here by changing our JSON representation of diagnostics to
include the source address form as a new property and then making the
human-readable UI code prefer to use that new property when present, but
anything consuming the JSON representation directly would have access to both
forms and any existing code already expecting to find a local filepath would
still get a suitable string.

Do we want to adopt a strategy like this, or is there another approach that
would be better?

(Note that the global cache paths for two distinct source addresses could be
the same if they both have the same immutable package address, and so in some
cases it'd be ambiguous which source address to use in the diagnostic message.
As a heuristic I'd propose that we just take whichever one is shortest and,
if that still doesn't produce a unique result, just take the one that sorts
lexically first. The point is just to show something that the end-user would
hopefully find meaningful to understand where the problematic code came from.)

## Appendices

### "Immutability Transforms" for different source address types

The following are the likely ways that each currently-supported module source
address type would implement their function for turning a given source address
into an immutable equivalent.

- `git::` is made immutable as follows:

    1. Use `git rev-parse` to find the ID of the specific commit that was
       selected.
    2. Return a source address with everything unchanged except that the `ref`
       argument is changed to the commit ID found in the previous step.

    The `depth` query string argument implicitly forces `ref` to be interpreted
    as a branch name rather than as a tag name or commit id due to requirements
    of the Git remote protocol, so unfortunately any address with `depth` set
    to anything other than zero must always be treated as floating. It would not
    be acceptable to just discard the `depth` argument because its presence
    causes different content to be available under `.git/` in the resulting
    package directory, and some git-hosted modules rely on being able to
    interrogate their own Git repository at runtime.

    (The "GitHub" and "BitBucket" source address types are just treated as
    shorthands for `git::`, so this item applies to those too.)

- `hg::` is similar to `git::` but using the Mercurial protocol instead, and
  using `hg identify --id` instead of `git rev-parse` to find the immutable
  string to specify in `ref`.

    The Mercurial "getter" does not have an equivalent of Git's `depth` argument.

- `oci::` is made immutable as follows:

    1. If the source address already includes the `digest` query string
       argument then it's already immutable and is just returned verbatim,
       without continuing to the remaining steps.
    2. Otherwise, use the OCI Distribution protocol to resolve the selected
       tag (which defaults to `latest`) to its associated digest.
    3. Return a source address with everything unchanged except that any `tag`
       argument is removed and `digest` is set to the digest found in the
       previous step.

- `http::` cannot be made immutable in general, but if the address is one
  that would match the
  [Fetching archives over HTTP](https://opentofu.org/docs/language/modules/sources/#fetching-archives-over-http)
  case _and_ a `checksum` query string argument is present then we assume
  the given address is already immutable without any modification.

    Aside from that special case, source addresses of this type are always
    treated as "floating".

- `s3::` cannot be made immutable in general, but if the address contains a
  `checksum` query string argument and/or a `version` query string argument
  then we assume the given address is already immutable without any modification.

    Aside from those special cases, source addresses of this type are always
    treated as "floating".

- `gcs::` is similar to `s3::` except that instead of a `version` query string
  argument the equivalent is for the URL to include a fragment part delimited
  by `#` which is a decimal representation of an integer that is treated as
  the "generation" of the GCS object.

- `file::` (which is used for all non-relative local file paths) is always
  treated as floating.

    Note that go-getter attempts to "install" a local filesystem package by
    creating a symlink to the source directory, and then falling back to deep
    copying if symlinking fails. Therefore the entry in `.terraform/modules`
    is likely to be a symlink rather than a normal directory in this case, and
    so the module loader must tolerate that and follow the symlink.

The step of finding the immutable equivalent of a given source address needs to
happen only in `tofu init` while deciding which cache directory to populate
and what package address to write into the dependency lock file, so it's
acceptable that some of the source types rely on remote information to make that
decision. Other commands like `tofu plan` will instead just trust the immutable
package address already recorded in the dependency lock file as long as
the author-specified source address still matches the one in the dependency lock
file, and will fail with a prompt to run `tofu init` if not.

### In CI systems like GitHub Actions

Those using GitHub Actions, or another similar CI system, may wish to use the
platform's mechanism for saving cache for reuse between jobs.

The cache directory layouts are intended to be compatible with the stock
[`actions/cache`](https://github.com/actions/cache) reusable action, though
workflow authors may wish to set `XDG_CACHE_HOME` to some predictable path
where this action will then read and write.

When operating this mode, workflow authors should:

- Run `tofu init` with `-lockfile=readonly` to ensure that only dependencies
  already recorded in the dependency lock file can be used.

    (Without this, OpenTofu might make implicit changes to the lock file that
    don't match what's in version control, and then the cache key wouldn't match
    between the creation and restoration of the cache blob, making the cache
    completely ineffective.)
- Set the cache key to include a hash of the content of the `.opentofu.lock.hcl`
  file, so that a new cache blob is created each time the dependencies change.
- Include the entire content of `$XDG_CACHE_DIR/opentofu` (with `XDG_CACHE_DIR`
  overridden to somewhere predictable that definitely doesn't overlap with
  any other usage) in the cache blob, so that it'll end up covering everything
  mentioned in the dependency lock file except for "floating" module packages.

In a configuration where all of the module sources can support immutable package
addresses, this should mean that `tofu init` will not need to install anything
at all except on the first run after each change to the selected dependencies,
because all of the needed packages will already be included in the cache.

We could consider offering built-in support for some or all of this mechanism
in our existing
[`opentofu/setup-opentofu`](https://github.com/opentofu/setup-opentofu/) reusable GitHub Action.

The strategy in this section should be adaptable to other CI systems that offer
a similar cache strategy as GitHub Actions does.
