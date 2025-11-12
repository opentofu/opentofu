package main

import (
	"context"
	"io"
	"log"
	"os"

	"github.com/opentofu/opentofu/internal/encryption"
	"github.com/opentofu/opentofu/internal/states/statemgr"
)

// Attempt to open and lock a tofu state file.
// Lock failure exits with 0 and writes "lock failed" to stderr.
func main() {
	if len(os.Args) != 2 {
		log.Fatal(os.Args[0], "statefile")
	}

	s := statemgr.NewFilesystem(os.Args[1], encryption.StateEncryptionDisabled())

	info := statemgr.NewLockInfo()
	info.Operation = "test"
	info.Info = "state locker"

	_, err := s.Lock(context.Background(), info)
	if err != nil {
		io.WriteString(os.Stderr, "lock failed")
	}
}
