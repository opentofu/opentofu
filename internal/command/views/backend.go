// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package views

import (
	"fmt"

	"github.com/opentofu/opentofu/internal/tfdiags"
)

type Backend interface {
	Basic
	InitializingBackend()
	InitializingCloudBackend()
	BackendTypeAlias(backendType, canonType string)
	MigratingFromCloudToLocal()
	UnconfiguringBackendType(backendType string)
	BackendTypeUnset(backendType string)
	BackendTypeSet(backendType string)
	CloudBackendUpdated()
	MigratingLocalTypeToCloud(fromBackendType string)
	MigratingCloudToLocalType(toBackendType string)
	BackendTypeChanged(oldBackendType string, newBackendType string)
	BackendReconfigured()
	MigrationCompleted(workspaces []string, currentWs string)

	StateLocker() StateLocker
}

// NewBackendHuman returns a new Backend instance that will print in human format.
// This particular function is meant to be used only in special cases, where the
// Backend view cannot be acquired from a command related view (eg: Apply.Backend).
// At the moment of writing this comment, this function is meant to be used only to
// create this view in cases where it is not initialised correctly, which are paths
// that are only reachable from incomplete configured tests.
func NewBackendHuman(view *View) Backend {
	return &BackendHuman{view: view}
}

type BackendMulti []Backend

var _ Backend = (BackendMulti)(nil)

func (m BackendMulti) Diagnostics(diags tfdiags.Diagnostics) {
	for _, o := range m {
		o.Diagnostics(diags)
	}
}
func (m BackendMulti) InitializingBackend() {
	for _, o := range m {
		o.InitializingBackend()
	}
}

func (m BackendMulti) InitializingCloudBackend() {
	for _, o := range m {
		o.InitializingCloudBackend()
	}
}

func (m BackendMulti) BackendTypeAlias(backendType, canonType string) {
	for _, o := range m {
		o.BackendTypeAlias(backendType, canonType)
	}
}

func (m BackendMulti) MigratingFromCloudToLocal() {
	for _, v := range m {
		v.MigratingFromCloudToLocal()
	}
}

func (m BackendMulti) UnconfiguringBackendType(backendType string) {
	for _, v := range m {
		v.UnconfiguringBackendType(backendType)
	}
}

func (m BackendMulti) BackendTypeUnset(backendType string) {
	for _, v := range m {
		v.BackendTypeUnset(backendType)
	}
}

func (m BackendMulti) BackendTypeSet(backendType string) {
	for _, v := range m {
		v.BackendTypeSet(backendType)
	}
}

func (m BackendMulti) CloudBackendUpdated() {
	for _, v := range m {
		v.CloudBackendUpdated()
	}
}

func (m BackendMulti) MigratingLocalTypeToCloud(fromBackendType string) {
	for _, v := range m {
		v.MigratingLocalTypeToCloud(fromBackendType)
	}
}

func (m BackendMulti) MigratingCloudToLocalType(toBackendType string) {
	for _, v := range m {
		v.MigratingCloudToLocalType(toBackendType)
	}
}

func (m BackendMulti) BackendTypeChanged(oldBackendType string, newBackendType string) {
	for _, v := range m {
		v.BackendTypeChanged(oldBackendType, newBackendType)
	}
}

func (m BackendMulti) BackendReconfigured() {
	for _, v := range m {
		v.BackendReconfigured()
	}
}

func (m BackendMulti) MigrationCompleted(workspaces []string, currentWs string) {
	for _, v := range m {
		v.MigrationCompleted(workspaces, currentWs)
	}
}

func (m BackendMulti) StateLocker() StateLocker {
	ret := make([]StateLocker, len(m))
	for i, v := range m {
		ret[i] = v.StateLocker()
	}
	return StateLockerMulti(ret)
}

type BackendHuman struct {
	view *View
}

var _ Backend = (*BackendHuman)(nil)

func (v *BackendHuman) Diagnostics(diags tfdiags.Diagnostics) {
	v.view.Diagnostics(diags)
}

func (v *BackendHuman) InitializingBackend() {
	_, _ = v.view.streams.Println(v.view.colorize.Color("\n[reset][bold]Initializing the backend..."))
}

func (v *BackendHuman) InitializingCloudBackend() {
	_, _ = v.view.streams.Println(v.view.colorize.Color("\n[reset][bold]Initializing cloud backend..."))
}

func (v *BackendHuman) BackendTypeAlias(backendType, canonType string) {
	_, _ = v.view.streams.Println(fmt.Sprintf("- %q is an alias for backend type %q", backendType, canonType))
}

func (v *BackendHuman) MigratingFromCloudToLocal() {
	_, _ = v.view.streams.Println("Migrating from cloud backend to local state.")
}

func (v *BackendHuman) UnconfiguringBackendType(backendType string) {
	_, _ = v.view.streams.Printf("OpenTofu has detected you're unconfiguring your previously set %q backend.\n", backendType)
}

func (v *BackendHuman) BackendTypeUnset(backendType string) {
	msg := fmt.Sprintf("[reset][green]\n\nSuccessfully unset the backend %q. OpenTofu will now operate locally.", backendType)
	_, _ = v.view.streams.Println(v.view.colorize.Color(msg))
}

func (v *BackendHuman) BackendTypeSet(backendType string) {
	msg := fmt.Sprintf("[reset][green]\nSuccessfully configured the backend %q! OpenTofu will automatically\nuse this backend unless the backend configuration changes.", backendType)
	_, _ = v.view.streams.Println(v.view.colorize.Color(msg))
}

func (v *BackendHuman) CloudBackendUpdated() {
	_, _ = v.view.streams.Println("Cloud backend configuration has changed.")
}

func (v *BackendHuman) MigratingLocalTypeToCloud(fromBackendType string) {
	_, _ = v.view.streams.Println(fmt.Sprintf("Migrating from backend %q to cloud backend.", fromBackendType))
}

func (v *BackendHuman) MigratingCloudToLocalType(toBackendType string) {
	_, _ = v.view.streams.Println(fmt.Sprintf("Migrating from cloud backend to backend %q.", toBackendType))
}

func (v *BackendHuman) BackendTypeChanged(oldBackendType string, newBackendType string) {
	msg := fmt.Sprintf("[reset]\nOpenTofu detected that the backend type changed from %q to %q.", oldBackendType, newBackendType)
	_, _ = v.view.streams.Println(v.view.colorize.Color(msg))
}

func (v *BackendHuman) BackendReconfigured() {
	const outputBackendReconfigure = `[reset][bold]Backend configuration changed![reset]

OpenTofu has detected that the configuration specified for the backend
has changed. OpenTofu will now check for existing state in the backends.`
	_, _ = v.view.streams.Println(v.view.colorize.Color(outputBackendReconfigure))
}

func (v *BackendHuman) MigrationCompleted(workspaces []string, currentWs string) {
	const msg = "[reset][bold]Migration complete! Your workspaces are as follows:[reset]"
	_, _ = v.view.streams.Println(v.view.colorize.Color(msg))
	buf := buildWorkspacesList(workspaces, currentWs)
	_, _ = v.view.streams.Println(buf.String())
}

func (v *BackendHuman) StateLocker() StateLocker {
	return &StateLockerHuman{
		view: v.view,
	}
}

type BackendJSON struct {
	view *JSONView
}

var _ Backend = (*BackendJSON)(nil)

func (v *BackendJSON) Diagnostics(diags tfdiags.Diagnostics) {
	v.view.Diagnostics(diags)
}

func (v *BackendJSON) InitializingBackend() {
	v.view.Info("Initializing the backend...")
}

func (v *BackendJSON) InitializingCloudBackend() {
	v.view.Info("Initializing cloud backend...")
}

func (v *BackendJSON) BackendTypeAlias(backendType, canonType string) {
	v.view.Info(fmt.Sprintf("%q is an alias for backend type %q", backendType, canonType))
}

func (v *BackendJSON) MigratingFromCloudToLocal() {
	v.view.Info("Migrating from cloud backend to local state.")
}

func (v *BackendJSON) UnconfiguringBackendType(backendType string) {
	v.view.Info(fmt.Sprintf("OpenTofu has detected you're unconfiguring your previously set %q backend", backendType))
}

func (v *BackendJSON) BackendTypeUnset(backendType string) {
	v.view.Info(fmt.Sprintf("Successfully unset the backend %q. OpenTofu will now operate locally", backendType))
}

func (v *BackendJSON) BackendTypeSet(backendType string) {
	msg := fmt.Sprintf("Successfully configured the backend %q! OpenTofu will automatically use this backend unless the backend configuration changes", backendType)
	v.view.Info(msg)
}

func (v *BackendJSON) CloudBackendUpdated() {
	v.view.Info("Cloud backend configuration has changed")
}

func (v *BackendJSON) MigratingLocalTypeToCloud(fromBackendType string) {
	v.view.Info(fmt.Sprintf("Migrating from backend %q to cloud backend", fromBackendType))
}

func (v *BackendJSON) MigratingCloudToLocalType(toBackendType string) {
	v.view.Info(fmt.Sprintf("Migrating from cloud backend to backend %q", toBackendType))
}

func (v *BackendJSON) BackendTypeChanged(oldBackendType string, newBackendType string) {
	msg := fmt.Sprintf("OpenTofu detected that the backend type changed from %q to %q.", oldBackendType, newBackendType)
	v.view.Info(msg)
}

func (v *BackendJSON) BackendReconfigured() {
	const msg = "Backend configuration changed! OpenTofu has detected that the configuration specified for the backend has changed. OpenTofu will now check for existing state in the backends"
	v.view.Info(msg)
}

func (v *BackendJSON) MigrationCompleted(workspaces []string, currentWs string) {
	v.view.log.Info("Migration complete", "workspaces", workspaces, "current_workspace", currentWs)
}

func (v *BackendJSON) StateLocker() StateLocker {
	return &StateLockerJSON{
		view: v.view,
	}
}
