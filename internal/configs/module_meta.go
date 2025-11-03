package configs

type ModuleAccessSafety string

const (
	ModuleAccessSafeModule = ModuleAccessSafety("module")
	ModuleAccessSafeTree   = ModuleAccessSafety("tree")
	ModuleAccessUnsafe     = ModuleAccessSafety("unsafe")
)

type ModuleMeta struct {
	Access *ModuleAccessSafety
}

type ModulePackageMeta struct {
	DefaultAccess  *ModuleAccessSafety
	OverrideAccess map[string]*ModuleAccessSafety
}
