package testhelpers

import (
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/zclconf/go-cty/cty"
)

// SimpleTestSchema returns a block schema that contains a few optional
// attributes for use in tests.
//
// The returned schema contains the following optional attributes:
//
//   - test_string, of type string
//   - test_number, of type number
//   - test_bool, of type bool
//   - test_list, of type list(string)
//   - test_map, of type map(string)
//
// Each call to this function produces an entirely new schema instance, so
// callers can feel free to modify it once returned.
func SimpleTestSchema() *configschema.Block {
	return &configschema.Block{
		Attributes: map[string]*configschema.Attribute{
			"test_string": {
				Type:     cty.String,
				Optional: true,
			},
			"test_number": {
				Type:     cty.Number,
				Optional: true,
			},
			"test_bool": {
				Type:     cty.Bool,
				Optional: true,
			},
			"test_list": {
				Type:     cty.List(cty.String),
				Optional: true,
			},
			"test_map": {
				Type:     cty.Map(cty.String),
				Optional: true,
			},
		},
	}
}
