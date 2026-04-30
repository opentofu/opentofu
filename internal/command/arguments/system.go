package arguments

type System struct {
	// RunningInAutomation indicates that commands are being run by an
	// automated system rather than directly at a command prompt.
	//
	// This is a hint to various command routines that it may be confusing
	// to print out messages that suggest running specific follow-up
	// commands, since the user consuming the output will not be
	// in a position to run such commands.
	//
	// The intended use-case of this flag is when OpenTofu is running in
	// some sort of workflow orchestration tool which is abstracting away
	// the specific commands being run.
	RunningInAutomation bool

	// CLIConfigDir is the directory from which CLI configuration files were
	// read by the caller and the directory where any changes to CLI
	// configuration files by commands should be made.
	//
	// If this is empty then no configuration directory is available and
	// commands which require one cannot proceed.
	CLIConfigDir string

	// PluginCacheDir, if non-empty, enables caching of downloaded plugins
	// into the given directory.
	PluginCacheDir string

	// GlobalPluginDirs contains additional paths to search for plugins
	GlobalPluginDirs []string

	// AllowExperimentalFeatures controls whether the current build of OpenTofu
	// has experimental features enabled.
	//
	// In normal code this would be set by package main only in builds
	// explicitly marked as being alpha releases or development snapshots,
	// making experimental features unavailable otherwise. Test code may
	// choose to set this if it needs to exercise experimental features.
	//
	// Some experiments predated the addition of this setting, and may
	// therefore still be available even if this flag is false. Our intent
	// is that all/most _future_ experiments will be unavailable unless this
	// flag is set, to reinforce that experiments are not for production use.
	AllowExperimentalFeatures bool
}
