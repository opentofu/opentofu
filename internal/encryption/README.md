# OpenTofu State and Plan encryption

> [!WARNING]
> This file is not an end-user documentation, it is intended for developers. Please follow the user documentation on the OpenTofu website unless you want to work on the encryption code.

This folder contains the code for state and plan encryption. For a quick example on how to use this package, please take a look at the [example_test.go](example_test.go) file.

## Structure

The current folder contains the top level API. It requires a registry for holding the available key providers and encryption methods, which is located in the [registry](registry/) folder. The key providers are located in the [keyprovider](keyprovider/) folder, while the encryption methods are located in the [method](method) folder. You can also find the configuration struct and its related functions in the [config](config) folder.

## Further reading

For a detailed design document on state encryption, please read [this document](../../docs/state_encryption.md).