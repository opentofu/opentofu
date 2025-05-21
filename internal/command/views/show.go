// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package views

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/opentofu/opentofu/internal/cloud/cloudplan"
	"github.com/opentofu/opentofu/internal/command/arguments"
	"github.com/opentofu/opentofu/internal/command/jsonconfig"
	"github.com/opentofu/opentofu/internal/command/jsonformat"
	"github.com/opentofu/opentofu/internal/command/jsonplan"
	"github.com/opentofu/opentofu/internal/command/jsonprovider"
	"github.com/opentofu/opentofu/internal/command/jsonstate"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/plans"
	"github.com/opentofu/opentofu/internal/states/statefile"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/opentofu/opentofu/internal/tofu"
)

type Show interface {
	// DisplayState renders the given state snapshot, returning a status code for "tofu show" to return.
	DisplayState(ctx context.Context, stateFile *statefile.File, schemas *tofu.Schemas) int

	// DisplayPlan renders the given plan, returning a status code for "tofu show" to return.
	//
	// Unfortunately there are two possible ways to represent a plan:
	// - Locally-generated plans are loaded as *plans.Plan.
	// - Remotely-generated plans (using remote operations) are loaded as *cloudplan.RemotePlanJSON.
	//
	// Therefore the implementation of this method must handle both cases,
	// preferring planJSON if it is not nil and using plan otherwise.
	DisplayPlan(ctx context.Context, plan *plans.Plan, planJSON *cloudplan.RemotePlanJSON, config *configs.Config, priorStateFile *statefile.File, schemas *tofu.Schemas) int

	// DisplayConfig renders the given configuration in JSON format, returning a status code for "tofu show" to return.
	DisplayConfig(config *configs.Config, schemas *tofu.Schemas) int

	// Diagnostics renders early diagnostics, resulting from argument parsing.
	Diagnostics(diags tfdiags.Diagnostics)
}

func NewShow(vt arguments.ViewType, view *View) Show {
	switch vt {
	case arguments.ViewJSON:
		return &ShowJSON{view: view}
	case arguments.ViewHuman:
		return &ShowHuman{view: view}
	default:
		panic(fmt.Sprintf("unknown view type %v", vt))
	}
}

type ShowHuman struct {
	view *View
}

var _ Show = (*ShowHuman)(nil)

func (v *ShowHuman) DisplayState(_ context.Context, stateFile *statefile.File, schemas *tofu.Schemas) int {
	renderer := jsonformat.Renderer{
		Colorize:            v.view.colorize,
		Streams:             v.view.streams,
		RunningInAutomation: v.view.runningInAutomation,
		ShowSensitive:       v.view.showSensitive,
	}

	if stateFile == nil {
		v.view.streams.Println("No state.")
		return 0
	}

	root, outputs, err := jsonstate.MarshalForRenderer(stateFile, schemas)
	if err != nil {
		v.view.streams.Eprintf("Failed to marshal state to json: %s", err)
		return 1
	}

	jstate := jsonformat.State{
		StateFormatVersion:    jsonstate.FormatVersion,
		ProviderFormatVersion: jsonprovider.FormatVersion,
		RootModule:            root,
		RootModuleOutputs:     outputs,
		ProviderSchemas:       jsonprovider.MarshalForRenderer(schemas),
	}

	renderer.RenderHumanState(jstate)
	return 0
}

func (v *ShowHuman) DisplayPlan(_ context.Context, plan *plans.Plan, planJSON *cloudplan.RemotePlanJSON, config *configs.Config, priorStateFile *statefile.File, schemas *tofu.Schemas) int {
	renderer := jsonformat.Renderer{
		Colorize:            v.view.colorize,
		Streams:             v.view.streams,
		RunningInAutomation: v.view.runningInAutomation,
		ShowSensitive:       v.view.showSensitive,
	}

	// Prefer to display a pre-built JSON plan, if we got one; then, fall back
	// to building one ourselves.
	if planJSON != nil {
		if !planJSON.Redacted {
			v.view.streams.Eprintf("Didn't get renderable JSON plan format for human display")
			return 1
		}
		// The redacted json plan format can be decoded into a jsonformat.Plan
		p := jsonformat.Plan{}
		r := bytes.NewReader(planJSON.JSONBytes)
		if err := json.NewDecoder(r).Decode(&p); err != nil {
			v.view.streams.Eprintf("Couldn't decode renderable JSON plan format: %s", err)
		}

		v.view.streams.Print(v.view.colorize.Color(planJSON.RunHeader + "\n"))
		renderer.RenderHumanPlan(p, planJSON.Mode, planJSON.Qualities...)
		v.view.streams.Print(v.view.colorize.Color("\n" + planJSON.RunFooter + "\n"))
	} else if plan != nil {
		outputs, changed, drift, attrs, err := jsonplan.MarshalForRenderer(plan, schemas)
		if err != nil {
			v.view.streams.Eprintf("Failed to marshal plan to json: %s", err)
			return 1
		}

		jplan := jsonformat.Plan{
			PlanFormatVersion:     jsonplan.FormatVersion,
			ProviderFormatVersion: jsonprovider.FormatVersion,
			OutputChanges:         outputs,
			ResourceChanges:       changed,
			ResourceDrift:         drift,
			ProviderSchemas:       jsonprovider.MarshalForRenderer(schemas),
			RelevantAttributes:    attrs,
		}

		var opts []plans.Quality
		if !plan.CanApply() {
			opts = append(opts, plans.NoChanges)
		}
		if plan.Errored {
			opts = append(opts, plans.Errored)
		}

		renderer.RenderHumanPlan(jplan, plan.UIMode, opts...)
	} else {
		v.view.streams.Println("No plan.")
	}
	return 0
}

func (v *ShowHuman) DisplayConfig(config *configs.Config, schemas *tofu.Schemas) int {
	// The human view should never be called for configuration display
	// since we require -json for -config
	v.view.streams.Eprintf("Internal error: human view should not be used for configuration display")
	return 1
}

func (v *ShowHuman) Diagnostics(diags tfdiags.Diagnostics) {
	v.view.Diagnostics(diags)
}

type ShowJSON struct {
	view *View
}

var _ Show = (*ShowJSON)(nil)

func (v *ShowJSON) DisplayState(_ context.Context, stateFile *statefile.File, schemas *tofu.Schemas) int {
	jsonState, err := jsonstate.Marshal(stateFile, schemas)
	if err != nil {
		v.view.streams.Eprintf("Failed to marshal state to json: %s", err)
		return 1
	}
	v.view.streams.Println(string(jsonState))
	return 0
}

func (v *ShowJSON) DisplayPlan(_ context.Context, plan *plans.Plan, planJSON *cloudplan.RemotePlanJSON, config *configs.Config, priorStateFile *statefile.File, schemas *tofu.Schemas) int {
	// Prefer to display a pre-built JSON plan, if we got one; then, fall back
	// to building one ourselves.
	if planJSON != nil {
		if planJSON.Redacted {
			v.view.streams.Eprintf("Didn't get external JSON plan format")
			return 1
		}
		v.view.streams.Println(string(planJSON.JSONBytes))
	} else if plan != nil {
		planJSON, err := jsonplan.Marshal(config, plan, priorStateFile, schemas)

		if err != nil {
			v.view.streams.Eprintf("Failed to marshal plan to json: %s", err)
			return 1
		}
		v.view.streams.Println(string(planJSON))
	} else {
		// Should not get here because at least one of the two plan arguments
		// should be present, but we'll tolerate this by just returning an
		// empty JSON object.
		v.view.streams.Println("{}")
	}
	return 0
}

func (v *ShowJSON) DisplayConfig(config *configs.Config, schemas *tofu.Schemas) int {
	configJSON, err := jsonconfig.Marshal(config, schemas)
	if err != nil {
		v.view.streams.Eprintf("Failed to marshal configuration to JSON: %s", err)
		return 1
	}
	v.view.streams.Println(string(configJSON))
	return 0
}

// Diagnostics should only be called if show cannot be executed.
// In this case, we choose to render human-readable diagnostic output,
// primarily for backwards compatibility.
func (v *ShowJSON) Diagnostics(diags tfdiags.Diagnostics) {
	v.view.Diagnostics(diags)
}
