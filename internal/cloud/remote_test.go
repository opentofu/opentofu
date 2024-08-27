// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package cloud

import (
	"flag"
	"os"
	"testing"
	"time"

	_ "github.com/terramate-io/opentofulib/internal/logging"
)

func TestMain(m *testing.M) {
	flag.Parse()

	// Make sure TF_FORCE_LOCAL_BACKEND is unset
	os.Unsetenv("TF_FORCE_LOCAL_BACKEND")

	// Reduce delays to make tests run faster
	backoffMin = 1.0
	backoffMax = 1.0
	planConfigurationVersionsPollInterval = 1 * time.Millisecond
	runPollInterval = 1 * time.Millisecond

	os.Exit(m.Run())
}
