// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package encryption

import "context"

// As early as possible, we store encryption in the context for use in obscure areas within OpenTofu

type ctxEncryptionKey struct{}

func ContextWithEncryption(parent context.Context, enc Encryption) context.Context {
	return context.WithValue(parent, ctxEncryptionKey{}, enc)
}

func ContextEncryption(ctx context.Context) Encryption {
	ret, ok := ctx.Value(ctxEncryptionKey{}).(Encryption)
	if !ok {
		// we ALWAYS need to return something for encryption
		// if this was not set in the context, set to disabled
		ret = Disabled()
	}
	return ret
}
