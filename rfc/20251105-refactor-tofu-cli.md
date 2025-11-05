# Cleaner and extensible CLI layer

Tickets that are targeted to be resolved by this:
* https://github.com/opentofu/opentofu/issues/748
* https://github.com/opentofu/opentofu/issues/3500
* https://github.com/opentofu/opentofu/issues/3050
* https://github.com/opentofu/opentofu/issues/2239

Due to a multiple legacy reasons, the whole startup procedure of OpenTofu, today is really intertwined and scattered all over the place.

This RFC will try to explain how it is working now, what the pain points of that are. It will also try to propose a different approach on how to rework the whole
startup to make it more maintainable extensible.

_This RFC was created during the Hackathon we had on November 2025._

## Current structure and biggest pain points
In this section, I will try to go in a natural flow of the components, starting from the `main` function and branching down in different existing flows.

### `main` function
For testing purposes, the `main` function calls another `realMain` function. This should be kept for testing purposes.
In order to highlight some of the issues in here, I want to list all the things that the `realMain` function does.
These will be listed in the order they are executed in the code.
A mark (X) in front of the item suggests that the step should be performed later in the execution and not in the `realMain` function.
If you don't want to read the entire list (and I agree that it's not that interesting), look for the bolded entries:
* Configure UI (to print into the terminal updates of the initialisation process)
* Enable CPU Profiling if requested through an env var (`TOFU_CPU_PROFILE`)
* Enable OTEL if configured
* Create a log file if requested by the env var `TF_TEMP_LOG_PATH`
* Print versions of some go dependencies the current build is using
* (X) loads the CLI configuration
* (X) loads credentials source based on the CLI configuration
* (X) creates a new service discovery (for providers and modules)
* (X) creates a package fetcher for modules and providers
* (X) figure out if it should reattach to a running provider instance by checking env var `TF_REATTACH_PROVIDERS`
* (X) initialize the backends factory
* extracts and executes `-chdir` out of the given args
* (X) transforms the CLI configuration into actionable provider sources to be able to download providers 
* initializes the commands (the way these are initialised today is one of the reasons why this RFC exists)
* (X) executes logic to extract the subcommand that is requested in order to inject some of the env variables into the `args` slice before actually running the requested command
* **If the user requested to print the version (by specifying `-v` or `--version` or `-version`) in the args, it will replace all the args with the version subcommand to run that**
* **After this, it tries to run the command. There is some custom logic to suggest another command in case the one given is misspelled.**

As you can see, the last 2 steps can be easily performed way before running any of the marked steps, boosting the basic validations way before doing all the heavy work listed before.

### `Meta` structure
The current `Meta` structure contains way too many configurations and responsibilities, being used as a container to carry common information between
`main` and other parts of the system.
The idea is not a bad one, but the fact that it's only one structure to rule everything, makes things hard to follow since not all of the commands
use all the attributes/functionality inside `Meta`.

The problems start with this structure being injected into each command during `initCommands`.
Because of that, over time, a lot of "useful" methods have been added to the struct, overloading it in such a way that now it's hard to track what is used where and how.

#### Flags and args
Because `Meta` is part of every existing command and because it carries such a high amount of functionality, commands' flags and arguments parsing
is done directly in its properties, mixing and matching attributes configured from the flags to make the inner functionality work correctly.
This adds a lot of complexity and confusion when analyzing what flags and arguments a command is meant to have and how those are mapped to the attributes
of the `Meta` struct. Moreover, the logic in `Meta` that uses these flags bounded attributes is messy and scattered, only a few of those methods being clear in their purpose.

### `github.com/mitchellh/cli`
From an early age of OpenTofu, `github.com/mitchellh/cli` has been used to configure the subcommands of the system.
The library is doing its job, but it's having several limitations and quirks that make the whole setup and parsing of commands and args confusing and error prone.

Another thing that is handled kind of weird, is the functionality around help text for the root command and subcommands where a lot of custom logic is written to handle
that, adding a lot to the maintenance cost whenever a new argument needs to be added or updated.
There is no auto generation of the help information of the flags or the commands from the command structure itself. All the help text is written separately in
a manually wrapped text at 80 characters.

Another tricky part (tackled later too) is that this library uses https://github.com/posener/complete for autocompletion functionality.
The autocomplete works totally fine even though its capabilities are limited:
* No autocompletion for flags and only autocompletion for basic commands.
* No way to customize where the autocompletion scripts are written, are always written to specific files based on the detected shell that's running it.

## Proposed Solution
If OpenTofu will decide to go forward with the refactoring of the stated issues above, here is a rough list of the actions that should be taken and the order of those.

The order in which are presented was done with careful consideration and deep analysis of the dependent subsystems. The goal of this ordering is to minimize the amount of
changes to enable easier and faster reviews while preserving the functionality and behavior.

The refactoring process would be summarized as follows:
* refactor `realMain` to move all the marked steps from the "[Current structure and biggest pain points](#current-structure-and-biggest-pain-points)" into their own components and call those only when needed
* extract all the quirky logic in `realMain` around `-version` and `TF_CLI_ARGS` in its own function/component
* break down the `Meta` struct and replace it with individual, one purpose components for each type of action that a command needs to take:
  * config loader
  * `-var/-var-file` parser and handler
  * backend handler (to have its own flags and configuration and the associated logic that is scattered for the moment)
  * `chdir`er. A component that will have its configuration that would be able to be used for flags configuration and a handler that will use that configuration to change dir when asked.
  * etc
* replace `github.com/mitchellh/cli` with `github.com/spf13/cobra` (details in a later section)
* **double/triple check that everything works as before**

The idea is to have a more "normal" flow of operations:
* run `main`
* execute the requested subcommand
  * if not found, offer a suggestion from the ones existing using Levenshtein distance
* the executed command parses flags and creates the required components
* executes the actions in the order they are executed today
* exit

### First part: refactor `realMain`
Extract the following bits in their own components/functions and pass these in `initCommands` and in the commands that require those to be called by the executed command before anything else.
This will alleviate some of the performance bottlenecks in the execution today, by not running logic that is not necessary for situations as described above (running `tofu -version` or when a command is misspelled).

The logic that needs to be extracted:
* load CLI configuration before executing `chdir`
* service discovery
* module package fetcher
* reattach providers
* init backend (requires service discovery)
* chdir executor
* provider source
* **extract meta creation in its own function where all the above are passed as arguments**
* init commands with the meta built above
* extract the logic for `TF_CLI_ARGS` and execute it
* extract the logic with `-version` flag and execute it

### Second part: remove the `Meta` struct
In order to restructure the entire initialisation part of OpenTofu we need to break down the `Meta` structure and allow granular composition
of the functionalities involved in each command.

#### First iteration: Break/split `Meta`
The general idea is to break the attributes that are used to configure flags and the associated actions from the `Meta` structure into separate, single purpose structs.
At the end of this step, we would end up with a `Meta` struct working similarly as it is doing right now:
```go
type Meta struct {
	VarsFlags
	VarsHandler
	
	// ...
}
```

##### Break the flags
> [!NOTE]
> Before creating the newly suggested `*Flags` structs (ie: `PlanFlags`, `ApplyFlags`, etc), we should check first the already existing
> implementation in [`command/arguments`](https://github.com/opentofu/opentofu/tree/cc8e86c99842f3ee943419a333bc996095834b0a/internal/command/arguments) that
> was created some time ago with a wanted end goal as this RFC.

In order to be able to move forward, first, I would suggest to extract the flags and arguments related attributes from the `Meta` struct in their own configuration structure.

As an example for this idea, we could look at the attributes related to `-var`/`-var-file`.
Right now, the attributes are stored in the `Meta` struct and have its associated functionality in [Meta#collectVariableValues](https://github.com/opentofu/opentofu/blob/93d095c67eeeb2dd50755f487549fb80d5cdd8c6/internal/command/meta_vars.go#L44).
There is already a structure which can be used as an example of the intended action on this step, [arguments.Vars](https://github.com/opentofu/opentofu/blob/93d095c67eeeb2dd50755f487549fb80d5cdd8c6/internal/command/arguments/extended.go#L290).

So we could create a struct like the following:
```go
type VarsFlags struct { // Maybe we can come up with a better naming
	vars map[string]string
	varFiles []string
}
```
In this step, we can embed the this struct in the `Meta` and replace the existing attributes for this. Also, ensure that the flags, all over the place do point to these new struct attributes.

##### Break the functionality
As said in the previous section, the functionality of the attributes associated with the `-var`/`-var-file` flags is in [Meta#collectVariableValues](https://github.com/opentofu/opentofu/blob/93d095c67eeeb2dd50755f487549fb80d5cdd8c6/internal/command/meta_vars.go#L44).
The idea is to have a new struct like the following:
```go
type VarsHandler struct {
	cfg VarsFlags
}
```

Now, we can move the `collectVariablesValues` on this struct and [embed](https://go.dev/doc/effective_go#embedding) it in the `Meta` struct again, allowing the existing code to work as it is doing right now:
```go
func (vh *VarsHandler) collectVariablesValues(...) (...) {
	...
}
```

#### Second iteration: Extract all the `*Flags` structs from the `Meta` into another similar in size structure
In this step we would target to separate the idea of "flags and arguments" from the actual functionality.

To do so, one approach would be to have the flags configured with a struct that will embed only the `*Flags` structs and have those used strictly to read and handle the arguments
and be able to present a *clear* configuration to its `*Handler` struct.
So `Meta` will be looking something like this:
```go
type Meta struct {
	VarsHandler
	// ...
}
```

And the new configuration structure should be looking like this:
```go
type MetaFlags struct {
	VarsFlags
	// ...
}
```

By doing so, we will draw a clear boundary, between the (CLI execution structure/parsing model) and the actual logic of the system.

#### Third iteration: Transform `Meta` in a container to carry around independent components
Now, since we have the `Meta` logic and attributes broke down in separate structs, we can start using "dependency injection" to chain these in the way they need to.
To continue with the example above, the new `VarsHandler` will require a `*configload.Loader` to be able to do its job. This will require for the `*configload.Loader` to be initialised
before having the `VarsHandler` created.
To achieve this, we need to extract these pieces in their own components and have clear logic, as detached as possible, to build all these independent components.

> [!NOTE]
> It's really important to have the components as single purpose as possible to allow easier composition later when we would have to create only some of the components for some commands.
> 
> This is suggested as such because we might actually improve the startup performance if we would be able to initialise only the minimum set of functionalities required by each command.

For sure there will be places where things will get a little bit messier, but the purpose of this step is to extract and move as much of the functionality from the `Meta` as possible.
Abstraction and DI could be done later when the boundaries between components will become clearer.

The way I envision the end result of this step at the moment of writing this document, is to have loosely coupled components inside the `Meta` struct offering a better understanding of the still existing tight coupled ones before moving forward.

#### Fourth iteration: resolve any tight coupled components
Not much to say here other than if during the third iteration we encountered any dependencies between components that require more risky refactoring, that should be deferred to this step
to be able to have clear reviews on these.

#### Fifth iteration: replace `Meta` entirely
Now that we have separate, clear purpose components that are composable, we can instantiate and execute only the logic that is strictly required by each command.
Therefore, in the `initCommands` we want to replace the `Meta` struct given to each command, with all the `*Flags` structs that right now are embedded into the the new idea of `MetaFlags`.

> [!NOTE]
> At this stage, we might need to actually to build all the `*Handler` structs before creating the commands, 
> but the end goal is for the commands to have little to no dependencies added when created. Maybe only the UI to print things on the output
> or other things that are built in `realMain`.
 
### Second part: replace CLI library
>[!NOTE]
> This second part can be started when all the flags and components are already moved and executed by the commands' `Run` method.

> [!NOTE]
> 
> Because OpenTofu uses [golang stdlib flag package](https://pkg.go.dev/flag), and because that lib supports long format flags
> with a single dash in front (eg: -flagname), we cannot use other libraries for flag parsing since the vast majority of those are 
> [GNU compliant](https://www.gnu.org/prep/standards/html_node/Command_002dLine-Interfaces.html), forcing single dash flags to have only one character.
> Therefore, we are stuck with the flags parsing that is already in place.

Here is a list of features from [Cobra](https://github.com/spf13/cobra) (and [pflag](https://github.com/spf13/pflag)) that we want to use here:
* [Flags](https://cobra.dev/docs/how-to-guides/working-with-flags/) and especially [copying those to golang stdlib](https://github.com/spf13/pflag/pull/330/files)
* [Wrapping help text](https://github.com/spf13/pflag/blob/6fcfbc9910e1af538fde31db820be7d1bec231e4/flag.go#L707) at a specific number of characters
* [Grouping](https://github.com/spf13/cobra/blob/611e16c322e3b0413a9ba4e489fcd98ef904e406/site/content/user_guide.md#grouping-commands-in-help) commands in help text
* Autocompletion capabilities that in addition to the ones that OpenTofu supports today (bash, zsh, fish) it adds also the PowerShell autocompletion.

With the above features, let's look how the migration _could_ look like:
* Create basic new commands for all the existing ones and link those to a new `rootCmd`
  * Ensure that any old command that was configured with 2 or more keywords are configured correctly in layers (eg: rootCmd -> env -> list; rootCmd -> env -> select; etc)
* Because we will use later the go stdlib `flag` parsing, we need to disable the flags parsing from cobra by configuring [`DisableFlagParsing`](https://pkg.go.dev/github.com/spf13/cobra#Command.DisableFlagParsing) on every command.
* (Optional) If we agree to generate the root command help text with cobra and not with a custom function, then configure commands in 2 groups: `main` and `other`. 
  * See what commands should be in `main` by checking the existing global variable `primaryCommands`.
  * We need to hide some commands by configuring [`Command.Hidden`](https://pkg.go.dev/github.com/spf13/cobra#Command.Hidden). For the commands that need to be hidden, check the global variable `hiddenCommands`
* For ease of maintenance, for each command we should migrate all the flag **definitions** from `flag` to `cobra`/`pflag`
  * This will provide an easier way to work with the flags, defining, generating help texts, etc.
  * Once those are defined, beware, because each command is configured now with [`DisableFlagParsing`](https://pkg.go.dev/github.com/spf13/cobra#Command.DisableFlagParsing), meaning that parsing should be called manually.
  * Generally, the first step when the `Run` function of a command will be executed, is to call [`pflag.CopyToGoFlagSet`](https://pkg.go.dev/github.com/spf13/pflag#FlagSet.CopyToGoFlagSet) that will copy all the defined flags to a golang `flag.FlagSet` and then call `flagSet.Parse(args)`.
    * Check the second note from this section for the reasoning around this.
* Implement a help function on the root command that will be applicable to all the subcommands. This is needed because if cobra is allowed to run its built-in function for this, it will print the flags with `--` in front, which is wrong in the grand scheme of things.
* Create a [mitchellh/cli](https://github.com/mitchellh/cli/blob/main/cli.go#L439) inspired logic to provide legacy autocompletion for users with already installed autocompletion scripts.

#### Help function
If we use grouping, rootCmd flags and short/long description for the commands, by default this is the output of the help:
```shell
The available commands for execution are listed below. The primary workflow commands are given first, followed by less common or more advanced commands.

Usage:
  tofu [command]

MAIN COMMANDS
  apply        Create or update infrastructure
  destroy      Destroy previously-created infrastructure
  init         Prepare your working directory for other commands
  plan         Show changes required by the current configuration
  validate     Check whether the configuration is valid

ALL OTHER COMMANDS
  console      Try OpenTofu expressions at an interactive command prompt
  fmt          Reformat your configuration in the standard style
  force-unlock Release a stuck lock on the current workspace
  get          Install or upgrade remote OpenTofu modules
  graph        Generate a Graphviz graph of the steps in an operation
  import       Associate existing infrastructure with a OpenTofu resource
  login        Obtain and save credentials for a remote host
  logout       Remove locally-stored credentials for a remote host
  metadata     Metadata related commands
  output       Show output values from your root module
  providers    Show the providers required for this configuration
  refresh      Update the state to match remote systems
  show         Show the current state or a saved plan
  state        Advanced state management
  taint        Mark a resource instance as not fully functional
  test         Execute integration tests for OpenTofu modules
  untaint      Remove the 'tainted' state from a resource instance
  version      Show the current OpenTofu version
  workspace    Workspace management

Additional Commands:
  help         Help about any command

Flags:
      --chdir string   Switch to a different working directory before executing the given subcommand
      --help           Show this help output, or the help for a specified subcommand
      --version        Alias to "version" command

Use "tofu [command] --help" for more information about a command.
```

We can customize this heavily and cobra offers the ability to define a function on the rootCmd that can handle the generation of the help messages for any sub command.
Most probably will have to do a custom function just because of the flags being prefixed wrongly.

You can compare this with the output generated when running `tofu -h` so that we can have a discussion around this.

#### Autocomplete
We have several tickets that suggest some enhance approaches related to autocompletion:
* [#2239](https://github.com/opentofu/opentofu/issues/2239) - Provide pre-generated autocompletion configuration scripts for inclusion in deb/rpm/homebrew/etc packages
* [#3500](https://github.com/opentofu/opentofu/issues/3500) - Expose shell completion content
 
As seen, there would be preference in being able to expose the autocomplete scripts and not have a direct installation of those, as it's done right now.
The current CLI library uses https://github.com/posener/complete that is doing only the installation of the autocomplete scripts,
without giving the option to print those to stdout for allowing the users to source those on the fly.

Compared with the library that we use right now for autocompletion, by using other libraries that support autocompletion, we would should target on a solution
that:
* allows customisation of the installation scripts (to be able to use the same ones that we use today)
* allows customisation of the stream to write the autocompletion to, therefore enabling us to use the same installation approach that we have today and in addition, allowing us to write those directly to stdout.

### Open Questions
* Is out there some context and quirks that we need to know about the order of execution of specific bits in our startup? 
  * To explain what I am referring to, I've seen that there is a specific logic around `chdir` where CLI configuration needs to be loaded before executing `chdir` to reference the initial workdir.
* What level of difference are we ok with when it comes to the help text?
  * This is related to the ability of the cobra commands to be grouped and there we could remove most of the code for help text generation and instead rely on the grouping that it's already doing.

### Future Considerations

We need to ensure that the current unit tests keep working and the changes on those is minimal and non-functional. And when it comes to changes for checks on generated visual aspects, we need to do the due diligence and check if it will not break current parsers or anything similar.

> [!WARN]
> If we ever go with this type of refactor, before starting any work, especially in `realMain`, we need to understand all the edge cases and all the "whys" about having things 
> created before others and what is the meaning and reason of doing it that way.
>
> One obvious example is the fact that CLI configuration is loaded before running the logic for the `chdir` flag to be sure that we load the relative configs from the initial workdir.
>
> We need to be sure that we follow exactly the same logic if we change anything, otherwise we might find ourselves in a state with a broken CLI layer.

## Potential Alternatives

As seen in the current RFC, the level of customisation that we have to add on top of the cobra library is significant.
But most of the customisation is due to the legacy behavior that we still want to keep:
* help texts formatting
* golang style flags
* autocompletion scripts similar with what we have today but more capable

That being said, by writing this RFC, I realised that there is no bullet proof CLI library that we can
choose to cover all the use cases that we have and want to keep, so _the most customisable_ library would be the one to go with.