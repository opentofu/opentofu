// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package statekeys

import (
	"fmt"
	"strings"

	"github.com/opentofu/opentofu/internal/states/statestore"
)

type Key interface {
	ForStorage() statestore.Key
	keySigil() // this is a sealed interface
}

var handlers = map[string]func(string) (Key, error){
	resourcePrefix:              resourceFromStorage,
	resourceInstancePrefix:      resourceInstanceFromStorage,
	rootModuleOutputValuePrefix: rootModuleOutputValueFromStorage,
}

// KeyFromStorage attempts to parse a previously-stored key into a higher-level
// representation with additional semantics.
//
// This function is written under the assumption that OpenTofu should only
// be parsing keys it originally generated and that direct end-user tampering
// is unsupported, so the errors returned by this function are intentionally
// simple and generic because they should not happen under correct use of
// OpenTofu.
func KeyFromStorage(stored statestore.Key) (Key, error) {
	raw := stored.Name()
	prefix, suffix, found := strings.Cut(raw, "-")
	if !found {
		return nil, fmt.Errorf("invalid state storage key: no hyphen-minus character")
	}

	handler, ok := handlers[prefix]
	if !ok {
		return nil, fmt.Errorf("unrecognized key prefix %q", prefix)
	}

	// The suffix is always a base32-encoded _something_, but we'll let the
	// handler decide what.
	suffix, err := decodeBase32(suffix)
	if err != nil {
		return nil, err
	}
	return handler(suffix)
}

// makeStorageKey is a small helper that concatenates the given prefix with
// the given suffix and returns the result as a [statestore.Key], without
// any further checks. Callers should ensure that the prefix includes only
// ASCII lowercase letters and digits, or the results are unspecified and
// likely to be upsetting.
func makeStorageKey(prefix, suffix string) statestore.Key {
	return statestore.MakeKey(prefix + "-" + encodeBase32(suffix))
}
