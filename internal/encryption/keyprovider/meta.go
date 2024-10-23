// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package keyprovider

// KeyMeta is a type alias for a struct annotated with JSON tags to store. Its purpose is to store parameters alongside
// the encrypted data which are required to later provide a decryption key.
//
// Key providers can use this to store, for example, a randomly generated salt value which is required to later provide
// the same decryption key.
type KeyMeta any

// MetaStorageKey signals the key under which the metadata for a specific key provider is stored.
type MetaStorageKey string
