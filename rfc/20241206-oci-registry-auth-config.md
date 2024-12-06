# OCI Distribution Registry Authentication Configuration

Issue: [#308](https://github.com/opentofu/opentofu/issues/308)

The associated issue represents support for using container image repositories in registries as defined by [The OpenContainers Distribution specification v1.0.0](https://specs.opencontainers.org/distribution-spec/?v=v1.0.0) as an alternative supported installation source location for OpenTofu module and provider packages.

Module and provider installation in OpenTofu have enough differences that we intend to treat most of the relevant details as separate RFCs for each case, but this proposal covers one part of the problem that is cross-cutting: configuring OpenTofu to be able to authenticate to registries that do not permit unauthenticated access.

This particular detail should be shared across all OpenTofu features that will interact with OCI registries now or in the future, because when handling credentials it's highly desirable to store them in only one place to minimize the risk of exposure.

OpenTofu already has some mechanisms for configuring authentication to its own native services -- module registries, provider registries, and remote collaboration systems. OpenTofu's model for remote services is that they are always provided under a hostname and all of the services under one hostname share the same credentials. Therefore OpenTofu's own credentials mechanisms all behave conceptually as a function that takes a hostname and returns credentials.

There are three main options available today:

- `credentials` blocks in the CLI configuration: this is the most direct option, but requires a sensitive token to be stored on disk as part of the configuration:

    ```hcl
    credentials "example.com" {
      token = "abc123example"
    }
    ```

    [The `tofu login` command](https://opentofu.org/docs/cli/commands/login/) automatically creates CLI configuration files using `credentials` blocks unless the user has pre-configured a "credentials helper" as described in the next item.

- [Credentials helpers](https://opentofu.org/docs/internals/credentials-helpers/), configured using `credentials_helper` blocks in the CLI configuration, cause OpenTofu to run an external program whenever credentials are required for a hostname.

    OpenTofu can ask a credentials helper for credentials for a specific hostname by running it with the arguments `get HOSTNAME`, at which point the helper is expected to print to its stdout a JSON object matching the same schema that OpenTofu would accept in a statically-configured `credentials` block. There are also operations for "storing" and "forgetting" credentials, used only by the `tofu login` command.

    This option is included to allow integrating OpenTofu with external credentials manager systems, such as an OS-level "keychain" API, or an external secret store accessed over the network. With credentials helpers the credentials are, from OpenTofu's perspective, retained only temporarily in memory while the `tofu` executable is running and not saved to disk. (Of course, the system that the credentials helper is interacting with might store the credentials on disk somewhere itself, but that's the external system's responsibility.)

- [Environment Variables whose names start with the `TF_TOKEN_` prefix](https://opentofu.org/docs/cli/config/config-file/#environment-variable-credentials): this is similar to a `credentials` block in the CLI configuration, but supports only the `token` argument (which is the only argument OpenTofu supports today anyway) and stores its value directly in the environment variable value.

    This option is primarily for ease of integration with CI systems that expect to provide secrets to their workloads using environment variables.

The OpenContainers ("OCI") ecosystem has its own de-facto conventions for meeting similar needs, and this proposal aims to integrate OpenTofu with those conventions while offering a similar set of options as are available for OpenTofu's own protocols.

## Proposed Solution

OpenTofu will primarily rely on [Docker Credential Helpers](https://github.com/docker/docker-credential-helpers) as the recommended way to provide OCI registry credentials. Despite the reference to Docker, this protocol is also implemented by some other client software in the OCI ecosystem, and there are already various different credential helper programs available for use.

To achieve a better "out of box" experience for anyone who is already using other prominent software in this ecosystem, OpenTofu will also support discovery either credential helpers or statically-configured credentials using a subset of [the Docker CLI configuration language](https://docs.docker.com/reference/cli/docker/#docker-cli-configuration-file-configjson-properties), discovering such files in both the location where `docker login` would write them _and_ in the location where [`podman login`](https://docs.podman.io/en/latest/markdown/podman-login.1.html) would write them by default. The Podman convention in particular has been adopted by some other software in the ecosystem, and so has emerged as a de-facto standard for a vendor-agnostic search scheme.

### User Documentation

OpenTofu users that need to interact with OCI registries that require authentication will have two main options for configuring OpenTofu with access to credentials: explicitly in OpenTofu's own CLI configuration, or implicitly via Docker CLI's and Podman CLI's own configuration files.

#### Explicit OpenTofu CLI Configuration

A new CLI configuration block type `oci_registries` encapsulates all of the cross-cutting settings for OpenTofu CLI's interactions with OCI registries.

```hcl
oci_registries {
  # credential_helper configures a single credential helper that should be
  # used by default for any registry that doesn't have an overridden
  # setting in domain_credential_helpers below.
  credential_helper = "osxkeychain"

  # domain_credential_helpers allows selecting a different credential helper
  # for each registry. The documentation for equivalent features in Docker CLI
  # and Podman CLI uses "registry domain" as the terminology for describing
  # the hostname under which the registry is offered, and so we follow that
  # here.
  domain_credential_helpers = {
    "example.com" = "pass"
  }
}
```

Directly configuring OpenTofu like this makes its behavior independent of how Docker CLI, Podman CLI, etc might be configured on the same system, and is a good option for anyone who is using OCI registries exclusively for OpenTofu packages and doesn't use them with any other tools.

The explicit configuration model only supports _indirect_ provision of credentials using external helper programs. Static configuration of credentials is not supported by this method, to discourage secret sprawl.

#### Implicit Configuration via Docker CLI, Podman CLI, etc

In practice we expect that many teams using OCI registries for OpenTofu packages will choose that option because they are already using such registries for container-image-related purposes, and in that case they are likely to already have other software installed on their system for interacting with OCI registries.

Allowing OpenTofu to infer authentication-related settings from the configuration files read and written by these other programs means that for many users OpenTofu's interactions with their existing registries will "just work" without additional configuration, and users can choose to use `docker login`, `podman login`, or similar to issue themselves credentials for multiple programs (including OpenTofu) at the same time.

There are more details on exactly how this might work under Technical Approach below, but from a user's perspective the behavior they can expect is that any credential helpers configured in Docker/Podman-style configuration files will be automatically discovered and used, and that OpenTofu will also make use of any static credentials that were previously recorded in those files by a command like `docker login`.

OpenTofu considers these other tools' configuration files as a fallback behavior for when there is no explicit configuration. Including an `oci_registries` block in the OpenTofu CLI configuration disables this automatic discovery behavior to ensure that OpenTofu will behave exactly as it was explicitly configured to behave, without potentially-confusing interference from other ambient configuration sources.

### Technical Approach

#### OpenTofu CLI Configuration and `package main`

Although there is considerable existing legacy code not following this pattern, the current intended design for OpenTofu is to follow the [dependency inversion principle](https://en.wikipedia.org/wiki/Dependency_inversion_principle) with `package main` acting as the ultimate arbiter of how different subsystems are configured to work together. The main package in turn uses `package cliconfig` (`internal/command/cliconfig`) to decide most of the locally-user-configurable settings that can affect those dependency resolution decisions.

We will continue that design approach by teaching `package cliconfig` to decode and validate the `oci_registries` block as described in [Explicit OpenTofu CLI Configuration](#explicit-opentofu-cli-configuration), returning the discovered information as part of the overall `cliconfig.Config` object it returns.

The implicit configuration mode acts as an alternative way to populate the same settings from the CLI configuration, and so is also implemented in `package cliconfig` by mapping the concepts from the Docker CLI configuration language (also used by Podman and others) to the same internal data types that we would decode our explicit configuration into, so that the rest of the system does not need to be concerned about how that information was discovered.

`package main` is responsible for using the information returned from the CLI configuration to configure and instantiate the [`ociclient.OCIClient`](https://pkg.go.dev/github.com/opentofu/libregistry@v0.0.0-20241121135917-6f06a9a60bb5/registryprotocols/ociclient#OCIClient) that will then be passed as a dependency into both the provider installer and the module installer, which will then encapsulate all of the OCI registry interactions including the collection and inclusion of credentials when making requests.

The fine details of how we will find and decode Docker CLI-style and Podman CLI-style configuration files are essentially to follow the rules implemented in those codebases as closely as possible, and those are defined as code rather than as specification so are hard to capture as prose here. We have [an experimental initial implementation](https://github.com/opentofu/opentofu/blob/f4c82859864d2ee3397e2f26875cfa73c796c28b/internal/command/cliconfig/oci_registry.go#L137) that illustrates the overall shape of the problem.

#### OCI Client for the Provider Installer

The provider installation components already follow the dependency inversion principle, with `package main` constructing various implementations of [`getproviders.Source`](https://pkg.go.dev/github.com/opentofu/opentofu/internal/getproviders#Source) based on the `provider_installation` block in the CLI configuration, or the implied fallbacks thereof.

The exact details of how OCI registries will be realized as new provider source types will be discussed in a later RFC focused on provider installation, but this proposal assumes that it will involve the addition of at least one new implementation of `getproviders.Source` that will take a preconfigured `ociclient.OCIClient` as a dependency during instantiation, and that the existing logic in `package main` will then be extended to pass the shared OCI registry client to those sources when instantiating them.

#### OCI Client for the Module Package Installer

The module installation mechanisms in OpenTofu are considerably older and have not yet been adapted to follow the dependency inversion principle. Therefore some refactoring of that subsystem will be required to implement this proposal. We can take inspiration from the design of the provider installation process to improve the consistency between these two subsystems.

Currently `package getmodules` (`internal/getmodules`) contains some statically-initialized data structures that effectively act as configuration for the third-party library [`go-getter`](https://pkg.go.dev/github.com/hashicorp/go-getter), which OpenTofu relies on for all module package retrieval.

Those static data structures are exposed to external callers only indirectly through [`getmodules.PackageFetcher`](https://pkg.go.dev/github.com/opentofu/opentofu/internal/getmodules#PackageFetcher), whose instantiation function currently takes no arguments because all of its dependencies are statically configured inside the package.

`getmodules.PackageFetcher` instances are currently instantiated inline within some of the functions of [`package initwd`](https://pkg.go.dev/github.com/opentofu/opentofu/internal/initwd), as an implementation detail. `package initwd` has seen _some_ efforts to adopt a dependency-inversion-style approach, with `initwd.ModuleInstaller` taking the modules directory, config loader, and module registry protocol client as arguments rather than instantiating them directly itself.

To continue that evolution, we will extend `initwd.NewModuleInstaller` to also take a `getmodules.PackageFetcher` as an argument rather than instantiating it directly inline. We can then extend `package command`'s `Meta` type to include a field for a provided `getmodules.PackageFetcher`, alongside [the existing field for a `getproviders.Source`](https://github.com/opentofu/opentofu/blob/ffa43acfcdc4431f139967198faa2dd20a2752ea/internal/command/meta.go#L127-L130).

[`command.Meta.installModules` currently calls `initwd.NewModuleInstaller` directly](https://github.com/opentofu/opentofu/blob/ffa43acfcdc4431f139967198faa2dd20a2752ea/internal/command/meta_config.go#L294), and so we can extend that call to pass in the provided `getmodules.PackageFetcher` alongside the module registry client and other dependency objects.

[`package main` directly instantiates `command.Meta`](https://github.com/opentofu/opentofu/blob/ffa43acfcdc4431f139967198faa2dd20a2752ea/cmd/tofu/commands.go#L89-L115) as its primary way of injecting dependencies into the CLI command layer, including the population of the `ProviderSource` field described above. We can therefore also pass the centrally-instantiated `getmodules.PackageFetcher` in the same way, thus completing the chain of dependency passing all the way from `package main` to the module installer.

The exact details of how OCI registries will be realized as a new module package source will be discussed in a later RFC focused on module package installation, but this proposal assumes that it will involve the addition of new go-getter components within `package getmodules`, and that `getmodules.NewPackageFetcher` will take a preconfigured `ociclient.OCIClient` as an argument and use it to instantiate the new go-getter components as an implementation detail.

### Open Questions

#### Should we allow explicit static credentials?

This proposal currently takes the opinion that the OpenTofu CLI Configuration only deals with OCI registry credentials indirectly through Docker CLI-style credential helpers, and that anyone who wants to use static credentials will do it by configuring them in the Docker CLI or Podman CLI configuration files instead.

This decision avoids the need for us to reimplement something equivalent to `docker login`/`podman login` in OpenTofu and instead delegate to other existing software in the ecosystem for that functionality. It also discourages "secret sprawl" by configuring the same static credentials in multiple places: placing them in the Docker CLI configuration means that they will be available to Docker CLI, Podman CLI, OpenTofu CLI, and various other OCI ecosystem tools.

#### Do we need to support multiple sets of credentials for the same registry?

This proposal currently assumes that it's sufficient to support one credentials method per distinct registry domain name, across all operations. That seems to follow the precedent set by other tools in the ecosystem, such as `docker login` being designed to issue one set of credentials for each distinct server domain name.

If we later add features to OpenTofu that involve _writing_ to OCI registries, is it likely that someone would want to use different credentials for the read-only use-cases of package installation than for use-cases that write to the registry? If so, does any other tooling in the ecosystem have some design precedent we can follow in supporting that?

### Future Considerations

This proposal was strongly motivated by the specific use-cases of fetching provider packages and module packages during the dependency installation step of `tofu init`.

This proposal _aims_ to be a cross-cutting piece of infrastructure that we could use for other features that behave as an OCI Distribution client in future. Some that we've already discussed in other locations are:

- Using an OCI repository to store OpenTofu state snapshots ([#1230](https://github.com/opentofu/opentofu/issues/1230), [#1363](https://github.com/opentofu/opentofu/issues/1363)): If we decide to integrate this into OpenTofu CLI, rather than [Backends as Plugins](https://github.com/opentofu/opentofu/issues/382), then it could benefit from the same credentials configuration.

    (On the other hand, if we decide to require state storage to always be in plugins then a plugin for OCI repositories would be a separate program that would need to solve credentials-gathering for itself some other way.)
- Commands for "pushing" modules and/or providers to a registry (also discussed in [#1672](https://github.com/opentofu/opentofu/issues/1672)): To reduce scope we're likely to focus only on read-only installation use-cases for a first round, but in future we might also offer features for copying module packages from local disk to remote locations, potentially including support for writing to an OCI repository, in which case the OCI-interacting parts of those features should benefit from the same credentials configuration.

## Potential Alternatives

### Support the Docker CLI / Podman CLI configuration files exclusively

Based on an informal survey, it seems relatively common to treat the Docker CLI and Podman CLI configuration files -- which both use a compatible file format but different filesystem search paths -- as the primary location for OCI registry credentials in other tools.

We could potentially decide to follow that lead and not offer any OpenTofu-specific CLI configuration options at all. That would eliminate some features that we need to develop, test, and maintain, but would make it harder for someone to use a separate configuration for OpenTofu than for other tools that interact with OCI registries.

It would not be impossible to vary those, though. For example, setting the `DOCKER_CONFIG` environment variable when running OpenTofu CLI would cause it to look in a different directory for the Docker CLI `config.json` file. This would allow using a separate configuration file, but would require that to be chosen by environment variables rather than by automatically-discovered CLI configuration on disk.

### Encode credentials directly into module source addresses

Since OpenTofu delegates module package fetching entirely to the third-party library go-getter, the existing precedent is that any credentials handling is dealt with in the implementation details of specific "getters" in that library, rather than following the dependency-inversion principle to configure them all from a central source.

In practice that means that existing sources that might require credentials, such as those handled by the "git", "http", and "s3" getters, each solve credentials directly in their own special way, which typically involves a combination of searching for "ambient" credentials in specific environment variables or configuration files, or allowing the credentials to be packed directly into the source address itself.

If we were to apply those similar design choices to fetching module packages from OCI repositories, that would imply having the OCI repository "getter" automatically discovering the Docker CLI / Podman CLI configuration files inline itself, and probably also supporting the inclusion of credentials directly in the source string using a syntax like this:

```hcl
module "example" {
  source = "oci://username:password@example.com/foo/bar/baz"
}
```

The author of this RFC considers this particular historical design decision to have been highly unfortunate: it's highly inappropriate to capture a fixed set of static credentials as part of the source location of an external dependency, and no other language ecosystem manages credentials for external dependencies in this way.

Since our OCI registry support is green-field, we have an opportunity to handle this in a more appropriate manner. The precedent in other language ecosystems is for the package management tool to gather credentials from a separate location to where the dependency location is specified, so that the main source code only describes _what is to be installed_ and not details about how the client ought to prove it is allowed to retrieve those dependencies.

This inline-credentials strategy is also not applicable at all to OpenTofu's provider installation model, since that was designed far later after the unfortunate characteristics of go-getter were already well understood. Gradually evolving the module package installation mechanism to better resemble the provider package installer, and to follow dependency-installation precedent in other language ecosystems more broadly, is the better direction for OpenTofu.
