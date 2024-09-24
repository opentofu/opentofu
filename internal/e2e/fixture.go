package e2e

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

type Fixture struct {
	requireNetwork bool
	files          map[string]string
	includes       map[string]string
	fixtures       map[string]*Fixture
}

func NewFixture() *Fixture {
	return &Fixture{
		requireNetwork: false,
		files:          make(map[string]string),
		includes:       make(map[string]string),
	}
}

func (f *Fixture) RequireNetwork() *Fixture {
	f.requireNetwork = true
	return f
}

func (f *Fixture) File(path string, contents string) *Fixture {
	f.files[path] = contents
	return f
}

func (f *Fixture) Include(source string, relative string) *Fixture {
	f.includes[relative] = source
	return f
}

func (f *Fixture) With(child *Fixture, relative string) *Fixture {
	f.fixtures[relative] = child
	return f
}

func (f *Fixture) copyTo(workDir string) error {
	var err error

	// Make sure we are dealing with absolute paths
	workDir, err = filepath.Abs(workDir)
	if err != nil {
		return err
	}

	for relative, sub := range f.fixtures {
		err = sub.copyTo(filepath.Join(workDir, relative))
		if err != nil {
			return err
		}
	}

	for relative, source := range f.includes {
		source, err = filepath.Abs(source)
		if err != nil {
			return err
		}
		target := filepath.Join(workDir, relative)

		info, err := os.Stat(source)
		if err != nil {
			return err
		}
		if info.IsDir() {
			err = copyDir(source, target)
		} else {
			err = copyFile(source, target)
		}
		if err != nil {
			return err
		}
	}

	for path, contents := range f.files {
		target := filepath.Join(workDir, path)

		err = os.MkdirAll(filepath.Dir(target), 0700)
		if err != nil {
			return err
		}

		err = os.WriteFile(target, []byte(contents), 0600)
		if err != nil {
			return err
		}
	}

	return nil
}
func copyDir(source string, target string) error {
	return filepath.WalkDir(source, func(sourcePath string, info fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(source, sourcePath)
		if err != nil {
			return err
		}

		targetPath := filepath.Join(target, relPath)

		if info.IsDir() {
			return os.Mkdir(targetPath, 0700)
		}
		return copyFile(sourcePath, targetPath)
	})
}
func copyFile(sourcePath string, targetPath string) error {
	src, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer src.Close()

	dst, err := os.OpenFile(targetPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer dst.Close()

	_, err = io.Copy(dst, src)
	return err

}
