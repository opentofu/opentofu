// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package views

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

func TestBackendRemoteViews(t *testing.T) {
	tests := map[string]struct {
		viewCall   func(view BackendRemote)
		wantStdout string
		wantStderr string
	}{
		"output without color": {
			viewCall: func(view BackendRemote) {
				view.Output("[red]hello world", false)
			},
			wantStdout: `[red]hello world
`,
		},
		"output with color": {
			viewCall: func(view BackendRemote) {
				view.Output("[red]hello world", true)
			},
			// color marks are replaced with nothing since coloring is disabled
			wantStdout: `hello world
`,
		},
		"runWarning empty": {
			viewCall: func(view BackendRemote) {
				view.RunWarning("")
			},
			wantStdout: "",
		},
		"runWarning with message": {
			viewCall: func(view BackendRemote) {
				view.RunWarning("something went wrong")
			},
			wantStdout: `Warning: something went wrong

`,
		},
		"operationCancelled": {
			viewCall: func(view BackendRemote) {
				view.OperationCancelled()
			},
			wantStdout: `The remote operation was successfully cancelled.
`,
		},
		"operationNotCancelled": {
			viewCall: func(view BackendRemote) {
				view.OperationNotCancelled()
			},
			wantStdout: `The remote operation was not cancelled.
`,
		},
		"preRefresh": {
			viewCall: func(view BackendRemote) {
				view.PreRefresh()
			},
			wantStdout: `Proceeding with 'tofu apply -refresh-only -auto-approve'.

`,
		},
		"initialRetryError remote": {
			viewCall: func(view BackendRemote) {
				view.InitialRetryError(true)
			},
			wantStdout: `There was an error connecting to the remote backend. Please do not exit
OpenTofu to prevent data loss! Trying to restore the connection...

`,
		},
		"initialRetryError cloud": {
			viewCall: func(view BackendRemote) {
				view.InitialRetryError(false)
			},
			wantStdout: `There was an error connecting to the cloud backend. Please do not exit
OpenTofu to prevent data loss! Trying to restore the connection...

`,
		},
		"repeatedRetryError": {
			viewCall: func(view BackendRemote) {
				view.RepeatedRetryError(5 * time.Second)
			},
			wantStdout: `Still trying to restore the connection... (5s elapsed)
`,
		},
		"unavailableVersionInBackend": {
			viewCall: func(view BackendRemote) {
				view.UnavailableVersionInBackend("1.0.0", "1.1.0")
			},
			wantStdout: `
The local OpenTofu version (1.0.0) is not available in the cloud backend, or your
organization does not have access to it. The new workspace will use 1.1.0. You can
change this later in the workspace settings.
`,
		},
		"applySavedHeader": {
			viewCall: func(view BackendRemote) {
				view.ApplySavedHeader()
			},
			wantStdout: `Running apply in the cloud backend. Output will stream here. Pressing Ctrl-C
will stop streaming the logs, but will not stop the apply running remotely.

Preparing the remote apply...

`,
		},
		"lockTimeoutError": {
			viewCall: func(view BackendRemote) {
				view.LockTimeoutError()
			},
			wantStdout: `Lock timeout exceeded, sending interrupt to cancel the remote operation.

`,
		},
		"remoteWorkspaceInRelativeDirectory": {
			viewCall: func(view BackendRemote) {
				view.RemoteWorkspaceInRelativeDirectory("./configs", "/home/user/project")
			},
			wantStdout: `The remote workspace is configured to work with configuration at
./configs relative to the target repository.

OpenTofu will upload the contents of the following directory,
excluding files or directories as defined by a .terraformignore file
at /home/user/project/.terraformignore (if it is present),
in order to capture the filesystem context the remote workspace expects:
    /home/user/project

`,
		},
		"waitingForCostEstimation without elapsed": {
			viewCall: func(view BackendRemote) {
				view.WaitingForCostEstimation("")
			},
			wantStdout: `Waiting for cost estimate to complete...

`,
		},
		"waitingForCostEstimation with elapsed": {
			viewCall: func(view BackendRemote) {
				view.WaitingForCostEstimation(" (10s elapsed)")
			},
			wantStdout: `Waiting for cost estimate to complete... (10s elapsed)

`,
		},
		"waitingForOperationToStart without elapsed": {
			viewCall: func(view BackendRemote) {
				view.WaitingForOperationToStart("plan", "")
			},
			wantStdout: `Waiting for the plan to start...
`,
		},
		"waitingForOperationToStart with elapsed": {
			viewCall: func(view BackendRemote) {
				view.WaitingForOperationToStart("plan", " (5s elapsed)")
			},
			wantStdout: `Waiting for the %s to start... (5s elapsed)

`,
		},
		"waitingForManuallyLockedWorkspace without elapsed": {
			viewCall: func(view BackendRemote) {
				view.WaitingForTheManuallyLockedWorkspace("")
			},
			wantStdout: `Waiting for the manually locked workspace to be unlocked...
`,
		},
		"waitingForManuallyLockedWorkspace with elapsed": {
			viewCall: func(view BackendRemote) {
				view.WaitingForTheManuallyLockedWorkspace(" (15s elapsed)")
			},
			wantStdout: `Waiting for the manually locked workspace to be unlocked... (15s elapsed)
`,
		},
		"waitingForRuns without elapsed": {
			viewCall: func(view BackendRemote) {
				view.WaitingForRuns(3, "")
			},
			wantStdout: `Waiting for 3 run(s) to finish before being queued...
`,
		},
		"waitingForRuns with elapsed": {
			viewCall: func(view BackendRemote) {
				view.WaitingForRuns(2, " (20s elapsed)")
			},
			wantStdout: `Waiting for 2 run(s) to finish before being queued... (20s elapsed)
`,
		},
		"waitingForQueuedRuns without elapsed": {
			viewCall: func(view BackendRemote) {
				view.WaitingForQueuedRuns(5, "")
			},
			wantStdout: `Waiting for 5 queued run(s) to finish before starting...
`,
		},
		"waitingForQueuedRuns with elapsed": {
			viewCall: func(view BackendRemote) {
				view.WaitingForQueuedRuns(4, " (30s elapsed)")
			},
			wantStdout: `Waiting for 4 queued run(s) to finish before starting... (30s elapsed)
`,
		},
		"operationHeader apply remote": {
			viewCall: func(view BackendRemote) {
				view.OperationHeader(true, true)
			},
			wantStdout: `Running apply in the remote backend. Output will stream here. Pressing Ctrl-C
will cancel the remote apply if it's still pending. If the apply started it
will stop streaming the logs, but will not stop the apply running remotely.

Preparing the remote apply...

`,
		},
		"operationHeader plan remote": {
			viewCall: func(view BackendRemote) {
				view.OperationHeader(false, true)
			},
			wantStdout: `Running plan in the remote backend. Output will stream here. Pressing Ctrl-C
will stop streaming the logs, but will not stop the plan running remotely.

Preparing the remote plan...

`,
		},
		"operationHeader apply cloud": {
			viewCall: func(view BackendRemote) {
				view.OperationHeader(true, false)
			},
			wantStdout: `Running apply in the cloud backend. Output will stream here. Pressing Ctrl-C
will cancel the remote apply if it's still pending. If the apply started it
will stop streaming the logs, but will not stop the apply running remotely.

Preparing the remote apply...

`,
		},
		"operationHeader plan cloud": {
			viewCall: func(view BackendRemote) {
				view.OperationHeader(false, false)
			},
			wantStdout: `Running plan in the cloud backend. Output will stream here. Pressing Ctrl-C
will stop streaming the logs, but will not stop the plan running remotely.

Preparing the remote plan...

`,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			view, done := testView(t)
			v := NewBackendRemote(view)
			tc.viewCall(v)
			output := done(t)
			if diff := cmp.Diff(tc.wantStderr, output.Stderr()); diff != "" {
				t.Errorf("invalid stderr (-want, +got):\n%s", diff)
			}
			if diff := cmp.Diff(tc.wantStdout, output.Stdout()); diff != "" {
				t.Errorf("invalid stdout (-want, +got):\n%s", diff)
			}
		})
	}
}
