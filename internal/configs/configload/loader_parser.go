package configload

import (
	"github.com/hashicorp/hcl/v2"
	"github.com/opentofu/opentofu/internal/configs"
)

func (l *loader) LoadConfigDirUneval(path string, load configs.SelectiveLoader) (*configs.Module, hcl.Diagnostics) {
	return l.LoadConfigDirUneval(path, load)
}

func (l *loader) LoadConfigDir(path string, call configs.StaticModuleCall) (*configs.Module, hcl.Diagnostics) {
	return l.parser.LoadConfigDir(path, call)
}

func (l *loader) LoadHCLFile(path string) (hcl.Body, hcl.Diagnostics) {
	return l.parser.LoadHCLFile(path)
}

func (l *loader) LoadConfigDirSelective(path string, call configs.StaticModuleCall, load configs.SelectiveLoader) (*configs.Module, hcl.Diagnostics) {
	return l.parser.LoadConfigDirSelective(path, call, load)
}

func (l *loader) LoadConfigDirWithTests(path string, testDirectory string, call configs.StaticModuleCall) (*configs.Module, hcl.Diagnostics) {
	return l.parser.LoadConfigDirWithTests(path, testDirectory, call)
}

func (l *loader) ForceFileSource(filename string, src []byte) {
	l.parser.ForceFileSource(filename, src)
}
