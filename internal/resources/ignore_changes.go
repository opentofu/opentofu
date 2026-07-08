// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package resources

import (
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/zclconf/go-cty/cty"
)

// Copied from NodeResourceAbstractInstance

func processIgnoreChanges(prior, config cty.Value, ignoreChanges []cty.Path, schema *configschema.Block) cty.Value {
	// ignore_changes only applies when an object already exists, since we
	// can't ignore changes to a thing we've not created yet.
	if prior.IsNull() {
		return config
	}

	ignoreAll := len(ignoreChanges) == 1 && len(ignoreChanges[0]) == 0 // Hacky signal

	if len(ignoreChanges) == 0 && !ignoreAll {
		return config
	}

	if ignoreAll {
		// Legacy providers need up to clean up their invalid plans and ensure
		// no changes are passed though, but that also means making an invalid
		// config with computed values. In that case we just don't supply a
		// schema and return the prior val directly.
		if schema == nil {
			return prior
		}

		// If we are trying to ignore all attribute changes, we must filter
		// computed attributes out from the prior state to avoid sending them
		// to the provider as if they were included in the configuration.
		ret, _ := cty.Transform(prior, func(path cty.Path, v cty.Value) (cty.Value, error) {
			attr := schema.AttributeByPath(path)
			if attr != nil && attr.Computed && !attr.Optional {
				return cty.NullVal(v.Type()), nil
			}

			return v, nil
		})

		return ret
	}

	if prior.IsNull() || config.IsNull() {
		// Ignore changes doesn't apply when we're creating for the first time.
		// Proposed should never be null here, but if it is then we'll just let it be.
		return config
	}

	return processIgnoreChangesIndividual(prior, config, ignoreChanges)
}

func processIgnoreChangesIndividual(prior, config cty.Value, ignoreChangesPath []cty.Path) cty.Value {
	type ignoreChange struct {
		// Path is the full path, minus any trailing map index
		path cty.Path
		// Value is the value we are to retain at the above path. If there is a
		// key value, this must be a map and the desired value will be at the
		// key index.
		value cty.Value
		// Key is the index key if the ignored path ends in a map index.
		key cty.Value
	}
	var ignoredValues []ignoreChange

	// Find the actual changes first and store them in the ignoreChange struct.
	// If the change was to a map value, and the key doesn't exist in the
	// config, it would never be visited in the transform walk.
	for _, icPath := range ignoreChangesPath {
		key := cty.NullVal(cty.String)
		// check for a map index, since maps are the only structure where we
		// could have invalid path steps.
		last, ok := icPath[len(icPath)-1].(cty.IndexStep)
		if ok {
			if last.Key.Type() == cty.String {
				icPath = icPath[:len(icPath)-1]
				key = last.Key
			}
		}

		// The structure should have been validated already, and we already
		// trimmed the trailing map index. Any other intermediate index error
		// means we wouldn't be able to apply the value below, so no need to
		// record this.
		p, err := icPath.Apply(prior)
		if err != nil {
			continue
		}
		c, err := icPath.Apply(config)
		if err != nil {
			continue
		}

		// If this is a map, it is checking the entire map value for equality
		// rather than the individual key. This means that the change is stored
		// here even if our ignored key doesn't change. That is OK since it
		// won't cause any changes in the transformation, but allows us to skip
		// breaking up the maps and checking for key existence here too.
		if !p.RawEquals(c) {
			// there a change to ignore at this path, store the prior value
			ignoredValues = append(ignoredValues, ignoreChange{icPath, p, key})
		}
	}

	if len(ignoredValues) == 0 {
		return config
	}

	ret, _ := cty.Transform(config, func(path cty.Path, v cty.Value) (cty.Value, error) {
		// Easy path for when we are only matching the entire value. The only
		// values we break up for inspection are maps.
		if !v.Type().IsMapType() {
			for _, ignored := range ignoredValues {
				if path.Equals(ignored.path) {
					return ignored.value, nil
				}
			}
			return v, nil
		}
		// We now know this must be a map, so we need to accumulate the values
		// key-by-key.

		if !v.IsNull() && !v.IsKnown() {
			// since v is not known, we cannot ignore individual keys
			return v, nil
		}

		// The map values will remain as cty values, so we only need to store
		// the marks from the outer map itself
		v, vMarks := v.Unmark()

		// The configMap is the current configuration value, which we will
		// mutate based on the ignored paths and the prior map value.
		var configMap map[string]cty.Value
		switch {
		case v.IsNull() || v.LengthInt() == 0:
			configMap = map[string]cty.Value{}
		default:
			configMap = v.AsValueMap()
		}

		for _, ignored := range ignoredValues {
			if !path.Equals(ignored.path) {
				continue
			}

			if ignored.key.IsNull() {
				// The map address is confirmed to match at this point,
				// so if there is no key, we want the entire map and can
				// stop accumulating values.
				return ignored.value, nil
			}
			// Now we know we are ignoring a specific index of this map, so get
			// the config map and modify, add, or remove the desired key.

			// We also need to create a prior map, so we can check for
			// existence while getting the value, because Value.Index will
			// return null for a key with a null value and for a non-existent
			// key.
			var priorMap map[string]cty.Value

			// We need to drop the marks from the ignored map for handling. We
			// don't need to store these, as we now know the ignored value is
			// only within the map, not the map itself.
			ignoredVal, _ := ignored.value.Unmark()

			switch {
			case ignored.value.IsNull() || ignoredVal.LengthInt() == 0:
				priorMap = map[string]cty.Value{}
			default:
				priorMap = ignoredVal.AsValueMap()
			}

			key := ignored.key.AsString()
			priorElem, keep := priorMap[key]

			switch {
			case !keep:
				// this didn't exist in the old map value, so we're keeping the
				// "absence" of the key by removing it from the config
				delete(configMap, key)
			default:
				configMap[key] = priorElem
			}
		}

		var newVal cty.Value
		switch {
		case len(configMap) > 0:
			newVal = cty.MapVal(configMap)
		case v.IsNull():
			// if the config value was null, and no values remain in the map,
			// reset the value to null.
			newVal = v
		default:
			newVal = cty.MapValEmpty(v.Type().ElementType())
		}

		if len(vMarks) > 0 {
			newVal = newVal.WithMarks(vMarks)
		}

		return newVal, nil
	})
	return ret
}
