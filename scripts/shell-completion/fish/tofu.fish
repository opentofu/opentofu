# OpenTofu command completion for Fish shell

function __tofu_complete
  # Get the current command line
  set -l cmd (commandline -poc)
  set -l line (commandline -p)
  set -l cursor_position (commandline -C)

  # Note: OpenTofu's built-in completion mechanism only works with Bash
  # We'll use pure Fish completion instead

  # Fallback for when OpenTofu doesn't provide completions
  
  # Check if we're completing the command itself or a subcommand/flag
  if test (count $cmd) -eq 1
    # Top-level commands
    echo apply\tBuild or change infrastructure
    echo console\tInteractive console for OpenTofu interpolations
    echo destroy\tDestroy OpenTofu-managed infrastructure
    echo fmt\tReformat your configuration in the standard style
    echo force-unlock\tRelease a stuck lock on the current workspace
    echo get\tInstall or upgrade OpenTofu modules
    echo graph\tCreate a visual graph of OpenTofu resources
    echo import\tImport existing infrastructure into OpenTofu
    echo init\tInitialize a new or existing OpenTofu working directory
    echo login\tObtain and save credentials for a remote host
    echo logout\tRemove locally-stored credentials for a remote host
    echo metadata\tMetadata related commands
    echo output\tShow output values from your root module
    echo plan\tShow changes required by the current configuration
    echo providers\tShow the providers required for this configuration
    echo refresh\tUpdate local state file against real resources
    echo show\tShow the current state or a saved plan
    echo state\tAdvanced state management
    echo taint\tMark a resource instance as not fully functional
    echo test\tExecute integration tests for OpenTofu modules
    echo untaint\tRemove the tainted status from a resource instance
    echo validate\tCheck whether the configuration is valid
    echo version\tShow the current OpenTofu version
    echo workspace\tWorkspace management
    return 0
  end
  
  # Handle subcommands for commands that have them
  if test (count $cmd) -eq 2
    switch $cmd[2]
      case state
        echo list\tList resources in the state
        echo mv\tMove an item in the state
        echo pull\tPull current state
        echo push\tUpdate remote state
        echo rm\tRemove resource from state
        echo show\tShow a resource in the state
        echo replace-provider\tReplace provider in the state
        return 0
        
      case workspace
        echo list\tList workspaces
        echo new\tCreate a new workspace
        echo select\tSelect a workspace
        echo show\tShow current workspace
        echo delete\tDelete a workspace
        return 0
        
      case providers
        echo lock\tWrite provider dependency locks
        echo mirror\tSave copies of provider plugins
        echo schema\tShow provider schemas
        return 0
        
      case metadata
        echo functions\tShow function signatures
        return 0
    end
  end

  # File-specific completions when OpenTofu doesn't handle them
  set -l last_arg $cmd[-1]
  
  if string match -q -- "-var-file" $last_arg
    # Complete with .tfvars files
    for f in *.tfvars
      echo $f
    end
    return 0
  else if string match -q -- "-chdir" $last_arg
    # Complete with directories
    __fish_complete_directories
    return 0
  end
  
  # Complete flags for commands
  if string match -q -- '-*' $cmd[-1] || test (count $cmd) -eq 2 || test (count $cmd) -eq 3
    # Common flags for all commands
    set -l common_flags
    set -a common_flags -help\t'Show help output'
    set -a common_flags -version\t'Show version'
    set -a common_flags -chdir\t'Change working directory'
    
    # Get current command being worked on
    set -l current_cmd
    if test (count $cmd) -ge 2
      set current_cmd $cmd[2]
    end
    
    # Command-specific flags
    switch $current_cmd
      case plan
        set -a common_flags -compact-warnings\t'Show compact warnings'
        set -a common_flags -destroy\t'Create a plan to destroy resources'
        set -a common_flags -detailed-exitcode\t'Return detailed exit codes'
        set -a common_flags -input\t'Enable or disable interactive input'
        set -a common_flags -lock\t'Control state file locking'
        set -a common_flags -lock-timeout\t'State file lock timeout'
        set -a common_flags -no-color\t'Disable color output'
        set -a common_flags -out\t'Write plan to specified file'
        set -a common_flags -parallelism\t'Resource operations limit'
        set -a common_flags -state\t'State file path'
        set -a common_flags -var\t'Set a variable value'
        set -a common_flags -var-file\t'Variable file'
        
      case apply
        set -a common_flags -auto-approve\t'Skip interactive approval'
        set -a common_flags -backup\t'Path to backup state file'
        set -a common_flags -compact-warnings\t'Show compact warnings'
        set -a common_flags -input\t'Enable or disable interactive input'
        set -a common_flags -lock\t'Control state file locking'
        set -a common_flags -lock-timeout\t'State file lock timeout'
        set -a common_flags -no-color\t'Disable color output'
        set -a common_flags -parallelism\t'Resource operations limit'
        set -a common_flags -state\t'State file path'
        set -a common_flags -state-out\t'Write state to different path'
        set -a common_flags -var\t'Set a variable value'
        set -a common_flags -var-file\t'Variable file'
        
      case init
        set -a common_flags -backend\t'Configure the backend'
        set -a common_flags -backend-config\t'Backend config'
        set -a common_flags -force-copy\t'Suppress prompts about copying state'
        set -a common_flags -from-module\t'Source module'
        set -a common_flags -get\t'Download modules'
        set -a common_flags -input\t'Enable or disable interactive input'
        set -a common_flags -lock\t'Control state file locking'
        set -a common_flags -lock-timeout\t'State file lock timeout'
        set -a common_flags -no-color\t'Disable color output'
        set -a common_flags -upgrade\t'Upgrade modules and plugins'
        
      case console
        set -a common_flags -compact-warnings\t'Show compact warnings'
        set -a common_flags -consolidate-warnings\t'Consolidate warnings by type'
        set -a common_flags -consolidate-errors\t'Consolidate errors by type'
        set -a common_flags -var\t'Set a variable value'
        set -a common_flags -var-file\t'Variable file'
        
      case refresh
        set -a common_flags -backup\t'Path to backup state file'
        set -a common_flags -compact-warnings\t'Show compact warnings'
        set -a common_flags -consolidate-warnings\t'Consolidate warnings by type'
        set -a common_flags -consolidate-errors\t'Consolidate errors by type'
        set -a common_flags -input\t'Enable or disable interactive input'
        set -a common_flags -lock\t'Control state file locking'
        set -a common_flags -lock-timeout\t'State file lock timeout'
        set -a common_flags -no-color\t'Disable color output'
        set -a common_flags -parallelism\t'Resource operations limit'
        set -a common_flags -state\t'State file path'
        set -a common_flags -state-out\t'Write state to different path'
        set -a common_flags -var\t'Set a variable value'
        set -a common_flags -var-file\t'Variable file'
        set -a common_flags -target\t'Target specific resource'
        set -a common_flags -target-file\t'Target resources from file'
        set -a common_flags -exclude\t'Exclude specific resource'
        set -a common_flags -exclude-file\t'Exclude resources from file'
        set -a common_flags -replace\t'Replace specific resource'
        
      case destroy
        set -a common_flags -auto-approve\t'Skip interactive approval'
        set -a common_flags -backup\t'Path to backup state file'
        set -a common_flags -compact-warnings\t'Show compact warnings'
        set -a common_flags -input\t'Enable or disable interactive input'
        set -a common_flags -lock\t'Control state file locking'
        set -a common_flags -lock-timeout\t'State file lock timeout'
        set -a common_flags -no-color\t'Disable color output'
        set -a common_flags -parallelism\t'Resource operations limit'
        set -a common_flags -state\t'State file path'
        set -a common_flags -state-out\t'Write state to different path'
        set -a common_flags -var\t'Set a variable value'
        set -a common_flags -var-file\t'Variable file'
        
      case taint untaint
        set -a common_flags -allow-missing\t'Succeed even if resource is missing'
        set -a common_flags -backup\t'Path for backup state file'
        set -a common_flags -lock\t'Control state file locking'
        set -a common_flags -lock-timeout\t'State file lock timeout'
        set -a common_flags -state\t'State file path'
        set -a common_flags -state-out\t'Write state to different path'
        set -a common_flags -ignore-remote-version\t'Continue even if versions differ'
        set -a common_flags -var\t'Set a variable value'
        set -a common_flags -var-file\t'Variable file'
        
      case import
        set -a common_flags -config\t'Directory containing config files'
        set -a common_flags -backup\t'Path for backup state file'
        set -a common_flags -ignore-remote-version\t'Continue even if versions differ'
        set -a common_flags -lock\t'Control state file locking'
        set -a common_flags -lock-timeout\t'State file lock timeout'
        set -a common_flags -input\t'Enable or disable interactive input'
        set -a common_flags -no-color\t'Disable color output'
        set -a common_flags -parallelism\t'Resource operations limit'
        set -a common_flags -state\t'State file path'
        set -a common_flags -state-out\t'Write state to different path'
        set -a common_flags -var\t'Set a variable value'
        set -a common_flags -var-file\t'Variable file'
        set -a common_flags -consolidate-warnings\t'Consolidate warnings by type'
        set -a common_flags -consolidate-errors\t'Consolidate errors by type'
        
      case graph
        set -a common_flags -draw-cycles\t'Highlight cycles in the graph'
        set -a common_flags -type\t'Type of graph to output'
        set -a common_flags -module-depth\t'Max depth for modules'
        set -a common_flags -verbose\t'Include detailed data'
        set -a common_flags -plan\t'Use specified plan file'
        set -a common_flags -var\t'Set a variable value'
        set -a common_flags -var-file\t'Variable file'
        
      case test
        set -a common_flags -compact-warnings\t'Show compact warnings'
        set -a common_flags -consolidate-warnings\t'Consolidate warnings by type'
        set -a common_flags -consolidate-errors\t'Consolidate errors by type'
        set -a common_flags -filter\t'Test file filter pattern'
        set -a common_flags -json\t'Output test results as JSON'
        set -a common_flags -no-color\t'Disable color output'
        set -a common_flags -test-directory\t'Directory containing test files'
        set -a common_flags -var\t'Set a variable value'
        set -a common_flags -var-file\t'Variable file'
        set -a common_flags -verbose\t'Show more test output'
        
      case output
        set -a common_flags -state\t'Path to read state'
        set -a common_flags -no-color\t'Disable color output'
        set -a common_flags -json\t'Print output in JSON format'
        set -a common_flags -raw\t'Print raw strings directly'
        set -a common_flags -show-sensitive\t'Show sensitive outputs'
        set -a common_flags -var\t'Set a variable value'
        set -a common_flags -var-file\t'Variable file'
        
      case get
        set -a common_flags -update\t'Check for available updates'
        set -a common_flags -no-color\t'Disable color output'
        set -a common_flags -test-directory\t'Directory for module tests'
        set -a common_flags -json\t'Output in JSON format'
        set -a common_flags -var\t'Set a variable value'
        set -a common_flags -var-file\t'Variable file'
        
      case fmt
        set -a common_flags -list\t'List files whose formatting differs'
        set -a common_flags -write\t'Write result to source files'
        set -a common_flags -diff\t'Display diffs of formatting changes'
        set -a common_flags -check\t'Check if input is formatted'
        set -a common_flags -no-color\t'Disable color output'
        set -a common_flags -recursive\t'Process files in subdirectories'
        
      case show
        set -a common_flags -json\t'Output in JSON format'
        set -a common_flags -show-sensitive\t'Show sensitive output values'
        set -a common_flags -state\t'Path to read state'
        set -a common_flags -plan\t'Path to read plan'
        set -a common_flags -config\t'Show configuration'
        set -a common_flags -module\t'Show module call source'
        set -a common_flags -var\t'Set a variable value'
        set -a common_flags -var-file\t'Variable file'
        
      case validate
        set -a common_flags -json\t'Output in JSON format'
        set -a common_flags -no-color\t'Disable color output'
        set -a common_flags -test-directory\t'Directory containing test files'
        set -a common_flags -no-tests\t'Don\'t validate test files'
        set -a common_flags -var\t'Set a variable value'
        set -a common_flags -var-file\t'Variable file'
        
      case version
        set -a common_flags -json\t'Output in JSON format'
    end
    
    # Add subcommand flags
    if test (count $cmd) -eq 3
      switch "$cmd[2] $cmd[3]"
        case "state list"
          set -a common_flags -state\t'Path to the state file'
          set -a common_flags -id\t'Show resource IDs only'
          
        case "state mv"
          set -a common_flags -state\t'Path to the state file'
          set -a common_flags -backup\t'Path for backup state file'
          set -a common_flags -lock\t'Control state file locking'
          set -a common_flags -lock-timeout\t'State file lock timeout'
          
        case "state push"
          set -a common_flags -force\t'Write even if lineages do not match'
          
        case "state rm"
          set -a common_flags -state\t'Path to the state file'
          set -a common_flags -backup\t'Path for backup state file'
          set -a common_flags -lock\t'Control state file locking'
          set -a common_flags -lock-timeout\t'State file lock timeout'
          
        case "state show"
          set -a common_flags -state\t'Path to the state file'
          
        case "workspace new"
          set -a common_flags -lock\t'Control state file locking'
          set -a common_flags -lock-timeout\t'State file lock timeout'
          
        case "workspace delete"
          set -a common_flags -force\t'Skip confirmation'
          set -a common_flags -lock\t'Control state file locking'
          set -a common_flags -lock-timeout\t'State file lock timeout'
          
        case "providers lock"
          set -a common_flags -fs\t'Force selection of given providers'
          set -a common_flags -net-mirror\t'Network mirror URL'
          set -a common_flags -platform\t'Target platform'
          
        case "providers mirror"
          set -a common_flags -platform\t'Target platform'
          
        case "providers schema"
          set -a common_flags -json\t'Output in JSON format'
          
        case "metadata functions"
          set -a common_flags -json\t'Output in JSON format'
      end
    end
    
    for flag in $common_flags
      echo $flag
    end
    
    return 0
  end

  # Default to file completion for unknown arguments
  return 1
end

# Register the main completion function
complete -f -c tofu -a "(__tofu_complete)"