# OpenTofu command completion for Fish shell

function __tofu_needs_command
  set -l cmd (commandline -opc)
  if test (count $cmd) -eq 1
    return 0
  end
  return 1
end

function __tofu_using_command
  set -l cmd (commandline -opc)
  if test (count $cmd) -gt 1
    if test $argv[1] = $cmd[2]
      return 0
    end
  end
  return 1
end

# Top-level commands
complete -f -c tofu -n '__tofu_needs_command' -a 'apply' -d 'Build or change infrastructure'
complete -f -c tofu -n '__tofu_needs_command' -a 'console' -d 'Interactive console for OpenTofu interpolations'
complete -f -c tofu -n '__tofu_needs_command' -a 'destroy' -d 'Destroy OpenTofu-managed infrastructure'
complete -f -c tofu -n '__tofu_needs_command' -a 'env' -d 'Workspace management (legacy)'
complete -f -c tofu -n '__tofu_needs_command' -a 'fmt' -d 'Reformat your configuration in the standard style'
complete -f -c tofu -n '__tofu_needs_command' -a 'force-unlock' -d 'Release a stuck lock on the current workspace'
complete -f -c tofu -n '__tofu_needs_command' -a 'get' -d 'Install or upgrade OpenTofu modules'
complete -f -c tofu -n '__tofu_needs_command' -a 'graph' -d 'Create a visual graph of OpenTofu resources'
complete -f -c tofu -n '__tofu_needs_command' -a 'import' -d 'Import existing infrastructure into OpenTofu'
complete -f -c tofu -n '__tofu_needs_command' -a 'init' -d 'Initialize a new or existing OpenTofu working directory'
complete -f -c tofu -n '__tofu_needs_command' -a 'login' -d 'Obtain and save credentials for a remote host'
complete -f -c tofu -n '__tofu_needs_command' -a 'logout' -d 'Remove locally-stored credentials for a remote host'
complete -f -c tofu -n '__tofu_needs_command' -a 'metadata' -d 'Metadata related commands'
complete -f -c tofu -n '__tofu_needs_command' -a 'output' -d 'Show output values from your root module'
complete -f -c tofu -n '__tofu_needs_command' -a 'plan' -d 'Show changes required by the current configuration'
complete -f -c tofu -n '__tofu_needs_command' -a 'providers' -d 'Show the providers required for this configuration'
complete -f -c tofu -n '__tofu_needs_command' -a 'refresh' -d 'Update local state file against real resources'
complete -f -c tofu -n '__tofu_needs_command' -a 'show' -d 'Show the current state or a saved plan'
complete -f -c tofu -n '__tofu_needs_command' -a 'state' -d 'Advanced state management'
complete -f -c tofu -n '__tofu_needs_command' -a 'taint' -d 'Mark a resource instance as not fully functional'
complete -f -c tofu -n '__tofu_needs_command' -a 'test' -d 'Execute integration tests for OpenTofu modules'
complete -f -c tofu -n '__tofu_needs_command' -a 'untaint' -d 'Remove the tainted status from a resource instance'
complete -f -c tofu -n '__tofu_needs_command' -a 'validate' -d 'Check whether the configuration is valid'
complete -f -c tofu -n '__tofu_needs_command' -a 'version' -d 'Show the current OpenTofu version'
complete -f -c tofu -n '__tofu_needs_command' -a 'workspace' -d 'Workspace management'

# Common options for all commands
complete -f -c tofu -n '__fish_use_subcommand' -l 'help' -d 'Show help output'
complete -f -c tofu -n '__fish_use_subcommand' -l 'version' -d 'Show version'
complete -f -c tofu -n '__fish_use_subcommand' -o 'chdir' -d 'Change the working directory' -r -a '(__fish_complete_directories)'

# Workspace subcommands
complete -f -c tofu -n '__tofu_using_command workspace' -a 'list' -d 'List workspaces'
complete -f -c tofu -n '__tofu_using_command workspace' -a 'select' -d 'Select a workspace'
complete -f -c tofu -n '__tofu_using_command workspace' -a 'new' -d 'Create a new workspace'
complete -f -c tofu -n '__tofu_using_command workspace' -a 'delete' -d 'Delete a workspace'
complete -f -c tofu -n '__tofu_using_command workspace' -a 'show' -d 'Show the name of the current workspace'

# State subcommands
complete -f -c tofu -n '__tofu_using_command state' -a 'list' -d 'List resources in the state'
complete -f -c tofu -n '__tofu_using_command state' -a 'ls' -d 'List resources in the state (alias of list)'
complete -f -c tofu -n '__tofu_using_command state' -a 'mv' -d 'Move an item in the state'
complete -f -c tofu -n '__tofu_using_command state' -a 'move' -d 'Move an item in the state (alias of mv)'
complete -f -c tofu -n '__tofu_using_command state' -a 'pull' -d 'Pull current state and output to stdout'
complete -f -c tofu -n '__tofu_using_command state' -a 'push' -d 'Update remote state from a local state file'
complete -f -c tofu -n '__tofu_using_command state' -a 'rm' -d 'Remove instances from the state'
complete -f -c tofu -n '__tofu_using_command state' -a 'remove' -d 'Remove instances from the state (alias of rm)'
complete -f -c tofu -n '__tofu_using_command state' -a 'show' -d 'Show a resource in the state'
complete -f -c tofu -n '__tofu_using_command state' -a 'replace-provider' -d 'Replace provider in the state'

# Providers subcommands
complete -f -c tofu -n '__tofu_using_command providers' -a 'lock' -d 'Write provider dependency locks to file'
complete -f -c tofu -n '__tofu_using_command providers' -a 'mirror' -d 'Save local copies of all required provider plugins'
complete -f -c tofu -n '__tofu_using_command providers' -a 'schema' -d 'Show schemas for the providers used in the configuration'

# Metadata subcommands
complete -f -c tofu -n '__tofu_using_command metadata' -a 'functions' -d 'Show function signatures'

# Apply options
complete -f -c tofu -n '__tofu_using_command apply' -l 'auto-approve' -d 'Skip interactive approval of plan'
complete -f -c tofu -n '__tofu_using_command apply' -l 'backup' -d 'Path to backup the existing state file' -r
complete -f -c tofu -n '__tofu_using_command apply' -l 'compact-warnings' -d 'Show compact warning messages'
complete -f -c tofu -n '__tofu_using_command apply' -l 'lock' -d 'Lock the state file'
complete -f -c tofu -n '__tofu_using_command apply' -l 'lock-timeout' -d 'Duration to wait for a state lock' -r
complete -f -c tofu -n '__tofu_using_command apply' -l 'input' -d 'Ask for input for variables'
complete -f -c tofu -n '__tofu_using_command apply' -l 'no-color' -d 'If specified, output will not contain color'
complete -f -c tofu -n '__tofu_using_command apply' -l 'parallelism' -d 'Number of parallel resource operations' -r
complete -f -c tofu -n '__tofu_using_command apply' -l 'state' -d 'Path to read and save state' -r
complete -f -c tofu -n '__tofu_using_command apply' -l 'state-out' -d 'Path to write updated state file' -r
complete -f -c tofu -n '__tofu_using_command apply' -l 'var' -d 'Input variables' -r
complete -f -c tofu -n '__tofu_using_command apply' -l 'var-file' -d 'Variable file' -r

# Plan options
complete -f -c tofu -n '__tofu_using_command plan' -l 'compact-warnings' -d 'Show compact warning messages'
complete -f -c tofu -n '__tofu_using_command plan' -l 'destroy' -d 'Select the destroy planning mode'
complete -f -c tofu -n '__tofu_using_command plan' -l 'detailed-exitcode' -d 'Return detailed exit codes'
complete -f -c tofu -n '__tofu_using_command plan' -l 'input' -d 'Ask for input for variables'
complete -f -c tofu -n '__tofu_using_command plan' -l 'lock' -d 'Lock the state file'
complete -f -c tofu -n '__tofu_using_command plan' -l 'lock-timeout' -d 'Duration to wait for a state lock' -r
complete -f -c tofu -n '__tofu_using_command plan' -l 'no-color' -d 'If specified, output will not contain color'
complete -f -c tofu -n '__tofu_using_command plan' -l 'out' -d 'Write a plan file to the given path' -r
complete -f -c tofu -n '__tofu_using_command plan' -l 'parallelism' -d 'Number of parallel resource operations' -r
complete -f -c tofu -n '__tofu_using_command plan' -l 'state' -d 'Path to read and save state' -r
complete -f -c tofu -n '__tofu_using_command plan' -l 'var' -d 'Input variables' -r
complete -f -c tofu -n '__tofu_using_command plan' -l 'var-file' -d 'Variable file' -r

# Init options
complete -f -c tofu -n '__tofu_using_command init' -l 'backend' -d 'Configure the backend for this configuration'
complete -f -c tofu -n '__tofu_using_command init' -l 'backend-config' -d 'Backend configuration' -r
complete -f -c tofu -n '__tofu_using_command init' -l 'force-copy' -d 'Suppress prompts about copying state data'
complete -f -c tofu -n '__tofu_using_command init' -l 'from-module' -d 'Copy the contents of the given module into the target directory' -r
complete -f -c tofu -n '__tofu_using_command init' -l 'get' -d 'Download any modules for this configuration'
complete -f -c tofu -n '__tofu_using_command init' -l 'input' -d 'Ask for input for variables'
complete -f -c tofu -n '__tofu_using_command init' -l 'lock' -d 'Lock the state file'
complete -f -c tofu -n '__tofu_using_command init' -l 'lock-timeout' -d 'Duration to wait for a state lock' -r
complete -f -c tofu -n '__tofu_using_command init' -l 'no-color' -d 'If specified, output will not contain color'
complete -f -c tofu -n '__tofu_using_command init' -l 'upgrade' -d 'Install the latest version allowed within version constraints'

# Fmt options
complete -f -c tofu -n '__tofu_using_command fmt' -l 'list' -d 'List files whose formatting differs'
complete -f -c tofu -n '__tofu_using_command fmt' -l 'write' -d 'Write result to source files'
complete -f -c tofu -n '__tofu_using_command fmt' -l 'diff' -d 'Display diffs of formatting changes'
complete -f -c tofu -n '__tofu_using_command fmt' -l 'check' -d 'Check if the input is formatted'
complete -f -c tofu -n '__tofu_using_command fmt' -l 'no-color' -d 'If specified, output will not contain color'
complete -f -c tofu -n '__tofu_using_command fmt' -l 'recursive' -d 'Also process files in subdirectories'

# Validate options
complete -f -c tofu -n '__tofu_using_command validate' -l 'json' -d 'Produce output in JSON format'
complete -f -c tofu -n '__tofu_using_command validate' -l 'no-color' -d 'If specified, output will not contain color'

# Output options
complete -f -c tofu -n '__tofu_using_command output' -l 'state' -d 'Path to read state' -r
complete -f -c tofu -n '__tofu_using_command output' -l 'no-color' -d 'If specified, output will not contain color'
complete -f -c tofu -n '__tofu_using_command output' -l 'json' -d 'Print output in JSON format'

# Common flags for commands that can use variable files
function __tofu_complete_tfvars_files
  for i in *.tfvars
    echo $i
  end
end

# Add tfvars files completion to commands that use variable files
complete -f -c tofu -n '__tofu_using_command apply' -l 'var-file' -d 'Variable file' -r -a '(__tofu_complete_tfvars_files)'
complete -f -c tofu -n '__tofu_using_command plan' -l 'var-file' -d 'Variable file' -r -a '(__tofu_complete_tfvars_files)'
complete -f -c tofu -n '__tofu_using_command destroy' -l 'var-file' -d 'Variable file' -r -a '(__tofu_complete_tfvars_files)'
