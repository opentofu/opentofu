# Encryption methods

> [!WARNING]
> This file is not an end-user documentation, it is intended for developers. Please follow the user documentation on the OpenTofu website unless you want to work on the encryption code.

This folder contains the implementations for the encryption methods used in OpenTofu. Encryption methods determine how exactly data is encrypted, but they do not determine what exactly is encrypted.

## Implementing a method

When you implement a method, take a look at the [aesgcm](aesgcm) method as a template.

### Testing your method (do this first!)

Before you even go about writing a method, please set up the compliance tests. You can create a single test case that calls `compliancetest.ComplianceTest`. This test suite will run your key provider through all important compliance tests and will make sure that you are not missing anything during the implementation.

### Implementing the descriptor

The descriptor is very simple, you need to implement the [`Descriptor`](descriptor.go) interface in a type. (It does not have to be a struct.) However, make sure that the `ConfigStruct` always returns a struct with `hcl` tags on it. For more information on the `hcl` tags, see the [gohcl documentation](https://godocs.io/github.com/hashicorp/hcl/v2/gohcl).

### The config struct

Next, you need to create a config structure. This structure should hold all the fields you expect a user to fill out. **This must be a struct, and you must add `hcl` tags to each field you expect the user to fill out.**

If the config structure needs input from key providers, it should declare one HCL-tagged field with the type of [`keyprovider.Output`](../keyprovider/output.go) to receive the encryption and decryption key. Note, that the decryption key is not always available.

Additionally, you must implement the `Build` function described in the [`Config` interface](config.go). You can take a look at [aesgcm/config.go](static/config.go) for an example on implementing this.

### The method

The heart of your method is... well, your method. It has the `Encrypt()` and `Decrypt()` methods, which should perform the named tasks. If no decryption key is available, the method should refuse to decrypt data. The method should under no circumstances pass through unencrypted data if it fails to decrypt the data.
