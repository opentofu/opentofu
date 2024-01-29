# State encryption

This document details our intended implementation of the state and plan encryption feature.

## Notes

This document mentions `HCL` as a short-form for OpenTofu code. Unless otherwise specified, everything written also applies to the [JSON-equivalent of the HCL code](https://opentofu.org/docs/language/syntax/json/).

## Goals

The goal of this feature is to allow OpenTofu users to fully encrypt state files when they are stored on the local disk, transferred to a remote backend. The feature should also allow reading from an encrypted remote backend using the `terraform_remote_state` data source. The encrypted version should still be a valid JSON file, but not necessarily a valid state file.

Furthermore, this feature should allow users to encrypt plan files when they are stored. However, plan files are not JSON, so there is no expectation of the resulting plan file being JSON either. **TODO: do we want to specify that we use ZIP encryption semantics?**

For the encryption key, users should be able to specify a key directly, use a remote key provider (such as AWS KMS, etc.), or create derivative keys (such as pbkdf2) from another key source. The primary encryption method should be AES-GCM, but the implementation should be open to different encryption methods. The user should also have the ability to decrypt a state file with one (older) key and then re-encrypt data with a newer key. As plan files are not long-lived, this only applies to state files.

To enable use cases where multiple teams need to collaborate, the user should be able to specify separate encryption methods and keys for individual uses, especially for the `terraform_remote_state` data source. However, to simplify configuration, the user should be able to specify single configuration for all remote state data sources.

It is the goal of this feature to let users specify their encryption configuration both in HCL and in environment variables. The latter is necessary to allow users the reuse of HCL code for encrypted and unencrypted state storage.

Finally, it is the goal of the encryption feature to make available a library that third party tooling can use to encrypt and decrypt state. **CAM: Should this be in Future for the first iteration?**

## Future goals

In the future, the user should be able to specify partial encryption. This encryption type would only encrypt sensitive values instead of the whole state file. **TODO how about the plan file?**

At this time, due to the limitations on passing providers through to modules, encryption configuration is global. However, in the future, the user should be able to create a module that carries along their own encryption method and how it relates to the `terraform_remote_state` data sources. This is important so individual teams can ship ready-to-use modules to other teams that access their state. However, due to the constraints on passing resources to modules this is currently out of scope for this proposal.

Finally, it is a future goal to enable providers to provide their own key providers and encryption methods.

## Non-goals

The primary goal of this feature is to protect state and plan files at rest. It is not the goal of this feature to protect other channels secrets may be accessed through, such as the JSON output. As such, it is not a goal of this feature to encrypt any output on the standard output, or file output that is not a state or plan file.

It is also not a goal of this feature to protect the state file against the operator of the device running `tofu`. The operator already has access to the encryption key and can decrypt the data without the `tofu` binary being present if they so chose.

## User-facing effects

Unless the user explicitly specifies encryption options, no encryption will take place. Other functionality, such as state management CLI functions, JSON output, etc. remain unaffected. Only state and plan files will be affected by the encryption.

Users will be able to specify their encryption configuration both in HCL and via environment variables. The environment variables must contain the JSON-equivalent of the HCL code, see the specific description below. Both configurations are equivalent and will be merged at execution time. For more details, see the [environment configuration](#environment-configuration) section below.
**CAM: The env var can be either HCL or JSON, it uses the same code path as reading a .hcl or .hcl.json file off disk.**

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

(JH: I don't like this functionality really. Maybe we can drop it? I can't find a nice reason to do it this way apart from avoiding polluting the encryption block in the terraform block)
Additionally, `terraform_remote_state` data source blocks can also contain an encryption configuration: **TODO: we are treating this data source as a special snowflake compared to other, provider-based data sources. I think this is not consistent and should be removed.**
**CAM: It's already a special snowflake as it builds a backend configuration on the fly and initializes it.  This is the most convenient way of doing this and I would expect to be able to do this as a user.  The implementation code impact is fairly minimal?**
** DEFERR TO FUTURE**

```hcl
data "terraform_remote_state" "foo" {
  //...
  encryption {
    method = method.aes_gcm.bar
  }
}
```

To facilitate key and method rollover, the user can specify a fallback configuration for state-related decryption, which applies to the `statefile`, `backend` and `remote_data_sources` blocks. The `planfile` block does not allow for a fallback configuration as plans are generally short-lived. When the user specifies a `fallback` block, `tofu` will first attempt to decrypt any state it reads with the primary method and then fall back to the `fallback` method. When `tofu` writes state, it does not use the `fallback` method and always writes with the primary method.
**CAM: Is there any harm in supporting fallback for plan?  I'm pretty sure the code would be simpler to keep it?**

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

As mentioned above, users can configure encryption in environment variables. The environment variables in this section contain a JSON configuration fragment in the [JSON configuration syntax](https://opentofu.org/docs/language/syntax/json/). With this format, the user can transform the following encryption block:

```hcl2
terraform {
  encryption {
    key_provider "static" "my_key" {
      key = "this is my encryption key"
    }
    method "aes_gcm" "foo" {
      key_provider = key_provider.static.my_key
    }
    statefile {
      method = method.aes_gcm.foo
    }
  }
}
```
**CAM: We probably don't want to wrap it in the `terraform -> encryption` block**

The resulting JSON will be:

```json
{
  "terraform": {
    "encryption": {
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
  }
}
```
**CAM: Same as above**

A user can now take the `terraform` → `encryption` part of this JSON and load it into the `TF_ENCRYPTION` environment variable:

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

When implementing the encryption tooling, the implementation should be split in two parts: the library and the OpenTofu implementation. The library should rely on the cty types as a means to specify schema, but should not be otherwise tied to the OpenTofu codebase. This is necessary to enable encryption capabilities for third party tooling that may need to work with state and plan files.

### Library implementation

**TODO what API should the library provide?**

The encryption library should create an interface that other projects and OpenTofu itself can use to encrypt and decrypt state. The library should express its schema needs (e.g. for key provider config) using [cty](https://github.com/zclconf/go-cty), but be otherwise independent of the OpenTofu codebase.

#### Top level interface

The top level interface is responsible for taking the `encryption` HCL block and setting up the key providers. Library users (both OpenTofu and third party tooling) should use these interfaces to access the encryption layer. Libraries wishing to implement a key provider or an encryption method should use the corresponding interfaces below.

```go
// EncryptionRegistry is a holder of KeyProvider and Method implementations. Key providers and methods can register
// themselves with this registry. You can call the Configure function to parse an HCL block as configuration.
type EncryptionRegistry interface {
	RegisterKeyProvider(KeyProvider) error
	RegisterMethod(Method) error

}

// Encryption contains the methods for obtaining a StateEncryption or PlanEncryption correctly configured for a specific
// purpose. If no encryption configuration is present, it returns a passthru method that doesn't do anything.
type Encryption interface {
	StateFile() StateEncryption
	PlanFile() PlanEncryption
	Backend() StateEncryption
	// RemoteString returns a StateEncryption suitable for a remote state data source. Note: this function panics
	// if the path of the remote state data source is invalid, but does not panic if it is incorrect.
	RemoteState(string) StateEncryption
}

type StateEncryption interface {
    EncryptState([]byte) ([]byte, error)
    DecryptState([]byte) ([]byte, error)
}
type PlanEncryption interface {
    EncryptPlan([]byte) ([]byte, error)
    DecryptState([]byte) ([]byte, error)
}
```

```go
JANOS IS ALSO MESSING AROUND - HE HATES THIS

type EncryptionConfiguration struct {
    KeyProviders map[string]KeyProviderConfig
}

type KeyProviderConfig map[string]any

MORE JAMES MESSING AROUND WITH KEY PROVIDER CONFIG

type KeyProviderInterfaceWhatever interface {
	New(config any) KeyProvideKeyProviderInterfaceWhatever
	GetKey()
}

type StaticKeyProviderInterfaceWhatever struct {
	KeyValue string
}

func (s *StaticKeyProviderInterfaceWhatever) GetKey() any? {
	// read from the (already merged?) config here and return
}

JAMES MESSING AROUND BLOCK

var DefaultEncryptionRegistry EncryptionRegistry

// EncryptionRegistry is a holder of KeyProvider and Method implementations. Key providers and methods can register
// themselves with this registry. You can call the Configure function to parse an HCL block as configuration.
type EncryptionRegistry interface {
	RegisterKeyProvider(KeyProvider) error
	RegisterMethod(Method) error
}

// Encryption contains the methods for obtaining a StateEncryption or PlanEncryption correctly configured for a specific
// purpose. If no encryption configuration is present, it returns a passthru method that doesn't do anything.
type Encryption interface {
	StateFile() StateEncryption
	PlanFile() PlanEncryption
	Backend() StateEncryption
	// RemoteString returns a StateEncryption suitable for a remote state data source. Note: this function panics
	// if the path of the remote state data source is invalid, but does not panic if it is incorrect.
	RemoteState(string) StateEncryption
}

func New(reg EncryptionRegistry, config enccfg.ConfigMap) *Encryption {
	return
}

type StateEncryption interface {
    EncryptState([]byte) ([]byte, error)
    DecryptState([]byte) ([]byte, error)
}
type PlanEncryption interface {
    EncryptPlan([]byte) ([]byte, error)
    DecryptState([]byte) ([]byte, error)
}

func init() {
	DefaultEncryptionRegistry := EncryptionRegistry{}
	DefaultEncryptionRegistry.RegisterKeyProvider(staticKeyProvider.New())
	DefaultEncryptionRegistry.RegisterMethod(.....)
}
```



Additionally, the OpenTofu codebase may define a global singleton to register key providers, but that should not be part
of the library.

#### KeyProvider

The key provider is responsible for providing the key.

```
config = "${key_provider.a.foo}${key_provider.a.bar}"
```

```go
type Schema struct {
	BodySchema hcl.BodySchema
	ReferenceFields []string
}

type KeyProvider interface {
	Schema() Schema

	Configure(cty.Block) (KeyProviderInstance, error)
}

// cty.Body + hcl.BodySchema -> cty.Block with Attributes / Sub Blocks

type KeyProviderInstance interface {
	Provide() ([]byte, error)
}
```

```go

// Meta_something.go

encryptionpkg.RegisterProvider()

var config EncryptionConfig
// All of your key providers and methods
// Gather configs from env / root module

// Either:
diags := encryptionpkg.SetupInstance(config)
// OR
metainst.encr, diags = encryptionpkg.Instance(config)

// At this point the config is wholy known (*)
// All key_providers / methods will be initialized

// Now you can either use
encryptionpkg.Instance().Stuff(...)
// OR
metainst.ecry.Stuff(...)
```


```go
// Lives inside the encryptor (impl Encryption interface)

// param block

providerSetup := encr.GetKeyProvider(block.label)
schema := providerSetup.GetSchema()
block, diags := schema.Decode(block.body)
provider := providerSetup.Init(block)
idents := provider.Dependencies()

```


```go
type KeyProviderInit func(cty.Body, cty.Range) (*KeyProvider, tfdiags)

type KeyProvider struct {
    func Provide() ([]byte, error)
	func Dependencies ([]string)
}

type EncryptionRegistry interface {
    func RegisterKeyProvider(name string, KeyProviderInit)
    func GetKeyProvider(name string, args cty.Value) (*KeyProvider, tfdiags)


type ERI struct {
    keyProviders map[string]KeyProviderInit
}
```

#### Method

```go
type Method interface {
	Schema() cty.???

	Configure(cty.???) (MethodInstance, error)
}

type MethodInstance interface {
    Encrypt(value []byte) ([]byte, error)
}
```

### OpenTofu integration

**TODO document the constraints of the OpenTofu integration. Where should we hook this into?**





/*

map1 := loadStateEncryptionFile("main.tf")
map2 := loadStateEncryptionFile("variables.tf")
map3 := loadStateEncryptionEnv()

merged := mergeOverrides(map1, map2, map3)

// Do it here for init errors

m.ecr = encpkg.Setup(merged)
encpkg.Singleton(merged)

*/
<!--  -->
