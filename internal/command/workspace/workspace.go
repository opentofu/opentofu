package workspace

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/opentofu/opentofu/internal/backend"
	"github.com/opentofu/opentofu/internal/backend/local"
	"github.com/opentofu/opentofu/internal/cloud"
	"github.com/opentofu/opentofu/internal/command/arguments"
	"github.com/opentofu/opentofu/internal/command/workdir"
	"github.com/opentofu/opentofu/internal/tofu"
)

// WorkspaceNameEnvVar is the name of the environment variable that can be used
// to set the name of the OpenTofu workspace, overriding the workspace chosen
// by `tofu workspace select`.
//
// Note that this environment variable is ignored by `tofu workspace new`
// and `tofu workspace delete`.
const WorkspaceNameEnvVar = "TF_WORKSPACE"

var errInvalidWorkspaceNameEnvVar = fmt.Errorf("Invalid workspace name set using %s", WorkspaceNameEnvVar)

type Workspace struct {
	*workdir.Dir
	Input   arguments.Input
	UIInput tofu.UIInput

	Test bool // TODO andrei this needs to be replaced when configured properly
}

// Workspace returns the name of the currently configured workspace, corresponding
// to the desired named state.
func (m *Workspace) Workspace(ctx context.Context) (string, error) {
	current, overridden := m.WorkspaceOverridden(ctx)
	if overridden && !ValidWorkspaceName(current) {
		return "", errInvalidWorkspaceNameEnvVar
	}
	return current, nil
}

// WorkspaceOverridden returns the name of the currently configured workspace,
// corresponding to the desired named state, as well as a bool saying whether
// this was set via the TF_WORKSPACE environment variable.
func (m *Workspace) WorkspaceOverridden(_ context.Context) (string, bool) {
	if envVar := os.Getenv(WorkspaceNameEnvVar); envVar != "" {
		return envVar, true
	}

	envData, err := os.ReadFile(filepath.Join(m.DataDir(), local.DefaultWorkspaceFile))
	current := string(bytes.TrimSpace(envData))
	if current == "" {
		current = backend.DefaultStateName
	}

	if err != nil && !os.IsNotExist(err) {
		// always return the default if we can't get a workspace name
		log.Printf("[ERROR] failed to read current workspace: %s", err)
	}

	return current, false
}

// SetWorkspace saves the given name as the current workspace in the local
// filesystem.
func (m *Workspace) SetWorkspace(name string) error {
	err := os.MkdirAll(m.DataDir(), 0755)
	if err != nil {
		return err
	}

	err = os.WriteFile(filepath.Join(m.DataDir(), local.DefaultWorkspaceFile), []byte(name), 0644)
	if err != nil {
		return err
	}
	return nil
}

// ValidWorkspaceName returns true is this name is valid to use as a workspace name.
// Since most named states are accessed via a filesystem path or URL, check if
// escaping the name would be required.
func ValidWorkspaceName(name string) bool {
	return name == url.PathEscape(name)
}

// SelectWorkspace gets a list of existing workspaces and then checks
// if the currently selected workspace is valid. If not, it will ask
// the user to select a workspace from the list.
func (m *Workspace) SelectWorkspace(ctx context.Context, b backend.Backend) error {
	workspaces, err := b.Workspaces(ctx)
	if err == backend.ErrWorkspacesNotSupported {
		return nil
	}
	if err != nil {
		return fmt.Errorf("Failed to get existing workspaces: %w", err)
	}
	if len(workspaces) == 0 {
		if c, ok := b.(*cloud.Cloud); ok && m.Input.Input(m.Test) {
			// len is always 1 if using Name; 0 means we're using Tags and there
			// aren't any matching workspaces. Which might be normal and fine, so
			// let's just ask:
			name, err := m.UIInput.Input(context.Background(), &tofu.InputOpts{
				Id:          "create-workspace",
				Query:       "\n[reset][bold][yellow]No workspaces found.[reset]",
				Description: fmt.Sprintf(inputCloudInitCreateWorkspace, strings.Join(c.WorkspaceMapping.Tags, ", ")),
			})
			if err != nil {
				return fmt.Errorf("Couldn't create initial workspace: %w", err)
			}
			name = strings.TrimSpace(name)
			if name == "" {
				return fmt.Errorf("Couldn't create initial workspace: no name provided")
			}
			log.Printf("[TRACE] Meta.selectWorkspace: selecting the new TFC workspace requested by the user (%s)", name)
			return m.SetWorkspace(name)
		} else {
			return fmt.Errorf("%s", strings.TrimSpace(errBackendNoExistingWorkspaces))
		}
	}

	// Get the currently selected workspace.
	workspace, err := m.Workspace(ctx)
	if err != nil {
		return err
	}

	// Check if any of the existing workspaces matches the selected
	// workspace and create a numbered list of existing workspaces.
	var list strings.Builder
	for i, w := range workspaces {
		if w == workspace {
			log.Printf("[TRACE] Meta.selectWorkspace: the currently selected workspace is present in the configured backend (%s)", workspace)
			return nil
		}
		fmt.Fprintf(&list, "%d. %s\n", i+1, w)
	}

	// If the backend only has a single workspace, select that as the current workspace
	if len(workspaces) == 1 {
		log.Printf("[TRACE] Meta.selectWorkspace: automatically selecting the single workspace provided by the backend (%s)", workspaces[0])
		return m.SetWorkspace(workspaces[0])
	}

	if !m.Input.Input(m.Test) {
		return fmt.Errorf("Currently selected workspace %q does not exist", workspace)
	}

	// Otherwise, ask the user to select a workspace from the list of existing workspaces.
	v, err := m.UIInput.Input(context.Background(), &tofu.InputOpts{
		Id: "select-workspace",
		Query: fmt.Sprintf(
			"\n[reset][bold][yellow]The currently selected workspace (%s) does not exist.[reset]",
			workspace),
		Description: fmt.Sprintf(
			strings.TrimSpace(inputBackendSelectWorkspace), list.String()),
	})
	if err != nil {
		return fmt.Errorf("Failed to select workspace: %w", err)
	}

	idx, err := strconv.Atoi(v)
	if err != nil || (idx < 1 || idx > len(workspaces)) {
		return fmt.Errorf("Failed to select workspace: input not a valid number")
	}

	workspace = workspaces[idx-1]
	log.Printf("[TRACE] Meta.selectWorkspace: setting the current workspace according to user selection (%s)", workspace)
	return m.SetWorkspace(workspace)
}

func ConfiguredWorkspace(in *Workspace, input arguments.Input, uiinput tofu.UIInput) *Workspace {
	return &Workspace{
		Dir:     in.Dir,
		Input:   input,
		UIInput: uiinput,
	}
}

const errBackendNoExistingWorkspaces = `
No existing workspaces.

Use the "tofu workspace" command to create and select a new workspace.
If the backend already contains existing workspaces, you may need to update
the backend configuration.
`
const inputCloudInitCreateWorkspace = `
There are no workspaces with the configured tags (%s)
in your cloud backend organization. To finish initializing, OpenTofu needs at
least one workspace available.

OpenTofu can create a properly tagged workspace for you now. Please enter a
name to create a new cloud backend workspace.
`

const inputBackendSelectWorkspace = `
This is expected behavior when the selected workspace did not have an
existing non-empty state. Please enter a number to select a workspace:

%s
`
