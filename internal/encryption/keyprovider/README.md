# OpenTofu key providers

> [!WARNING]
> This file is not an end-user documentation, it is intended for developers. Please follow the user documentation on the OpenTofu website unless you want to work on the encryption code.

This folder contains the interface that key providers must implement in order to work with OpenTofu. Please read this document carefully if you intend to work on a key provider.

## What are key providers?

Key providers in OpenTofu are responsible for integrating various key-management systems or key derivation (passphrase) functions. They consist of 3 components:

1. The **descriptor** provides a unique ID and a struct with HCL tags to read the configuration into.
2. The **configuration** is a struct OpenTofu parses the user-supplied configuration into. The `Build` function on this struct creates the key provider itself.
3. The **key provider** is responsible for creating one encryption key and one decryption key. It receives stored metadata and must return similar metadata in the result.

## What is metadata?

Some key providers need to store data alongside the encrypted data, such as the salt, the hashing function name, the key length, etc. The key provider can use this metadata to recreate the exact same key for decryption as it used for encryption. However, the key provider could also provide a different key for each encryption and decryption, allowing for a quick rotation of encryption parameters. 

> [!WARNING]
> The metadata is **bound to the key provider name**. In order words, if you change the key provider name, OpenTofu will be unable to decrypt old data.

## Implementing a key provider

When you implement a key provider, take a look at the [static](static) key provider as a template. You should never use this provider in production because it exposes users to certain weaknesses in some encryption methods, but it is a simple example for the structure.

### Testing your provider (do this first!)

Before you even go about writing a key provider, please set up the compliance tests. You can create a single test case that calls `compliancetest.ComplianceTest`. This test suite will run your key provider through all important compliance tests and will make sure that you are not missing anything during the implementation.

### Implementing the descriptor

The descriptor is very simple, you need to implement the [`Descriptor`](descriptor.go) interface in a type. (It does not have to be a struct.) However, make sure that the `ConfigStruct` always returns a struct with `hcl` tags on it. For more information on the `hcl` tags, see the [gohcl documentation](https://godocs.io/github.com/hashicorp/hcl/v2/gohcl).

### The config struct

Next, you need to create a config structure. This structure should hold all the fields you expect a user to fill out. **This must be a struct, and you must add `hcl` tags to each field you expect the user to fill out.**

Additionally, you must implement the `Build` function described in the [`Config` interface](config.go). You can take a look at [static/config.go](static/config.go) for an example on implementing this.

### The metadata

The metadata can be anything as long as it's JSON-serializable, but we recommend using a struct for future extensibility. If you do not need metadata, simply use `nil`.

Think about what data you will need to decrypt data. For example, the user may change the key length in a key derivation function, but you still need the old key length to decrypt. Hence, it needs to be part of the metadata.

> [!WARNING]
> The metadata is stored **unencrypted** and **unauthenticated**. Do not use it to store sensitive details and treat it as untrusted as it may contain malicious data.

### The key provider

The heart of your key provider is... well, your key provider. It has two functions: to create a decryption key and to create an encryption key. If your key doesn't change, these two keys can be the same. However, if you generate new keys every time, you should provide the old key as the decryption key and the new key as the encryption key. If you need to pass along data to help with recreating the decryption key, you can use the metadata for that.

### The output

Your key provider must emit the [`keyprovider.Output`](output.go) struct with the keys.