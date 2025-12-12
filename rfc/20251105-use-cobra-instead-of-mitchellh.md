# Exploring [cobra](https://github.com/spf13/cobra) replacement

Tickets that are targeted to be resolved by this:
* https://github.com/opentofu/opentofu/issues/748
* https://github.com/opentofu/opentofu/issues/3500
* https://github.com/opentofu/opentofu/issues/2239

When OpenTofu predecessor project started, the options for CLI libraries were slim and were lacking 
features heavily.
Because of that, [mitchellh/cli](https://github.com/mitchellh/cli) has been created and served its purpose.
Now, it is archived and some dependencies of that (e.g.: https://github.com/posener/complete) are 
not maintained anymore, putting OpenTofu in a weird position where, in order to be able to improve on 
layers controlled by it, we would need to either fork and maintain those ourselves or switch to 
another maintained library.

The main road blocks we hit because of using this old library are as follows:
* Deprecated autocompletion scripts for zsh.
  * More details in the [autocomplete](#autocomplete) section.
* Hardcoded handling of the autocompletion scripts. The library used (https://github.com/posener/complete) is writing these scripts to hardcoded file paths without any option to specify the stream to be written into.
* Flags parsing is scattered all over the place and help text is written and _formatted_ manually for each and every flag.
* The flags format we have is the golang style which is way more uncommon compared with the POSIX style one.

In this RFC, after several in-depth tests, I want to share a possible solution of switching 
to [cobra](https://github.com/spf13/cobra).

I chose cobra for this because of its adoption in the go community, of its vast catalogue of features, 
and for its extensibility. 
In this RFC I want to list some of the approaches that we could take on making this transition 
and what would be the steps for it.

## Backwards compatibility
From the get go, we need to be clear that by attempting such a change, we need to be mindful about 
the risks and the promises that we need to keep about the UI of OpenTofu.

Risks and careful consideration:
* flags parsing could result in different parsed values because of some structures that we have today.
* flags ordering and propagation between commands could be slightly different. This would be handled by cobra instead and not by custom implementation that we have today.
* autocompletion by using the previously installed scripts could be broken

> [!NOTE]
> Recommended is to add unit tests for these risks before doing any changes.

Ensure that:
* once a user will switch to the new version, no change will be needed for it to work as before.
* all the features should work the same as they did before the switch:
  * flags should be parsable with single dash in front (e.g.: `-flag`).
  * autocomplete should work with the previously installed scripts.
  * the help function(s) should print the same way. The only acceptable change could be that the text will be correctly (automatically) wrapped at 80 chars. 

## Proposed Solution

### Autocomplete
In order to preserve the current functionality and the one provided by cobra we would have to create 
some kind of a "bridge" between the two.

This means that when autocompletion scripts are already installed into a system by an old `tofu` binary,
we could forward the requests to `posener/complete` to ensure that the users that didn't update yet 
the autocompletion scripts, will still have the autocompletion work properly.

To be able to do so, understanding how the options work is crucial.
#### posener/complete
`posener/complete` has been created initially as a pure bash autocomplete library.
Because of that, it relies heavily on the bash inner works to provide this capability, by using two
[environment variable](https://github.com/posener/complete/blob/9a4745ac49b29530e07dc2581745a218b646b7a3/complete.go#L85):
* `COMP_LINE`
* `COMP_POINT`

As the library evolved, it included support for others shells (fish, zsh) but it didn't add
official support of those, but took the shortest path on achieving this capability by exporting
the env vars aforementioned before calling the binary to provide suggestions (E.g.: [`fish` generated script](https://github.com/posener/complete/blob/9a4745ac49b29530e07dc2581745a218b646b7a3/cmd/install/fish.go#L57)).

In the same time, to make this work for `zsh`, it's not using the built-in functionality for 
autocompletion, but instead [it loads the `bashcompinit`](https://github.com/posener/complete/blob/9a4745ac49b29530e07dc2581745a218b646b7a3/cmd/install/zsh.go#L25) 
to achieve this (which is `bash` specific library that `zsh` offers for [compatibility purposes](https://zsh.sourceforge.io/Doc/Release/Completion-System.html#Use-of-compinit)).

#### spf13/cobra
Cobra instead, relies on an automatically included, hidden [`__complete` command](https://github.com/spf13/cobra/blob/fc81d2003469e2a5c440306d04a6d82a54065979/completions.go#L242).
This command contains a generic logic for handling autocompletion based on the arguments received.
The scripts that are generated for each supported shell are baked in cobra, use official completion
APIs of the targeted shell and converts the specific shell information in values understandable 
by the `__complete` command.

Here is a list of the scripts cobra generate and their associated official documentation for reference:
* [bash](https://github.com/spf13/cobra/blob/fc81d2003469e2a5c440306d04a6d82a54065979/bash_completionsV2.go#L37) - official docs ([programmable completion](https://www.gnu.org/software/bash/manual/html_node/Programmable-Completion.html) and [builtins](https://www.gnu.org/software/bash/manual/html_node/Programmable-Completion-Builtins.html))
  * Uses `complete` builtin function together with other functions to provide the aforementioned env vars for the target application to provide completion suggestions.
* [zsh](https://github.com/spf13/cobra/blob/fc81d2003469e2a5c440306d04a6d82a54065979/zsh_completions.go#L92) - [official docs](https://zsh.sourceforge.io/Doc/Release/Completion-System.html)
  * To be able to use `source <(tofu completion zsh)`, it uses the `compdef` function as the second line of the completion script. This works similarly with the `complete` command in bash.
  * To allow zsh recommended way to install these scripts, the first line from the cobra script
    contains [`#compdef` directive](https://zsh.sourceforge.io/Doc/Release/Completion-System.html#Autoloaded-files). This is used by zsh to lazy load the script.
* [fish](https://github.com/spf13/cobra/blob/fc81d2003469e2a5c440306d04a6d82a54065979/fish_completions.go#L36) - [official docs](https://fishshell.com/docs/current/completions.html)
  * It uses its own [flavored `complete` function](https://fishshell.com/docs/current/cmds/complete.html) that can be used to handle the completion for a program.
* [powershell](https://github.com/spf13/cobra/blob/fc81d2003469e2a5c440306d04a6d82a54065979/powershell_completions.go#L38) - [official docs](https://learn.microsoft.com/en-us/powershell/module/microsoft.powershell.core/register-argumentcompleter?view=powershell-7.5)
  * Uses Microsoft PS Core `Register-ArgumentCompleter` to register a completion script that calls into the cobra logic.

One possible downside of the cobra approach around this is about the length of the scripts. 
Because for each shell, the script is responsible with converting the shell specific information 
in arguments for the cobra command, the scripts tend to be quite complex.
But the advantage of having those scripts baked in cobra is that the maintenance of those is 
ensured by the community supporting cobra from different shell communities. 
Therefore, for the foreseeable future, the scripts will be updated with the best practices for each 
shell and we can inherit those with each update of the library. 

#### "Bridge" between old and new autocompletion scripts 
To ensure backwards compatibility, `posener/complete` should be the first called to provide autocompletion
suggestions if possible.
When it will provide none, we will allow cobra to execute, which internally will provide suggestions
if `__complete` command has been invoked.
The logic for such a "bridge" is already in [`mitchellh/cli`](https://github.com/mitchellh/cli/blob/main/cli.go#L408-L416).
The idea is quite simple: since `posener/complete` relies on the `COMP_LINE` env var to execute its
logic, it checks if that is configured and if it isn't returns `false` announcing that it could not 
provide suggestions.
The idea that I sketched is exactly like that, where I do the same check, and if it returns `true`, I
build the autocompletion context required by `posener/complete` and run its logic. 
Check [this](https://github.com/yottta/cobra_tofu/blob/ea1564d3542062f8016a5f75d6559c498c17797d/commands/autocomplete_legacy.go#L12-L14) for details.

Another advantage of using cobra is that the autocompletion scripts can be written to any buffer 
(default stdout), which allows us to do several things:
* allow users to [`source`](https://www.gnu.org/software/bash/manual/html_node/Bash-Builtins.html#index-source) the scripts directly, without the need to write those to a file. Helpful
  when OpenTofu is executed on a system where the files like `.zshrc` are read only.
* allows generating the scripts right before the release and package those in the delivery archives
  for each OS.
  * a good idea got during reviews, was to not generate the scripts during the release, but instead 
  to generate the scripts by using `go generate` in the normal development flow
  and include the files in the final binary and serve it like that. 
  This way, we can configure [goreleaser](https://github.com/opentofu/opentofu/blob/ee0029965f332f2deddc67ab7bc78fa32a970696/.goreleaser.yaml#L298-L302) 
  to include the already generated files without relying on the binaries compiled during release.
  An added benefit in doing this is that the scripts will be reviewable before release.
  Such an example can be seen in the [experiment repo](https://github.com/yottta/cobra_tofu/commit/52fbd6ed1aee8d6fa5c28c423300797363bcaca5).

Cobra offers a `ValidArgsFunction` that can calculate on the fly the valid arguments for a command.
One such example can be seen in [`tofu workspace select`](https://github.com/yottta/cobra_tofu/blob/ea1564d3542062f8016a5f75d6559c498c17797d/commands/cmd_other.go#L66-L68).

### Flags
When it comes to flags, this is more of a sensitive topic since these control OpenTofu in a more
functional manner, so we need to ensure that the parsing of the values is not affected.
One other thing that we need to be sure of, is that the position of these will not raise errors
to the users once they start using a new version that will include the cobra integration.

#### Problem statement
> [!NOTE]
> The issue with how **in general** flags are handled in OpenTofu, is a topic for another RFC.
> 
> In this one we will explore only the challenges of migrating from the flags format we have to 
> POSIX compliant ones.

Because OpenTofu uses [golang stdlib flag package](https://pkg.go.dev/flag), and because that lib 
supports long format flags with a single dash in front (e.g.: `-flagname`), the migration to a POSIX
compliant format (e.g.: `--flagname`) is more challenging especially since we don't want to break already
configured CI/CD that uses go style flags.


In order for tidying up and making the flags handling more clear in OpenTofu, we strive towards
defining all the flags by using [spf13/pflag](https://github.com/spf13/pflag), which works
hand in hand with [spf13/cobra](https://github.com/spf13/cobra).

The next two sections shows different approaches on achieving a migration without breaking existing flows.
#### Copy `spf13/pflag/FlagSet` to `flag/FlagSet`
This was a first attempt since the golang style flags do support [single and double dashes for
the defined flags](https://pkg.go.dev/flag#hdr-Command_line_flag_syntax) (getting us closer
to POSIX formatted flags).

`spf13/pflag` offers a functionality to [copy the flags](https://github.com/spf13/pflag/pull/330/files#diff-8662a44abae11986156e88dd0d690ae1b76ffa61d2f02909979b2c24fcce22f5R121) defined into the golang
standard [lib `flag.FlagSet`](https://pkg.go.dev/flag#FlagSet).

This would achieve 2 things:
* Common way to define flags by using `pflag`
* A backwards compatible way to parse flags from the arguments

One downside that I encountered while testing this is related to the integration of this approach
in cobra.

To be able to use this approach, we would do the following:
* defined flags using `pflag`
* _disable cobra flags parsing_
* copy the flags to stdlib `flag.FlagSet`
* use the stdlib `flag.FlagSet.Parse(os.Args)`

Because of the 2nd step, since cobra does not parse the flags from the args, it will pass all the args
(including flags) into the args of the execution function (`Run`, `PreRun`, etc) of each command 
(root, sub command, etc).

For example, having this command line:
```
tofu -chdir=test apply -auto-approve planfile
```
By having the flags parsing disabled, all the execution functions of the commands will receive exactly
the same args slice: `["-chdir=test", "-auto-approve", "planfile"]`.
I am talking here about the `rootCmd.PersistentPreRun`, `rootCmd.Run/RunE` and `applyCmd.Run`, etc.

This adds maintenance cost, mixing of concerns and a quite tangled way to have logic bound to the flags.

> [!NOTE]
> As noted in the [official documentation](https://github.com/spf13/cobra/blob/main/site/content/user_guide.md#local-flag-on-parent-commands),
> tried also to use `Command.TraverseChildren` to force args given to each command object to contain
> only its defined flags, but that works only when `Command.DisableFlagParsing = false`, which breaks
> the first requirement we have to be able to copy the flags in the `flag` stdlib.

#### Use the spf13/pflag parsing
To use this approach, is quite straight forward:
* For each command define its flags
* Those will be parsed before execution of the command

One big benefit that we have with this is that combined with `Command.TraverseChildren`, each `*cobra.Command`
object receives strictly the arguments and not the flag values, flags being already parsed and injected
in the structs used to configure the flags.

An additional advantage compared with the copying of the flags, is that for any complex flag configured
with a [custom type behind right now](https://github.com/opentofu/opentofu/blob/c3fe83a177f4c49609a46302bec13de61d2c90cf/internal/command/meta_config.go#L523-L528), we avoid having its backing structure handle the parsing in unknown
ways.

Probably till now, you asked yourself, ok, but what about backwards compatibility?
For this approach to work, the backwards compatibility would be covered by a silly hack visible [here](https://github.com/yottta/cobra_tofu/blob/ea1564d3542062f8016a5f75d6559c498c17797d/main.go#L32-L44).

Considering that right now all the OpenTofu's flags are long form, ensuring that each argument that looks
like a single-dash flag should be converted to a double-dash one, is safe enough to unlock all the
inner functionality of cobra.

For the `TF_CLI_ARGS` I don't have a specific proposal, but there are multiple ways of 
doing it:
* similar with what we have today where we alter the `os.Args` before executing cobra command
* process this in the executed command `PreRun` function
* parse again the command defined flagset against the `TF_CLI_ARGS` env var and merge
  it with the struct that was parsed directly by cobra (following the precedence in place right now)
* out of this scope, but we could use also [viper](https://github.com/spf13/viper) that
  can bind to flags and when configs loaded, [it follows the same precedence as we have now](https://github.com/spf13/viper?tab=readme-ov-file#putting-values-in-viper).

### Help text
Once we have all the things mentioned above, cobra offers the option to customise the help
by our needs.
There is already a good [example](https://github.com/yottta/cobra_tofu/blob/ea1564d3542062f8016a5f75d6559c498c17797d/commands/help_text.go#L17) 
wrote in my experiment. The function is [configured on the root command](https://github.com/yottta/cobra_tofu/blob/ea1564d3542062f8016a5f75d6559c498c17797d/commands/cmd_root.go#L61-L64) 
and any other sub command makes use of it when `-h/--help` is provided.

For backwards compatibility of this part, we should render the flags with one dash only, but
personally, I am inclined generating these with 2 dashes in front to have a smooth 
transition for the newcomers.

### Open Questions
* Why there are flags that are hidden or never visible? Like `-install-autocomplete`.
  * Asking because I would like to make those visible with this change

### Future Considerations
This RFC is tackling the most important aspects of such a migration but there are still
a lot to be done around details of this:
* TF_CLI_ARGS
* Streams and View/UI proper handling
* CLI configuration before chdir and provider sources
* TF_REATTACH_PROVIDERS
* cleanup provider clients
* Suggestions for misspelled commands. Something like [this](https://github.com/spf13/cobra/commit/046a67325286b5e4d7c95b1d501ea1cd5ba43600)

We might consider including [opentofu#3050](https://github.com/opentofu/opentofu/issues/3050) under these
changes too.

When implementing this, we might want to consider having this under an experimental
flag that will allow users to opt-in the new CLI integration for one minor version,
and in the next minor version, we can change the meaning of the experimental flag from
enabling the new CLI to enable the old CLI, making the new implementation the default
one.

## Potential Alternatives
* Do not migrate to a new library and try to do the most we can by forking the existing ones
  and just add the missing features
* Do nothing :(
* Check other libraries

### Additional resources
Cobra completion command reference ([link](https://github.com/spf13/cobra/blob/main/site/content/completions/_index.md)).
Cobra user guide ([link](https://github.com/spf13/cobra/blob/main/site/content/user_guide.md#local-flag-on-parent-commands))