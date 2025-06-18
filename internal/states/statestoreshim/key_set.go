// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package statestoreshim

import (
	"iter"

	"github.com/opentofu/opentofu/internal/states/statekeys"
	"github.com/opentofu/opentofu/internal/states/statestore"
)

// CollectStorageKeys takes an iterable sequence of [statekeys.Key] values
// and produces a [statestore.KeySet] of unique storage-level representations
// of the same keys.
func CollectStorageKeys(from iter.Seq[statekeys.Key]) statestore.KeySet {
	if from == nil {
		return nil
	}
	ret := make(statestore.KeySet)
	for key := range from {
		storageKey := key.ForStorage()
		ret[storageKey] = struct{}{}
	}
	return ret
}
