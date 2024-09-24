package e2e

import (
	"os"
	"path/filepath"
	"testing"
)

/*


 */

type Scenario struct {
	t       *testing.T
	workDir string
}

func NewScenario(f *Fixture, t *testing.T) *Scenario {
	if f.requireNetwork && os.Getenv("TOFUTEST_NETWORK") == "" {
		t.Skip()
	}

	workDir := os.Getenv("TOFUTEST_WORKDIR")
	if workDir != "" {
		workDir = filepath.Join(workDir, t.Name())
	} else {
		workDir = t.TempDir()
	}

	err := f.copyTo(workDir)
	if err != nil {
		t.Fatal(err)
	}

	return &Scenario{
		t:       t,
		workDir: workDir,
	}
}

func (s *Scenario) Tofu(args ...string) *Tofu {
	return &Tofu{
		args:     args,
		env:      make(map[string]string),
		scenario: s,
		dir:      s.workDir,
		t:        s.t,
	}
}

func (s *Scenario) FileContents(path string) []byte {
	data, err := os.ReadFile(filepath.Join(s.workDir, path))
	if err != nil {
		s.t.Fatal(err)
	}
	return data
}

func (s *Scenario) WriteFile(path string, data []byte) {
	err := os.WriteFile(filepath.Join(s.workDir, path), data, 0600)
	if err != nil {
		s.t.Fatal(err)
	}
}

// TODO files/plans/state...
