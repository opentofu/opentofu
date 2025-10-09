# Local-exec providers

OpenTofu currently assumes that providers are always installed from some other location into an OpenTofu-managed cache directory and executed from there. This brings with it various complexities around version management and cross-platform support.

This document proposes an alternative (opt-in) model where OpenTofu can be instructed to assume that a provider is somehow ready to execute on the system where OpenTofu is running and thus skip installing anything extra for it at all.

## Proposed Solution

This proposal builds on the ["Registry in a File"](https://github.com/opentofu/opentofu/pull/2892) idea, which calls for allowing authors to include a file either inside their root module directory or in any ancestor directory which tells OpenTofu how to interpret provider and module source addresses within the affected configurations.

Specifically, it extends that idea to support a new `local_exec` block type in `providers` blocks:

```hcl
providers "example.com/infrastructure/custom-system" {
  local_exec {
    command = ["docker", "run", "example.com/opentofu/custom-system-provider"]
  }
}
```

The above block declares that whenever an OpenTofu module depends on the provider source adderss `example.com/infrastructure/custom-system` OpenTofu should skip trying to install anything for it during `tofu init`, and then should start the provider by just executing the given command.

In this example the command uses `docker run`, which would then first fetch the container image `example.com/opentofu/custom-system-provider` if it isn't already cached in the local Docker daemon and would then start a new container based on that image.

### User Documentation

As noted above, this proposal depends on ["Registry in a File"](https://github.com/opentofu/opentofu/pull/2892) and builds on the new dependency mapping files discussed there. An author would therefore use this by creating an `.opentofu.deps.hcl` either in the same directory as their root module or in some ancestor directory, with the most likely location being the root of a version control repository containing one or more OpenTofu root modules that are all expected to share the same dependency sources.

The dependency mapping file format uses patterns to systematically map provider source addresses with certain prefixes to installation or direct execution strategies. Therefore in the _general_ form it's possible in principle to map multiple different source addresses beneath a single prefix using a single block and then use reference expressions to pass the wildcarded portions to the program in question, therefore potentially allowing for e.g. a systematic mapping from OpenTofu provider addresses to OCI repository addresses should an organization wish to use multiple different docker-based local-exec providers:

```hcl
providers "example.com/infrastructure/*" {
  local_exec {
    command = ["docker", "run", "example.com/opentofu/${type}-provider"]
  }
}
```

As with `oci_repository` as described in the "Registry in a File" prototype, the available symbols are `hostname`, `namespace`, and `type` corresponding to each of the three provider source address segments respectively. However, in the simple case where a `providers` block has no wildcards at all and therefore matches only one provider source address none of those are needed and the program can instead be configured literally.

The examples so far have only included the required argument `command`, but this block would also support optional arguments `working_dir` and `environment` for overriding the child process's working directory and configuring additional environment variables respectively.

### Technical Approach

OpenTofu already internally separates the provider installation and provider execution needs relatively cleanly, although the current "registry in a file" idea is defined as only a provider installation concern while this idea requires it also being visible to the provider execution code.

Specifically:

- The provider installer would not take any action at all for a `local-exec` provider, under the assumption that whatever command has been specified will have been arranged to work correctly by the time the provider needs to be executed.
- When preparing to execute a provider, instead of dynamically building a command line to execute an executable program under the provider cache directory OpenTofu would just use the command line and other settings directly configured in the `local_exec` block.

    [The plugin executable to run is already specified as a normal `os/exec.Command` object](https://github.com/opentofu/opentofu/blob/5bd9f9d5cbcdf3608f0c2f8cf7991728f00de171/internal/command/meta_providers.go#L366), and so as long as that codepath is able to "see" the configuration in the `local_exec` block it can build a suitable object with the given command arguments, working directory, and environment variables.

(Currently this RFC is just a draft to gauge general interest in this idea, so we'll wait to see what the reaction is before increasing the level of detail in this section.)

### Open Questions

#### Potential security concerns

OpenTofu's traditional provider model uses a dependency lock file and checksums to give some reassurance that the provider packages being used have not been modified since they were originally selected.

The local-exec model shifts that responsibility onto whatever is making the providers available for use locally. In the original motivating example that is Docker CLI, which allows specifying a specific container image checksum to install, which would achieve a similar effect as the dependency lock file. Ultimately though, it's up to the person writing the `local_exec` block to ensure that they are specifying the program to run in a suitable way for the security concerns of the system where OpenTofu will run.

Is that an acceptable tradeoff?

#### Provider version constraints

This proposal introduces for the first time the idea of a provider that OpenTofu should just assume is already somehow available on the system, and thus there's no remote package to install and no meaningful concept of version constraints.

Does that mean that `version` arguments related to this provider should be completely forbidden? That would represent reality most strongly, but would potentially make it hard to substitute a local-exec provider for a traditional provider without modifying existing modules that call it.

Alternatively, OpenTofu could just ignore the version constraints completely and assume that whatever is configured is of a suitable version. That would make it possible for a certain provider source address to be treated in some contexts as a traditional provider and in other contexts as a local-exec provider while still allowing for shared modules that can work in both modes, but may be problematic if the traditional provider makes a breaking change across a major version and then the local-exec provider needs to somehow provider support for both APIs at once.

We could also potentially somehow inform the command of what version constraints were configured for it, letting it then configure itself to match the expectations of the caller. This puts some additional burden on the developer of the provider to perform version constraint parsing and resolution work, but this would only be necessary in the hopefully-rare case of a single program trying to act as multiple different major versions of a provider at once.

### Future Considerations

#### An easier-to-implement provider protocol

This proposal currently focuses only on a different installation and execution model for providers, without proposing any significant change to how OpenTofu interacts with a provider process once it's running.

However, a key assumption behind this proposal is that it would be useful for organizations who want to write custom providers for in-house specialized systems where development and maintenence of a traditional provider would be overkill. We've previously heard complaints about the complexity of the overall protocol stack we use for providers today -- a gRPC API at the lowest-level, typically with various complicated Go libraries layered on top, and no strong story for implementing providers in other programming languages even though it's technically possible -- and so we might choose to compliment this proposal with an alternative provider protocol that's designed to be easier to implement.

[OpenTofu Middleware System](https://github.com/opentofu/opentofu/pull/3016) suggested that JSON-RPC is a more friendly base layer for external plugins, based on its use in [Model Context Protocol](https://modelcontextprotocol.io/). We could potentially offer a JSON-RPC-based alternative provider protocol as an alternative for both local-exec providers _and_ for providers installed by the traditional mechanism, since the question of how OpenTofu interacts with the child process once it's installed and running is largely orthogonal to how it gets installed and launched.

However, our current provider protocol prefers to use [MessagePack](https://msgpack.org/) for encoding dynamically-typed data because it has an extension mechanism we use to represent OpenTofu's concept of "unknown values". There is no JSON serialization of unknown values and the entire JSON infoset is already used to represent valid known values, so if we chose to use JSON we'd likely need to use it with a non-trivial mapping to OpenTofu's type system to make room for the additional concepts that OpenTofu needs to represent. Alternatively, we could seek a compromise of switching to a simpler stdio-based protocol while still using MessagePack, since that serialization format has wide support across many languages itself anyway.

#### "OpenTofu Middleware System"

[OpenTofu Middleware System](https://github.com/opentofu/opentofu/pull/3016) proposed a new kind of plugin-like program for OpenTofu to interact with, completely separate from the concept of providers and using an entirely separate protocol.

If we chose to support local-exec providers along with an easier-to-implement provider protocol as described in the previous section then we might choose to reunify these concepts by making the middleware hooks be just another concept offered by providers, available through both traditional _and_ local-exec providers -- thereby avoiding introducing another special kind of plugin and making the choice of protocol and installation strategy completely orthogonal to the functionality provided by the plugin.

If we did this then we could still offer a middleware-specific library for use in providers that _only_ provide middleware, thereby avoiding the need for users of that library to worry about any of the other provider protocol features. This could therefore still offer a developer experience similar to the commonly-used Model Context Protocol SDKs for simpler cases, while still allowing a more general-purpose provider to offer middleware hooks alongside other provider protocol functionality like resource types and functions. (A similar specialized library could focus only on functions, like [go-tf-func-provider](https://github.com/apparentlymart/go-tf-func-provider) already did for Go under the existing provider protocol.)

## Potential Alternatives

The main alternative is to do nothing at all: our existing provider installation model, recently extended with support for using OCI registries as an installation source, already gives a fair amount of flexibility for how providers can be installed.
