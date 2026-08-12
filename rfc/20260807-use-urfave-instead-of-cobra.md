# Using urfave/cli instead of spf13/cobra for the command line

> [!IMPORTANT]
This RFC has superseded [20251105-use-cobra-instead-of-mitchellh.md](20251105-use-cobra-instead-of-mitchellh.md)

As we started to investigate switching to cobra, one critical issue was found that prevented us from pursuing adoption of the cobra CLI. Under the hood, spf13/cobra uses spf13/pflags, a POSIX compatible alternative to the standard library flags package. With OpenTofu's wide adoption and integration into existing tooling, a swap to a completely different flag paradigm is not a realistic option.

With the go standard library flags, `tofu -json` is understood as `Flag(-json)`, while POSIX flags would understand it as `Flag(-j) Flag(-s) Flag(-o) Flag(-n)`. We had hoped to disable or work around that portion of POSIX integration in OpenTofu, but it would have had deep ramifications on everything from command handling to auto-complete.

Although discussed in the cobra RFC, the ramifications if this were not fully evident until we were mostly through a full draft implementation.

This RFC proposes using `urfave/cli` as an alternative, as it's flag parsing follows the go standard library conventions by default. If we ever decide to switch to true POSIX compatible flags in the future (tofu 2.0 at the earliest), there seems to be an option for that. It also has a similar feature set to cobra, is under a MIT license, and is actively maintained with a large user base.

Many of the same challenges exist in swapping out the backing command line implementation:
* Backwards compatibility
* Help text
* Autocomplete integration

## Backwards compatibility

We want to ensure that the command line experience is as close to the current implementation as we can reasonably get.

This means we want to ensure the following is as identical as possible
* Commands available with their appropriate sub-commands
* Command aliases
* Flags and arguments available for different commands
* Flag and argument validation and parsing logic
* TF_CLI_ARGS handling

Acceptable differences:
* Missing or incorrectly handled flags now present/functional
* Improved flag and argument validation and parsing logic

Given how the current arguments package is written, there are likely some interesting bugs to be identified. See implementation approach below for how these differences will be applied to both new and old implementations.

## Help Text

The help text in OpenTofu is manually created, formatted, and updated. This is a very tedious process and is quite error prone

Acceptable differences:
* Currently "hidden" options (not present in manual help text)
  - Unclear which ones are intentional omissions vs accidental.
  - We will need to discuss these on a case by case basis when reviewing the eventual implementation.
* Slight formatting differences in help text
  - Manual help text follows some rough conventions, but has variation and no true standard
  - Automatically generated help text would actually be standardized.

## Autocomplete integration

As with cobra, urfave/cli ships with a full autocomplete implementation, following a similar pattern.

Most modern tools follow the pattern of `tofu completion $SHELL_NAME` producing the autocomplete script to stdout. It is then loaded with `source <(tofu completion $SHELL_NAME)` or similar, either manually or via addition to the appropriate shell autoload file. It also may called by a package maintainer to ship as part of the packaging artefacts.

Unfortunately, neither cobra or urfave/cli supports editing the shell autoload files directly and is something we would likely have to write ourselves. The author of this RFC believes this should be a simple library outside of OpenTofu and upstreamed if possible.

One potential complication is that bash-completion ships with a [default for OpenTofu](https://github.com/scop/bash-completion/blob/585037356ba48fb37ea3df299e0a8177b3bed557/completions-fallback/Makefile.am#L315) and takes the form of `complete -C "\"$1\" 2>/dev/null" "$1"`. This means that most bash users will automatically be opted-in to the existing complete implementation.

Another potential complication is the migration process between the autocomplete scripts. Users who upgrade after the new command implementation is made default will have their tab completions broken if not properly addressed.

For the moment, the proposed solution is to use posener/complete with both the old and new command line implementations and revisit either improving the library or finding a safe way to switch to an alternative.

Some additional context can be found in https://github.com/scop/bash-completion/issues/1718, which suggests that urfave/cli has some key missing functionality that could make a full implementation difficult.  A big thanks to the maintainers of bash-completion for taking the time to help guide us through this process.

Potentially Future Solutions:
* Switch `-install-autocomplete` to always generate the new-style `tofu completion shell` approach
  - If we keep the existing CLI around for a release or two, patch `posener/complete` to produce the autocomplete scripts as text and add that subcommand to the existing CLI.
* Add detection for the `complete -C `/`$COMP_LINE` approach and handle it one of two ways
  - Print out a warning message that autocomplete needs to be reinstalled, this works well but is annoying to users.
  - Continue to use `posener/complete` as a fallback and build out what we need to fill in it's understanding of our command structure.


## Implementation Approach

There are several interacting components that need to be migrated and are detailed below.

### Command Arguments

Within `internal/command/arguments`, we define all of the arguments available to OpenTofu's commands.

Overall this is a reasonable approach, though the current implementation has some distinct limitations:
* It hardwired to use go's standard library flags package.
* It does not expose any of the information in a way that could be used to generate help text.
* It does not have consistent handling or error messages of positional arguments.
* It duplicates a lot of parsing and validation code (which has drifted over time).

We propose to change the purpose of this package slightly, from parsing and validating the raw command arguments directly, to exposing a structure that represents the logic and metadata for command arguments with raw argument parsers attached.

In more specific terms, the proposed `arguments.CommandLine` structure would have a form similar to:
```go
struct CommandLine {
  Flags      []Flag               // Metadata and implementation
  FlagGroups []FlagGroup          // Metadata
  Arguments  []PositionalArgument // Metadata and implementation
  Hooks      []Hook               // Validation, Parsing, Cleanup (for --json-into)
}

// Used during the migration but eventually removed
func (c CommandLine) Stdlib(args []string) tfdiags.Diagnostics {}
```

Instead of the current `Parse<Command>` functions, we would instead approach this as `Bind<Command>` functions in a form like:
```go
// Current implementation parses the raw args directly and returns the command's arg structure, a closer function for -json-into and diagnostics.
func ParseFmt(args []string) (*Fmt, func(), tfdiags.Diagnostics) {}

// Would become a function that binds the arguments to a command line and returns the bound structure.
func BindFmt(cli *CommandLine) *Fmt {}
```

Although they will be eventually removed, `Parse<Command>` would be kept temporarily and re-written to call the Binder and use the Stdlib functions to process the given args.

This approach has several distinct advantages:
* We can migrate the tests over time and don't need to do it all in one *massive* PR.
* The Metadata available can be used externally to build the full help text automatically!
* Any bugs fixed will apply to both paths into the arguments package (Bind or Parse)
  - With the downside that any newly introduced bugs will be also in both paths.
* The actual handling of flags and arguments is now up to the caller instead of being hardcoded.
  - If we ever need to migrate the cli package again, the churn to the codebase is minimal.
  - This was put into practice in the draft series of PRs that quickly pivoted from cobra to urfave/cli.

## Commands

Our goal for the `internal/command` package is to keep the changes as minimal as possible. Additionally, we would like to make the migration in several steps to keep the changes easier to review and test.

Proposed steps:
* Introduce a alternative way to represent a "Command" that includes:
  - Name and Descriptions
  - A bound `arguments.CommandLine`
  - A Run function that takes a pre-built `command.Meta` struct
  - A function to generate help text formatted in our existing style
* Modify existing `mitchellh/cli` Command.Run functions to wrap the new Command structures
  - This allows us to leverage all of our existing tests with minimal tweaks to ensure the new structure is correct.
* Switch tests over to run the new command structures directly instead of using the `mitchellh/cli` Command.Run functions.
* Remove `mitchellh/cli` related functions and any test quirks that resulted from that path.

## CLI Main

Under the assumption that we will not want to do a hard-cutover to the new command implementation, we propose to split the command integration out from the main function into two implementations. One that leverages `mitchellh/cli` and the other that uses `urfave/cli`.

This allows us to keep the existing code path working for as long as we like, while allowing to opt-in or out of the new cli via an environment variable (and potential build flag). This has distinct advantages for side-by-side testing during the migration period. It also allows us to keep the non-command specific logic in the existing main and not duplicated between code paths.

In practice, `urfave/cli` already supports most cli patterns and features that we need for OpenTofu. We can take the new style Commands described above and walk them to build the corresponding `urfave/cli` structs without much effort.

Eventually the `mitchellh/cli` will be removed, and the alternative code path along with it.

## Open Questions
* Should we switch directly over to the new implementation and not keep the legacy implementation?
  - The likelihood that the new CLI would block users is relatively low, especially given the implementation approach.
  - Approach that allows users to unblock themselves in case of a critical bug: [accepted by maintainers]
    - In v1.13, TOFU_EXPERIMENTAL_CLI_ENABLED=false to opt-out of the new CLI
    - In v1.14 remove the old CLI and the env var.

## Future Considerations

* This RFC deals with the logic around our command implementations, but does not propose cleaning up any of the existing deeper implementation quirks and spaghetti.
  - A future set of RFCs would be useful to describe the process of breaking down the meta struct and improving the underlying details of our command package.
* We could generate man pages automatically with `urfave/cli-docs`
* We could retire our custom help text formatting and switch to `urfave/cli`'s default formatter if desired.
  - The maintenance burden is low, but non-zero
* Also see items discussed in the original cobra RFC linked above.
