// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

import (
	"path/filepath"
	"regexp"
	"sort"
	"testing"

	"github.com/opentofu/opentofu/internal/encryption"
	"github.com/opentofu/opentofu/internal/states/statemgr"
)

// testStateBackups returns the list of backups in order of creation
// (oldest first) in the given directory.
func testStateBackups(t *testing.T, dir string) []string {
	// Find all the backups
	list, err := filepath.Glob(filepath.Join(dir, "*"+DefaultBackupExtension))
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	// Sort them which will put them naturally in the right order
	sort.Strings(list)

	return list
}

func TestStateDefaultBackupExtension(t *testing.T) {
	testCwdTemp(t)

	s, err := (&StateMeta{}).State(t.Context(), encryption.Disabled())
	if err != nil {
		t.Fatal(err)
	}

	backupPath := s.(*statemgr.Filesystem).BackupPath()
	match := regexp.MustCompile(`terraform\.tfstate\.\d+\.backup$`).MatchString
	if !match(backupPath) {
		t.Fatal("Bad backup path:", backupPath)
	}
}
