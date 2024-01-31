# State encryption

This document details our intended implementation of the state and plan encryption feature.

## Notes

This document mentions `HCL` as a short-form for OpenTofu code. Unless otherwise specified, everything written also applies to the [JSON-equivalent of the HCL code](https://opentofu.org/docs/language/syntax/json/).

## Goals

The goal of this feature is to allow OpenTofu users to fully encrypt state files when they are stored on the local disk or transferred to a remote backend. The feature should also allow reading from an encrypted remote backend using the `terraform_remote_state` data source. The encrypted version should still be a valid JSON file, but not necessarily a valid state file.

Furthermore, this feature should allow users to encrypt plan files when they are stored. However, plan files are not JSON, so there is no expectation of the resulting plan file being JSON either. **TODO: do we want to specify that we use ZIP encryption semantics?**

For the encryption key, users should be able to specify a key directly, use a remote key provider (such as AWS KMS, etc.), or create derivative keys (such as pbkdf2) from another key source. The primary encryption method should be AES-GCM, but the implementation should be open to different encryption methods. The user should also have the ability to decrypt a state file with one (older) key and then re-encrypt data with a newer key.

To enable use cases where multiple teams need to collaborate, the user should be able to specify separate encryption methods and keys for individual uses, especially for the `terraform_remote_state` data source. However, to simplify configuration, the user should be able to specify single configuration for all remote state data sources.

It is the goal of this feature to let users specify their encryption configuration both in HCL and in environment variables. The latter is necessary to allow users the reuse of HCL code for encrypted and unencrypted state storage.

Finally, it is the goal of the encryption feature to make available a library that third party tooling can use to encrypt and decrypt state. This may be implemented as a package within the opentofu repository, or as a standalone repository.

## Future goals

In the future, the user should be able to specify partial encryption. This encryption type would only encrypt sensitive values instead of the whole state file. **TODO how about the plan file?**

At this time, due to the limitations on passing providers through to modules, encryption configuration is global. However, in the future, the user should be able to create a module that carries along their own encryption method and how it relates to the `terraform_remote_state` data sources. This is important so individual teams can ship ready-to-use modules to other teams that access their state. However, due to the constraints on passing resources to modules this is currently out of scope for this proposal.

Finally, it is a future goal to enable providers to provide their own key providers and encryption methods.

## Non-goals

The primary goal of this feature is to protect state and plan files at rest. It is not the goal of this feature to protect other channels secrets may be accessed through, such as the JSON output. As such, it is not a goal of this feature to encrypt any output on the standard output, or file output that is not a state or plan file.

It is also not a goal of this feature to protect the state file against the operator of the device running `tofu`. The operator already has access to the encryption key and can decrypt the data without the `tofu` binary being present if they so chose.

## User-facing effects

Unless the user explicitly specifies encryption options, no encryption will take place. Other functionality, such as state management CLI functions, JSON output, etc. remain unaffected. Only state and plan files will be affected by the encryption.

Users will be able to specify their encryption configuration both in HCL and via environment variables. Both configurations are equivalent and will be merged at execution time. For more details, see the [environment configuration](#environment-configuration) section below.

When a user wants to enable encryption, they must specify the following block:

```hcl
terraform {
  encryption {
    // Encryption options
  }
}
```

However, the `encryption` block alone does not enable encryption because the user must explicitly specify what key and method to use. The user is responsible for keeping this key safe and generally follow disaster recovery best practices. Therefore, the user must fill out this block with the settings described below.

As a first step, the user must specify one or more key providers. These key providers serve the purpose of creating or providing the encryption key. For example, the user could hard-code a static key named foo:

```hcl
terraform {
  encryption {
    key_provider "static" "foo" {
      key = "this is my encryption key"
    }
  }
}
```

The user then has to specify at least one encryption method referencing the key provider. This encryption method determines how the encryption takes place. It is the user's responsibility to make sure that they provided a key that is suitable for the encryption method.

```hcl
terraform {
  encryption {
    //...
    method "aes_gcm" "bar" {
      key_provider = key_provider.static.foo
    }
  }
}
```

Finally, the user must reference the method for use in their state file, plan file, etc. This enables the encryption for that specific purpose:

```hcl
terraform {
  encryption {
    //...
    statefile {
      method = method.aes_gcm.bar
    }
    planfile {
      method = method.aes_gcm.bar
    }
    backend {
      method = method.aes_gcm.bar
    }
    remote_data_sources {
      default {
        method = method.aes_gcm.bar
      }
      remote_data_source "some_module.remote_data_source.foo" {
        method = method.aes_gcm.bar
      }
    }
  }
}
```

To facilitate key and method rollover, the user can specify a fallback configuration for state-related decryption. When the user specifies a `fallback` block, `tofu` will first attempt to decrypt any state it reads with the primary method and then fall back to the `fallback` method. When `tofu` writes state, it does not use the `fallback` method and always writes with the primary method.

```hcl
terraform {
  encryption {
    //...
    statefile {
      method = method.aes_gcm.bar
      fallback {
        method = method.aes_gcm.baz
      }
    }
  }
}
```

### Composite keys

When a user desires to create a composite key, such as for creating a passphrase-based derivative key or a shared custody key. For these use cases, the user is able to chain those key providers:

```hcl
terraform {
  encryption {
    key_provider "static" "my_passphrase" {
      key = "this is my encryption key"
    }
    key_provider "pbkdf2" "my_derivative_key" {
      key_providers = [key_provider.static.my_passphrase]
    }
    method "aes_gcm" "foo" {
      key_provider = key_provider.pbkdf2.my_derivative_key
    }
    //...
  }
}
```

**Note:** the specific implementation of the derivative key must pay attention to combine the keys securely if it supports multiple key providers as inputs, such as using HMAC to combine keys.

### Environment configuration

As mentioned above, users can configure encryption in environment variables, either as HCL or JSON.

```hcl2
    key_provider "static" "my_key" {
      key = "this is my encryption key"
    }
    method "aes_gcm" "foo" {
      key_provider = key_provider.static.my_key
    }
    statefile {
      method = method.aes_gcm.foo
    }
```

```json
{
  "key_provider" : {
    "static": {
      "my_key": {
        "key": "this is my encryption key"
      }
    }
  },
  "method": {
    "aes_gcm": {
      "foo": {
        "key_provider": "${key_provider.static.my_key}"
      }
    }
  },
  "statefile": {
    "method": "${method.aes_gcm.foo}"
  }
}
```

This data can then be provided to tofu by setting the `TF_ENCRYPTION` environment variable:

```bash
export TF_ENCRYPTION='{"key_provider":{...},"method":{...},"statefile":{...}}'
```

To ensure that the encryption is enforced and state cannot be stored unencrypted, the user can specify the `enforced` option in the HCL configuration:

```hcl
terraform {
  encryption {
    //...
    statefile {
      enforced = true
    }
    planfile {
      enforced = true
    }
    backend {
      enforced = true
    }
    terraform_remote_states {
      default {
        enforced = true
      }
    }
  }
}
```

**Note:** The `enforced` option is also available in the environment configuration and works as intended, but doesn't make much sense because its primary purpose is to guard against environment variable omission.

## Encrypted state format

When `tofu` encrypts a state file, the encrypted state is still a JSON file. Any implementations can distinguish encrypted files by the `encryption` key being present in the JSON structure. However, there may be other keys in the state file depending on the encryption method and type.

## Encrypted plan format

A plan file in OpenTofu is a ZIP file. However, there are no guarantees that the encrypted plan file will also be a valid ZIP file. The implementation is free to choose any format and there is no way to determine if a plan file is encrypted.

**TODO: why not use the ZIP headers to specify this? Could we use ZIP encryption semantics?**

## Implementation aspect

When implementing the encryption tooling, the implementation should be split in two parts: the library and the OpenTofu implementation. The library should rely on the cty types and hcl as a means to specify schema, but should not be otherwise tied to the OpenTofu codebase. This is necessary to enable encryption capabilities for third party tooling that may need to work with state and plan files.

### Library implementation

The encryption library should create an iinterface that other projects and OpenTofu itself can use to encrypt and decrypt state. The library should express its schema needs (e.g. for key provider config) using [cty](https://github.com/zclconf/go-cty) and hcl, but be otherwise independent of the OpenTofu codebase.

Additionally, the OpenTofu codebase may define a global singleton to register key providers, but that should not be part
of the library.

### Implementation Caveats

The heavy use of gohcl simplifys the code significantly, but does add some complexity around providing source ranges in diagnostic errors. Changes can be made to the design to mitigate this where nessesary.

Additionaly, gohcl does not support identifiying Variables (hcl.Tranversals) contained in a hcl.Body's expression. A package has been temporarily added in `internal/varhcl` to provide this functionality. It will be upstreamed when time allows.

Overall `gohcl` is a very useful package, that will need some additional functionality if we decide to adopt it in additional locations in the tofu codebase.

#### Encryption Interface

The Encryption interface contains the methods for obtaining a State or a Plan encryptor/decryptor interface. It should be constructed with a parsed encryption.Config containing merged file/env data and encryption.Registry containing all required key_providers and methods.

#### Configuration

The Config struct and it's child structs contain all of the fields required by the above reference document, with accompanying `hcl` tags for use with gohcl. Aspects of the Config may not be known during the initial parse and will be stored as `hcl.Body`s until the key_provider or method can be identified via the registry.

It will also contain the ability to merge multiple Config structs together, overriding fields via a well documented and standard methodology (similar to how tofu currently works).

#### Registry

The Registry is a struct that contains mappings of KeyProviderSources and MethodSources. These sources are functions to construct instances
of KeyProviders and Methods respectively. After construction, the caller will need to configure them using `gohcl` before use.

##### KeyProvider

The KeyProvider is an interface responsible for providing the raw `KeyData()` as an `[]byte` and an `error`.

KeyProviders will not deal with the details of decoding a `key_provider` configuration block, other than being annotated with being annotated with `hcl` tags.

##### Method

The Method is an interface responsible for providing Encrypt/Decrypt functions that transform `[]byte` -> `[]byte`, error.

Methods will not deal with the details of decoding a `method` configuration block, other than being annotated with being annotated with `hcl` tags.

### OpenTofu integration

Integrating this library into OpenTofu's flow will be difficult, regardless of the approach taken. At the moment, there are two general approaches: singleton vs passed instance

#### Singleton

A singleton would be created in a internal package within the opentofu codebase. This singleton would be initialized once the encryption configuration can be loaded from the root module.

Pros:
* Simpler access to the singleton from calling code
* Less refactoring as the singleton is either available or not.

Cons:
* Harder to trace when/where the configuration is initialized.
* Easy to introduce new code paths that access the singleton before it is available.

#### Passed Instance

The opentofu codebase could be refactored to pass the required state encryption target through to different objects that may requre it. This includes, but is not limited to backends, plans, state manipulation commands, remote_data_source graph nodes.

Pros:
* Easy to trace where a given encryption instance comes from
* Hard to introduce new code paths without passing the correct encryption interface.

Cons:
* More in-depth refactoring is required / more of the codebase edited in this work

A example of what this may look like can be found in [this git comparison](https://github.com/cam72cam/opentofu/compare/state_encryption_config_sketch...cam72cam:opentofu:state_encryption_direct_passing). This branch is an incomplete sketch that is based off of an older understanding of what the state encryption feature would look like, however it does show many of the places that will need to be modified.

#### Hurdles

Regardless of which method we choose to initially implemnent, the trickiest integration will likely be with the command package. This package has four or five partial refactors applied to it and is an absolute mess. Making sure the encryption is initialized at the correct time, with the correct values will be a challenge.
