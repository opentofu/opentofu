# Cleaner and extensible CLI layer

Due to multiple compounding legacy reasons, at the moment of writing this RFC, the whole startup procedure 
of OpenTofu is heavy with too many responsibilities. 
It also performs several initialisations just to drop them in case of early exit requests:
* version printing
* autocomplete suggestions
* `metadata functions` command 
* and more

Next, we would like to talk shortly about the main reasons of this RFC:
* the `Meta` structure grew over years into a hard to maintain layer, containing tons of logic that is used 
  by different commands in different ways. We want to alleviate some of that maintenance cost from the future.
* we want to detach the flags and system logic from the CLI library to be able to switch later to a different CLI
  library, that will enable delivery of a more capable UI and satisfy all the currently existing requests:
  * https://github.com/opentofu/opentofu/issues/748
  * https://github.com/opentofu/opentofu/issues/3500
  * https://github.com/opentofu/opentofu/issues/2239
  * https://github.com/opentofu/opentofu/issues/3050 (maybe)
  * https://github.com/opentofu/opentofu/issues/3044

This RFC attempts to propose some approaches to rework this and will tackle different challenges that we might
encounter in doing so.
This RFC is not meant to be an exhaustive guide on how to approach such a change, but it is meant to provide
the rules and draw the boundaries on which this refactor should stay in.

Before kicking this off, there are several requirements that need to be noted and some things that we need 
to be mindful about:
* The changes proposed in this RFC **must not change any existing material functionality**.
  * The purpose of some changes are to postpone (defer) some of the logic that is today performed
    ahead of its actual purpose. Hence, there might be changes where some errors would be returned before others.
    But the important thing is that the errors must still be reported, even if the order is different.
  * Today, because the `realMain` function performs some common initialisations, the general UX of OpenTofu
    prompts the user right away about the issues with its environment (like invalid CLI configuration) which helps
    with pin pointing from the get go issues that user can tackle right away.
    Even though this is having the advantage of having "one central place" to validate basic environment configuration,
    it might change in terms that the commands that act on nothing to do related to providers, config or state, might
    not error on the wrong environment configuration of the user. This change should be adopted mostly for
    the commands that are purely informative or acting on other resources (like `fmt`, `--version`, `metadata functions`).
    The current behavior should be kept for any command that acts on critical parts of a user configuration like providers,
    configuration, modules, state, etc.
* The logic around flags parsing will change but **must keep the same validation logic**.
* Because this RFC will not tackle CLI library swap (there's [#3541](https://github.com/opentofu/opentofu/pull/3541) for that),
  we will not target on changing much on that layer, but the main goal here is to add the necessary
  abstractions allowing existing code without knowing about the parsed flags and arguments. 
* Since this is not meant to be a user facing change, we do not intend to have this delivered under experimental
  flag. 
  * Therefore, as we go, the changes added **must be constantly vetted and tested against old behavior.**
    **No changes in the logic of the application will be allowed.**
* The changes in the [Proposed Solution](#proposed-solution) section are meant to be incremental and should
  be applied and merged only in working state.
  * **After the whole refactor will be done, the system must work the same and provide comparable results.**

## Proposed solution
This proposal wants to tackle 2 main things:
* abstract the logic executed before choosing a command to run and execute that logic inside the command before
  executing the actual command's logic.
* separate the CLI arguments/flags parsing from the actual command execution to be able to switch to another
  CLI library in the future.

In the end, the idea is to have a more streamlined flow of operations:
* run `main`
* execute the requested subcommand
  * if not found, offer a suggestion from the existing ones, if possible
* the executed command parses flags and creates the required components
* executes the actions in the order they are executed today
* exit

### `realMain` function
Looking at the [`realMain` function](https://github.com/opentofu/opentofu/blob/fd19a3763f67e3dd3d29734dc0eae4220cdc08c3/cmd/tofu/main.go#L72),
can be seen that there is some logic that should reside inside the commands, instead of being executed 
before running the layer that decides what command is invoked.

What I propose in this particular case, is to keep this function with critical logic untouched and 
extract the specific logic bits in their own abstractions to be used later.

> [!NOTE]
> Even though we should keep the "critical logic" bits in `realMain`, would be advisable to extract these in
different methods for a more concise content of the `realMain` function.

To make things clear, let's categorise these bits:
* critical logic
  * logging
  * telemetry
  * profiling
  * debugging configurations
  * fips warning
  * experiments system
  * terminal traces
  * version shortcircuit
  * autocomplete (where we will have to choose the autocompletion approach based on how its invoked. 
    See details about this in [here](https://github.com/opentofu/opentofu/blob/9e1161a6e6d83c271e06afbaec6fa0cca505c219/rfc/20251105-use-cobra-instead-of-mitchellh.md#bridge-between-old-and-new-autocompletion-scripts).)
* OpenTofu specific functionalities:
  * cli configuration
  * credentials source
  * chdir
  * provider sources
  * default config directory initialisation

The bits under "OpenTofu Specific functionalities" must be extracted in their own abstractions and 
moved in a different layer where will be executed only when actually required. 

Some examples of commands that **don't** need to execute some (or none) of the bits in the aforementioned category are:
* `metadata functions`
* `fmt`
* `version`
* `workspace`
* `get`

One particular functionality that we need to experiment with to understand better how we could handle it 
in isolation, is the processing of `TF_CLI_ARGS` env var(s). But for the moment it can stay in `realMain`.

> [!NOTE]
> One idea that looks promising is to use the prefixed environment variables from [sfp13/viper](https://github.com/spf13/viper/tree/528f7416c4b56a4948673984b190bf8713f0c3c4#working-with-environment-variables), 
> but this is out of scope of this RFC.

### `Meta` structure
The current `Meta` structure contains way too many configurations and responsibilities, being used as a 
container to carry common information and logic between `main` and other parts of the system.

Due to the high complexity of the implementation around flags, there is hardly a specific "recipe" on how to refactor
all of this and it will have to be approached different case by case.
This chapter will try to highlight mostly the end goals that we strive for.

In the end, all the logic that today lives in the `Meta` structure should be extracted and used from its
own abstractions.

#### Extract logic in their own abstractions (and flags)
The way flags are configured and parsed today (generally) includes the following steps:
* define the flag
* parse the flag
* validate the given values as some might depend on others (e.g.: see `-backend` vs `-cloud`)
* build components

> [!NOTE]
> Before creating new structs to hold the functionality and the flags for a particular logic bit, first, should be checked
> if the already existing implementation in [`command/arguments`](https://github.com/opentofu/opentofu/tree/cc8e86c99842f3ee943419a333bc996095834b0a/internal/command/arguments)
> could be used moving forward.
> 
> If possible, we should build on that, that package responsibility being to do specifically the "validate" part of the 
> points above.

Since many `Meta` arguments are used as containers for flag values, we want to extract logically grouped
flags in specific functional structs and implement in those structs the steps listed above.

In many cases, the flags extraction will force the logic around those to be extracted too, in the same iteration,
in a way to allow building the associated components based on the given arguments.

To be able to extract all functionality out of the `Meta` without breaking anything, this needs to be done incrementally 
starting from the components with no dependencies and find the way through the entire functionality.

At a first look, there are already some bits that can be extracted in their own components:
* [Workdir](https://github.com/opentofu/opentofu/blob/b2c6b935e06bfca47662792e7f71a7df8a6a36ad/internal/command/workdir/dir.go)
  * this _could_ also integrate the `-chdir` logic from the `realMain` and be given as dependency into the logic
    that relies on it
* cliconfig
  * multiple bits
    * loading of the config
    * creation of credentials sources
    * creation of service discovery
    * remote module fetcher
    * creation of provider sources
  * are all connected in terms that are reliant on the [cliconfig.Config](https://github.com/opentofu/opentofu/blob/b2c6b935e06bfca47662792e7f71a7df8a6a36ad/internal/command/cliconfig/cliconfig.go#L40) and any configuration around that

Then, there are some bits that depend mostly (if not solely) on flags and default configuration: 
* ui/view
  * sadly, this particular part started to be refactored [back in 2021](https://github.com/opentofu/opentofu/commit/6f58037d6a6aae531cf8caf848478dc78ff9acb2) 
    but the refactoring has not been concluded.
  * there are commands (like `init`) that still use both, `UI` and `View` that will make this particular
    proposal a little bit convoluted on this area, but nothing impossible
  * later, once the logic of building and configuring the `UI` and `View` will be extracted in its component,
    we can continue with the old refactor to unify the way json/human views are built
* config loader
  * this is mainly dependent on the 2 components that we described above: UI and Workdir. Once those will be extracted,
    this can be extracted too.

Therefore, starting with the least dependent bits will allow walking down the chain and extract everything in separate
components that can be instantiated inside the `Run` method of the commands.

> [!NOTE]
> About the `ui/view`. After did some work proposed by this RFC, became clearer that we can continue without
> the migration of all commands to the Views concept from the old UI, but in doing so will add more shim
> code to make things work properly.
> 
> Therefore, as part of this RFC, we can carry on the migration of all the commands to the Views abstraction,
> which will make any subsequent work way easier.

During this work, all logically grouped flags should be moved to these structs and allow the struct logic to
record these on a particular FlagSet or parse the values for those directly.

E.g.:
```go
type Workdir struct {
	chdir *string
}

func (w *Workdir) ParseFlags(args []string) error { ... }
// or 
func (w *Workdir) RecordFlags(in *flag.FlagSet) {
    in.StringVar(&w.chdir, "chdir", "", "Switch to a different working directory before executing the given subcommand.")
}
```

> [!NOTE]
> 
> To make it easier to be reviewed and iterate on the implementation, 
> we can make use of the `Meta` struct as the container of each component instance.
> Later, those can be created in the commands `Run()` method directly once we manage to extract all the logic out
> of the `Meta` struct.

As this will progress, more and more components will expose their dependencies on other components, so those will have 
to be chained accordingly, leading in the end to multiple components, with specific responsibility and visible
dependencies between one another.

> [!NOTE]
> It's really important to have the components as single purpose as possible to allow easier composition 
> later when we would have to instantiate only the required components by each command.
>
> This is suggested as such because we might be actually able to improve the startup performance if the refactor
> will allow initialisation only of the components needed by each command.

#### Meta backend
The most complex and sensitive part of the `Meta` structure is the [backend implementation](https://github.com/opentofu/opentofu/blob/85de3d40fae67efb8cfe9020bba738433b49c409/internal/command/meta_backend.go#L95).

The implementation visible in [`meta_backend.go`](https://github.com/opentofu/opentofu/blob/d24b85fc685be224da93de546b5566429ca7b62f/internal/command/meta_backend.go)
and in [`meta_backend_migrate.go`](https://github.com/opentofu/opentofu/blob/d24b85fc685be224da93de546b5566429ca7b62f/internal/command/meta_backend_migrate.go)
tackles a lot of edge cases and legacy concerns that the reasons behind it is not that clear.
Therefore, this RFC would want only to extract these 2 files in their own components (or just only one component),
to make things clearer on what it depends on. Some clear dependencies are:
* workspace information
* discovery for services
* config loader
* ui/view
* workdir information
* state related flags (`-state`, `-state-out`, `-backup`, `-lock`, `-lock-timeout`)
* backend related flags (`-reconfigure`, `-migrate-state`, `-force-copy`)
* the logic for asking user input together with its flag (`-input`)
  * in here we also have [this ugly global bool](https://github.com/opentofu/opentofu/blob/d24b85fc685be224da93de546b5566429ca7b62f/internal/command/command.go#L15) 
    that controls if it's enabled or not and this should be handled separately and included in the user input prompt component.

Before even attempting backend implementation isolation, at least the list of dependencies above should be handled.

## Open Questions
* Is out there some context and quirks that we need to know about, like the order of execution of specific bits 
  in OpenTofu's startup?
  * To explain what I am referring to, I've seen that there is a specific logic around `chdir` where CLI 
    configuration needs to be loaded before executing `chdir` to reference the initial workdir. Do we have other
    bits similar to that that you are aware of?

## Future Considerations
We need to ensure that the current unit tests keep working and the changes on those is minimal and as 
non-functional as possible.

## Potential Alternatives
**As long as the requirements on top of this RFC are respected** any other way to do this refactor can be 
considered a "potential alternative".
