package tofu

import (
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs/hcl2shim"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/zclconf/go-cty/cty"
)

// providerForTest is a wrapper around real provider to allow certain resources to be overridden
// for testing framework. Currently, it's used in NodeAbstractResourceInstance only in a format
// of one time use. It handles overrideValues and plannedChange for a single resource instance
// (i.e. by certain address).
// TODO: providerForTest should be extended to handle mock providers implementation with per-type
// mocking. It will allow providerForTest to be used for both overrides and full mocking.
// In such scenario, overrideValues should be extended to handle per-type values and plannedChange
// should contain per PlanResourceChangeRequest cache to produce the same plan result
// for the same PlanResourceChangeRequest.
type providerForTest struct {
	providers.Interface
	schema providers.ProviderSchema

	overrideValues map[string]cty.Value
	plannedChange  *cty.Value
}

func (p providerForTest) ReadResource(r providers.ReadResourceRequest) providers.ReadResourceResponse {
	var resp providers.ReadResourceResponse

	resSchema, _ := p.schema.SchemaForResourceType(addrs.ManagedResourceMode, r.TypeName)

	resp.NewState, resp.Diagnostics = hcl2shim.ComposeMockValueBySchema(resSchema, r.ProviderMeta, p.overrideValues)
	return resp
}

func (p providerForTest) PlanResourceChange(r providers.PlanResourceChangeRequest) providers.PlanResourceChangeResponse {
	if r.Config.IsNull() {
		return providers.PlanResourceChangeResponse{
			PlannedState: r.ProposedNewState, // null
		}
	}

	if p.plannedChange != nil {
		return providers.PlanResourceChangeResponse{
			PlannedState: *p.plannedChange,
		}
	}

	resSchema, _ := p.schema.SchemaForResourceType(addrs.ManagedResourceMode, r.TypeName)

	var resp providers.PlanResourceChangeResponse

	resp.PlannedState, resp.Diagnostics = hcl2shim.ComposeMockValueBySchema(resSchema, r.Config, p.overrideValues)

	return resp
}

func (p providerForTest) ApplyResourceChange(r providers.ApplyResourceChangeRequest) providers.ApplyResourceChangeResponse {
	return providers.ApplyResourceChangeResponse{
		NewState: r.PlannedState,
	}
}

func (p providerForTest) ReadDataSource(r providers.ReadDataSourceRequest) providers.ReadDataSourceResponse {
	resSchema, _ := p.schema.SchemaForResourceType(addrs.DataResourceMode, r.TypeName)

	var resp providers.ReadDataSourceResponse

	resp.State, resp.Diagnostics = hcl2shim.ComposeMockValueBySchema(resSchema, r.Config, p.overrideValues)

	return resp
}
