# State encryption

This document details our intended implementation of the state and plan encryption feature.

## Notes

This document mentions `HCL` as a short-form for OpenTofu code. Unless otherwise specified, everything written also applies to the [JSON-equivalent of the HCL code](https://opentofu.org/docs/language/syntax/json/).

## Goals

The goal of this feature is to allow OpenTofu users to fully encrypt state files when they are stored on the local disk or transferred to a remote backend. The feature should also allow reading from an encrypted remote backend using the `terraform_remote_state` data source. The encrypted version should still be a valid JSON file, but not necessarily a valid state file.

Furthermore, this feature should allow users to encrypt plan files when they are stored. However, plan files are not JSON, they are undocumented binary files and should be treated as such.

For the encryption key, users should be able to specify a key directly, use a remote key provider (such as AWS KMS, etc.), or create derivative keys from another key source. The primary encryption method should be AES-GCM, but the implementation should be open to different encryption methods. The user should also have the ability to decrypt a state or plan file with one (older) key and then re-encrypt data with a newer key. Multiple fallbacks should be avoided in the implementation.

To enable use cases where multiple teams need to collaborate, the user should be able to specify separate encryption methods and keys for individual uses, especially for the `terraform_remote_state` data source. However, to simplify configuration, the user should be able to specify a default configuration for all remote state data sources.

It is the goal of this feature to let users specify their encryption configuration both in (HCL) code and in environment variables. The latter is necessary to allow users the reuse of code for both encrypted and unencrypted state storage.

Finally, it is the goal of the encryption feature to make available a library that third party tooling can use to encrypt and decrypt state. This may be implemented as a package within the OpenTofu repository, or as a standalone repository.

## Possible future goals

This section describes possible future goals. However, these goals are merely aspirations, and we may or may not implement them, or implement them differently based on community feedback. We describe these aspirations here to make clear which features we intentionally left out of scope for the current implementation.

Users use CI/CD systems or security scanners that need to read the state or plan files, but may not fully trust these systems. In the future, the user should be able to specify partial encryption. This encryption type would only encrypt sensitive values instead of the whole state file. 

At this time, due to the limitations on passing providers through to modules, encryption configuration is global. However, in the future, the user should be able to create a module that carries along their own encryption method and how it relates to the `terraform_remote_state` data sources. This is important so individual teams can ship ready-to-use modules to other teams that access their state. However, due to the constraints on passing resources to modules this is currently out of scope for this proposal.

Finally, it is a future goal to enable providers to provide their own key providers and encryption methods. Users may also want to create additional, encryption-related, such as merely signing plan files, which this functionality would enable.

## Non-goals

In this section we describe the features that are out of scope for state and plan encryption. We do not aspire to solve these problems with the same implementation, and they must be addressed separately if the community chooses to support these endeavours. 

The primary goal of this feature is to protect state and plan files **at rest**. It is not the goal of this feature to protect other channels secrets may be accessed through, such as the JSON output. As such, it is not a goal of this feature to encrypt any output on the standard output, or file output that is not a state or plan file.

Furthermore, it is not a goal of this feature to *authenticate* that the user is running an up-to-date plan file. It does not protect against, among others, replay attacks where a malicious actor replaces a current plan or state file with an old one.

It is also not a goal of this feature to protect the state file against the operator of the device running `tofu`. The operator already has access to the encryption key and can decrypt the data without the `tofu` binary being present if they so chose.

## User-facing effects

Unless the user explicitly specifies encryption options, no encryption will take place and OpenTofu will continue to function as before. No forced encryption will take place. Furthermore, regardless of the encryption status, other functionality, such as state management CLI functions, JSON output, etc. remain unaffected and will be readable in plain text if they were readable as plain text before. Only state and plan files will be affected by the encryption.

Users will be able to specify their encryption configuration both in code and via environment variables. Both configurations are equivalent and will be merged at execution time. For more details, see the [environment configuration](#environment-configuration) section below.

When a user wants to enable encryption, they must specify the following block:

```hcl2
terraform {
  encryption {
    // Encryption options
  }
}
```

The mere presence of the `encryption` block alone should not enable encryption because the user should explicitly specify what key and method to use. The implementation should error and alert the user if the encryption block is present but has no configuration.

The encryption relies on an encryption key, or a composite encryption key, which the user can provide directly or via a key management system. The user must provide at least one `key_provider` block with the settings described below. These key providers serve the purpose of creating or providing the encryption key. For example, the user could hard-code a static key named foo:

```hcl2
terraform {
  encryption {
    key_provider "static" "foo" {
      key = "6f6f706830656f67686f6834616872756f3751756165686565796f6f72653169"
    }
  }
}
```

> [!NOTE]
> The user is responsible for keeping this key safe and follow disaster recovery best practices. OpenTofu is a read-only consumer for the key the user provided and will not perform tasks on the key management system like key rotation.

The user also has to specify at least one encryption method referencing the key provider. This encryption method determines how the encryption takes place. It is the user's responsibility to make sure that they provided a key that is suitable for the encryption method.

```hcl2
terraform {
  encryption {
    //...
    method "aes_gcm" "bar" {
      key_provider = key_provider.static.foo
    }
  }
}
```

Finally, the user must reference the method for use in their state file, plan file, etc. This enables creating a different configuration for different purposes.

```hcl2
terraform {
  encryption {
    //...
    state {
      method = method.aes_gcm.abc
    }
    plan {
      method = method.aes_gcm.cde
    }
    remote_state_data_sources {
      default {
        method = method.aes_gcm.ghi
      }
      remote_state_data_source "some_module.remote_data_source.foo" {
        method = method.aes_gcm.ijk
      }
    }
  }
}
```

To facilitate key and method rollover, the user can specify a fallback configuration for state and plan decryption. When the user specifies a `fallback` block, `tofu` will first attempt to decrypt any state or plan it reads with the primary method and then fall back to the `fallback` method. When `tofu` writes a state or plan, it will not use the `fallback` method and always writes with the primary method. 

```hcl2
terraform {
  encryption {
    //...
    state {
      method = method.aes_gcm.bar
      fallback {
        method = method.aes_gcm.baz
      } 
    }
  }
}
```

> [!NOTE]
> The `fallback` is a block because the future goals may require adding additional options.

> [!NOTE]
> Multiple `fallback` blocks should not be supported or should be discouraged because they would be detrimental to performance and encourage keeping old encryption keys in the configuration.

In the situation where `enforce` is not set to true, and no `method` or `fallback` is specified, no encryption will take place. If the user does not specify a method, but specifies a fallback, the next apply command will disable the encryption given that `enforce` is not set to true. This is to allow the user to disable encryption without removing the encryption configuration from the code.

### Composite keys

When a user desires to create a composite key, such as for creating a passphrase-based derivative key or a shared custody key, they may avail themselves of a key provider that supports multiple inputs. The user can chain key providers together:

```hcl2
terraform {
  encryption {
    key_provider "static" "my_passphrase" {
      key = "this is my encryption key"
    }
    key_provider "some_derivative_key_provider" "my_derivative_key" {
      key_providers = [key_provider.static.my_passphrase]
    }
    method "aes_gcm" "foo" {
      key_provider = key_provider.some_derivative_key_provider.my_derivative_key
    }
    //...
  }
}
```

> [!NOTE]
> The specific implementation of the derivative key must pay attention to combine the keys securely if it supports multiple key providers as inputs, such as using HMAC to combine keys.

### Environment configuration

As mentioned above, users can configure encryption in environment variables, either as HCL or JSON. To do this, the user has to specify the encryption configuration fragment in either of the two formats. The following two examples are equivalent: 

```hcl2
key_provider "static" "my_key" {
  key = "this is my encryption key"
}
method "aes_gcm" "foo" {
  key_provider = key_provider.static.my_key
}
state {
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
  "state": {
    "method": "${method.aes_gcm.foo}"
  }
}
```

The user can set either of these structures in the `TF_ENCRYPTION` environment variable:

```bash
export TF_ENCRYPTION='{"key_provider":{...},"method":{...},"state":{...}}'
```

When the user specifies both an environment and a code configuration, `tofu` merges the two configurations. If two values conflict, the environment configuration takes precedence.

To ensure that the encryption cannot be accidentally forgotten or disabled and the data stored unencrypted, the user can specify the `enforced` option in the HCL configuration:

```hcl2
terraform {
  encryption {
    //...
    state {
      enforced = true
    }
    plan {
      enforced = true
    }
  }
}
```

> [!NOTE]
> The `enforced` option is also available in the environment configuration and works as intended, but doesn't make much sense because its primary purpose is to guard against environment variable omission.

## Encrypted state format

When `tofu` encrypts a state file, the encrypted state is still a JSON file. Any implementations can distinguish encrypted files by the `encryption` key being present in the JSON structure. However, there may be other keys in the state file depending on the encryption method and type.

For example (not final):

```json
{
  "encryption": {
    "method": "aes_gcm",
    "key_provider": "static.my_key"
  },
  // ... Additional keys here
}
```

> [!WARNING]
> Tools working with state files should not make assumptions about the type or structure of the `encryption` field as it may vary from implementation to implementation.

## Encrypted plan format

An unencrypted plan file in OpenTofu is an opaque binary. This specification makes no rules for how the encrypted format should look like and all non-encryption routines should treat the value as opaque.

## Implementation

When implementing the encryption tooling, the implementation should be split in two parts: the library and the OpenTofu implementation. The library should rely on the cty types and hcl as a means to specify schema, but should not be otherwise tied to the OpenTofu codebase. This is necessary to enable encryption capabilities for third party tooling that may need to work with state and plan files.

### Library implementation

The encryption library should create an interface that other projects and OpenTofu itself can use to encrypt and decrypt state. The library should express its schema needs (e.g. for key provider config) using [cty](https://github.com/zclconf/go-cty) and [hcl](https://github.com/hashicorp/hcl), but be otherwise independent of the OpenTofu codebase. Ideally, the OpenTofu project should provide this library as a standalone dependency that does not pull in the entire OpenTofu dependency tree.

#### Encryption interface

The main component of the library should be the `Encryption` interface. This interface should provide methods to request an encryption tool for each individual purpose, such as:

```go
type Encryption interface {
	State() StateEncryption
	Plan() PlanEncryption
	RemoteState(string) StateEncryption
}
```

Each of the returned encryption tools should provide methods to encrypt the data of the specified purpose, such as:

```go
type StateEncryption interface {
    DecryptState([]byte) ([]byte, error)	
	EncryptState([]byte) ([]byte, error)
}
```

The encryption routines should assume that they get passed a valid state or plan file and encrypt it as described in this document. Conversely, the decryption routines should assume that their input will be an encrypted state or plan file and should attempt to decrypt. The state decryption function should follow the fallback process described in this document.

#### Key providers

The main responsibility of a key provider is providing a key in a `[]byte`. It may consume structured configuration, which may also include references to other key providers. However, an implementation of a key provider should never have to deal with resolving these dependencies. Instead, the library should correctly resolve the key provider order and look up the keys in the right order and pass the already-resolved data in as [configuration](#configuration).

In addition to the encryption key, key providers may also emit additional metadata. The library must store this metadata alongside the encrypted data and pass it to the key provider when initializing the key provider for decryption in a subsequent run. The key provider is responsible for ensuring that no sensitive data is stored in the metadata.

> [!NOTE]
> Since a user can chain key providers, the library must make sure to store metadata from all key providers in the encrypted form. However, when the user renames the key provider the library may fail to decrypt the state or plan files if the user fails to provide an adequate fallback with the correct naming. The documentation for this feature should encourage users to create new key providers if they change the parameters in a backwards-incompatible manner, and they want to decrypt older state or plan files.

#### Methods

The responsibility of a method is to encrypt and decrypt an opaque block of data. The method is not responsible for understanding the structure of the data. Instead, the library core should take care of traversing the state or plan files and deciding specifically what to encrypt. A method must implement the encrypted format in such a way that it can determine if a subsequent decryption failed or not. Methods that cannot decide on decryption success without validating the underlying data, such as rot13, are not supported.

Similar to [key providers](#key-providers), the method may need configuration but should not have to deal with lookup up key providers itself.

#### Registering key providers and methods

The library should be modular. Anyone using the library, including OpenTofu, should be able to add new key providers and methods and read the configuration for these without modifying the library code. To that end, the library should provide a registry for key providers and methods.

The library should also not force any included key providers or methods onto its user, so the registry should not be global. Instead, every library user should configure their own registry. However, the library should provide a way to obtain a preconfigured registry with built-in key providers and methods.

#### Configuration

In order to ensure consistency between OpenTofu and other library users, the library should provide a method to parse an HCL or JSON block and turn it into configuration structures. In parallel, the library should also make it as simple as possible for implementers to safely provide new key providers and methods, which is why the library should also use struct tags in Go to convert the incoming configuration.  

```go
type Config struct {
	Key string `hcl:"key"`
}
```
