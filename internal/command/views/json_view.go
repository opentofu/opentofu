// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package views

import (
	encJson "encoding/json"
	"fmt"

	"github.com/hashicorp/go-hclog"

	"github.com/opentofu/opentofu/internal/command/jsonentities"
	"github.com/opentofu/opentofu/internal/command/views/json"
	"github.com/opentofu/opentofu/internal/tfdiags"
	tfversion "github.com/opentofu/opentofu/version"
)

// This version describes the schema of JSON UI messages. This version must be
// updated after making any changes to this view, the jsonHook, or any of the
// command/views/json package.
const JSON_UI_VERSION = "1.2"

func NewJSONView(view *View) *JSONView {
	log := hclog.New(&hclog.LoggerOptions{
		Name:               "tofu.ui",
		Output:             view.streams.Stdout.File,
		JSONFormat:         true,
		JSONEscapeDisabled: true,
	})
	jv := &JSONView{
		log:  log,
		view: view,
	}
	jv.Version()
	return jv
}

type JSONView struct {
	// hclog is used for all output in JSON UI mode. The logger has an internal
	// mutex to ensure that messages are not interleaved.
	log hclog.Logger

	// We hold a reference to the view entirely to allow us to access the
	// ConfigSources function pointer, in order to render source snippets into
	// diagnostics. This is even more unfortunate than the same reference in the
	// view.
	//
	// Do not be tempted to dereference the configSource value upon logger init,
	// as it will likely be updated later.
	view *View
}

func (v *JSONView) Version() {
	version := tfversion.String()
	v.log.Info(
		fmt.Sprintf("OpenTofu %s", version),
		"type", json.MessageVersion,
		"tofu", version,
		"ui", JSON_UI_VERSION,
	)
}

func (v *JSONView) Log(message string) {
	v.log.Info(message, "type", json.MessageLog)
}

func (v *JSONView) StateDump(state string) {
	v.log.Info(
		"Emergency state dump",
		"type", json.MessageLog,
		"state", encJson.RawMessage(state),
	)
}

func (v *JSONView) Diagnostics(diags tfdiags.Diagnostics, metadata ...interface{}) {
	sources := v.view.configSources()
	for _, diag := range diags {
		diagnostic := jsonentities.NewDiagnostic(diag, sources)

		args := []interface{}{"type", json.MessageDiagnostic, "diagnostic", diagnostic}
		args = append(args, metadata...)

		switch diag.Severity() {
		case tfdiags.Warning:
			v.log.Warn(fmt.Sprintf("Warning: %s", diag.Description().Summary), args...)
		default:
			v.log.Error(fmt.Sprintf("Error: %s", diag.Description().Summary), args...)
		}
	}
}

func (v *JSONView) PlannedChange(c *jsonentities.ResourceInstanceChange) {
	v.log.Info(
		c.String(),
		"type", json.MessagePlannedChange,
		"change", c,
	)
}

func (v *JSONView) ResourceDrift(c *jsonentities.ResourceInstanceChange) {
	v.log.Info(
		fmt.Sprintf("%s: Drift detected (%s)", c.Resource.Addr, c.Action),
		"type", json.MessageResourceDrift,
		"change", c,
	)
}

func (v *JSONView) ChangeSummary(cs *json.ChangeSummary) {
	v.log.Info(
		cs.String(),
		"type", json.MessageChangeSummary,
		"changes", cs,
	)
}

func (v *JSONView) Hook(h json.Hook) {
	v.log.Info(
		h.String(),
		"type", h.HookType(),
		"hook", h,
	)
}

func (v *JSONView) Outputs(outputs jsonentities.Outputs) {
	v.log.Info(
		outputs.String(),
		"type", json.MessageOutputs,
		"outputs", outputs,
	)
}

// Output is designed for supporting command.WrappedUi
func (v *JSONView) Output(message string) {
	v.log.Info(message, "type", "output")
}

// Info is designed for supporting command.WrappedUi
func (v *JSONView) Info(message string) {
	v.log.Info(message)
}

// Warn is designed for supporting command.WrappedUi
func (v *JSONView) Warn(message string) {
	v.log.Warn(message)
}

// Error is designed for supporting command.WrappedUi
func (v *JSONView) Error(message string) {
	v.log.Error(message)
}
