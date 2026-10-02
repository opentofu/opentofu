# Linting suppresion

(This RFC is primarily for GitHub [issue #2213](https://github.com/opentofu/opentofu/issues/2213) and the 2nd RFC on linting, next in line after [#3999](https://github.com/opentofu/opentofu/pull/3999).)

In the initial [linting RFC](https://github.com/opentofu/opentofu/blob/a7bb6584740c91579c31a984315fcd23534d7872/rfc/20260406-linting.md), were discussed different [possible
extensions](https://github.com/opentofu/opentofu/blob/a7bb6584740c91579c31a984315fcd23534d7872/rfc/20260406-linting.md#possible-future-extensions) for the linting functionality.

This RFC will explore one of the extensions: a linting suppression mechanism.

## Why the linting suppression before other extensions?

Even though in order to become more and more useful over time, the linting functionality should be extended with other ways of defining linting rules, there are several
reasons why I propose to postpone any of those extensions for the moment:
* The included `-lint=` flag is the only way to enable/disable/configure the linting functionality and that is on purpose to allow users to execute the 
  delivered core linting rules. But the flag provides only a limited configuration capability. For more advanced features to be developed, we would 
  need a more granular and capable configuration mechanism for the linting functionality.
* For a more seamless integration and to avoid designing a new propagation channel for the linting issues, the current used channel is through the existing 
  diagnostics implementation. With this being delivered in v1.13, we expect to have various requests on the limitations user will find about this and we want 
  to gather as many use cases as possible to be able to decide on the right approach going forward.  

Most of the extensions proposed in the previous RFC ([providers-based lint rules](https://github.com/opentofu/opentofu/blob/a7bb6584740c91579c31a984315fcd23534d7872/rfc/20260406-linting.md#provider-based-lint-rules), [in-configuration lint rules](https://github.com/opentofu/opentofu/blob/a7bb6584740c91579c31a984315fcd23534d7872/rfc/20260406-linting.md#in-configuration-lint-rules), [fix-it hints](https://github.com/opentofu/opentofu/blob/a7bb6584740c91579c31a984315fcd23534d7872/rfc/20260406-linting.md#fix-it-hints)) require 
the existence and stability of a more granular configuration option and a stable propagation channel of the linting issues.

Therefore, until we gather enough feedback to be able to design a comprehensive solution for the linting configuration and propagation channel, 
we would like to focus on the linting suppresion support.  

## Proposed solution

In the existing linting implementation, the only way to disable the reporting of a linting issue that wants to be ignored is to not execute that linting rule at all, either by excluding it from the
rules executed or skipping the linting feature entirely.

But the option above disables the unwanted rule for the entire configuration and not only a particular block of the configuration.
Additionally, if the linting is enabled for a CI pipeline, the user might not even have the option to control what rules are executed. 
Therefore, it will not be able to act on a false-positive for its configuration.

For that, we would like to add a functionality that could encompass also a linting suppression mechanism.

The control of the linting rules execution should be allowed from two places:
* If we think of the linting as a policy enforcer mechanism, the control should be allowed in a global space, where organisation wide
agreed linting rules to be executed can be defined and enforced;
* Then, thinking of the more granular control that users might need, it should be allowed at the configuration level too where users can disable 
(or enable) specific rules in particular places of the configuration, when those specific rules are not enforced by the global space configuration.

The enforcing is the mechanism through which a linting rule enable/disable status can be configured in a global space and cannot 
be overwritten by the in-configuration directives. Doing so, will result in an error.

That being said, to be able to achieve this, the following rules should be able to be expressed through the proposed mechanisms:
* By default, all rules are disabled. This will ignore any of the in-configuration directives;
* Only global space can enable the linting functionality:
  * In the initial version, it will work alongside `-lint=` flag and the flag will have priority if specified, overwriting anything configured in the global space. 
* In the global space, linting rules can be enabled or disabled;
* In the global space, any of the enabled/disabled linting rules could be also **specifically** enforced:
  * If a linting rule is enabled and enforced, the in-configuration directives cannot disable it, doing such resulting in an error;
  * If a linting rule is disabled and enforced, the in-configuration directives cannot enable it, doing such resulting in an error.
* The in-configuration directive could enable additional linting rules, if and only if, that particular linting rule is not specifically disabled and enforced **and** the global space configures linting as enabled;
* The in-configuration directive could disable from the enabled linting rules in the global space, if and only if, that particular linting rule is not enabled and enforced **and** the global space configures linting as enabled.

Allowing the granularity of the rules above requires some specific keywords:
* `disable` - attribute used in the global space to enable or disable a specific rule. It can be used in the in-configuration directive to disable execution of a linting rule on a specific place in the configuration;
* `enforce` - attribute in the global space configuration to block the overriding of the rule enabling status by using directives;
* `enable` - keyword used in the in-configuration directive to enable a specific rule if it was disabled from the global space configuration and not enforced.

### Global space 

The global space configuration for linting would look as follows:
```hcl
linting {
  enable = true # The value of this should be configurable by environment variable too (eg: TF_LINT=true)
  rules {
    "core:all" = {} # this configuration enables all the core linting rules since it defines no `disable=true`
    "core:no-type-variable" = { # enables the rule and does not allow to be disabled
      enforce = true
    }
    "core:count-instead-enabled": { # disables this rule and does not allow to be enabled
      disable = true
      enforce = true
    }
    "core:unused-variable": { # disables this rule but allows it to be enabled from the configuration
      enforce = false
    }
  }
}
```

As this RFC is not about the linting configuration, the place where such a configuration could be defined will be decided 
in a future RFC when we will define the more granular configuration for linting.
For the moment, the options where such a configuration could be stored are as follows:
* CLI configuration
* A new specific configuration file made only for the linting
* In the `language` block of the root module

### In-configuration directives
From the newly introduced keywords, 2 of them can be used as directives: `disable` and `enable`.

A in-configuration directive is a comment that uses the Unix-style shell comment, followed by a keyword and the linting rule that it refers to.
It has the following format:
```shell
#<keyword>(<rule_id>): <comment>
```
> [!NOTE]
> Since HCL exposes the comments with also the leading comment marker, I would recommend to allow only Unix-style shell comments
> to be considered as "directives".

To silence a particular linting rule warning, `disable` directive can be used:
```hcl
#disable(core:all): in this particular case, this directive supresses 2 possible warnings: core:no-type-variable, core:unused-variable
variable "in_string" {
  default = "input"
}

#disable(core:no-type-variable)
variable "in_number" {
  default = 42
}
```

The `enable` directive is meant to reactivate a linting rule disabled through the global space configuration while the linting functionality is enabled:

_global space configuration_:
```hcl
linting {
  enable = true # Linting is enabled
  rules {
    "core:no-type-variable" = {
      # disables the rule from the global space
      disable = true
    }
  }
}
```

_user configuration_:
```hcl
#enable(core:no-type-variable): enable it back for this particular variable
variable "in_number" {
  default = 42
}
```

If the rule is later enabled in the global space, the existing `enable` directives become moot.

### User Documentation

When a user wants to exclude a specific linting issue from generating warnings, the user will have the option to add a comment
**above the line that is shown in the diagnostic**, instructing OpenTofu to skip generating the warning diagnostic for it.

OpenTofu will parse any directive, but its applicability is restricted solely to the concept it precedes. 
The reason for this is to encourage carreful consideration whenever such a directive is used and by not allowing it to affect a wider
scope, it also removes the risk of hiding other issues that the user didn't initially intent to supress the warnings for.

Some ways the directives can be used:
```hcl
#disable(core:all): in this particular case, this nolint directive supresses 2 possible warnings: core:no-type-variable, core:unused-variable
variable "in_string" {
  default = "input"
}

#disable(core:no-type-variable)
variable "in_number" {
  default = 42
}

locals {
  #disable(core:unused-local)
  temp = var.in_number
}

resource "terraform_data" "all_answers" {
  #disable(core:count-instead-enabled): I need this to be portable
  count = var.in_number == 42 ? 1 : 0
}
```

The directive comment will be recognised only when it follows the following format:
```hcl
#enable(<linting rule identifier>)[: reason]
#disable(<linting rule identifier>)[: reason]
```

In order to supress correctly the linting warnings, it needs be present right above the line indicated by the linting diagnostic.

Invalid usage:
```hcl
#disable(core:all): in this particular case, this directive supresses 2 possible warnings: core:no-type-variable, core:unused-variable
#
# This is another comment that will break the supression of the nolint directive above
variable "in_string" {
  default = "input"
}

variable "in_number" {
  # Because the core:no-type-variable linting rule diagnostic refers to the whole variable block, having it added inside the block will not be used to supress the warning 
  #disable(core:no-type-variable)
  default = 42
}

# core:unused-local points directly to the local variable declaration. nolint directive at the `locals` block level will have no effect in the linting warning supresssion
# disable(core:unused-local)
locals {
  temp = var.in_number
}

# Since OpenTofu does not look for block level nolint directives and because the core:count-instead-enabled linting rule warning points directly to the `count` meta-argument, the usage of nolint directive at this level will have no effect in the suppression
#disable(core:count-instead-enabled): I need this to be portable
resource "terraform_data" "all_answers" {
  # Since the format of a directive requires no space between the comment token and the `disable` keyword, this will be discarded
  # disable(core:count-instead-enabled): I need this to be portable
  count = var.in_number == 42 ? 1 : 0
}
```

### Technical Approach

One of the most important technical concern is to expose the comments parsed from files. A quick POC can be found [here](https://github.com/opentofu/hcl/compare/opentofu...opentofu:hcl:poc-collect-comments-too).

With the comments exposed, then those will be collected into the [parsed module](https://github.com/opentofu/opentofu/compare/8368dc8f09d8b2863b88d6373c1076f548ac638d...poc-linting-nolint#diff-ae965095e04b6e1ad339db43a381b3873408d5e18f5b0cdaf29853bc7a0d0f64R79) 
during the configuration parsing and later injected into the [linting context](https://github.com/opentofu/opentofu/compare/8368dc8f09d8b2863b88d6373c1076f548ac638d...poc-linting-nolint#diff-523e4fe1e28b881a87526c2c7c97a796ffe0dc27029816548709479103503167R249):
```go
// LintingControl is a type that describes an instance of a `#<enable/disable>(<for rule>): <reason>` entry.
// This holds the declaration place, the rule ID that this is for and the reason
type LintingControl struct {
  controlType lintingControlType
  Decl        hcl.Range
  ForRule     linting.RuleAddr
  Reason      string
}

// ContextWithLintingControls stores the LintingControl slice into the linting context. These will be later to skip the linting rules execution
// indicated by the given directives.
func ContextWithLintingControls(parent context.Context, controls []LintingControl) context.Context {
	v := lintHintsFromContext(parent)
	if v == nil {
		return parent
	}
	v.lintingControl = lintingControl
	return parent // no need to create a new context. The hints from the context is a pointer so we just store the directives into that.
}
```

With that information in the linting context, then the execution can mark the linting rule as executed and generate no diagnostics.

An OpenTofu POC for this can be found [here](https://github.com/opentofu/opentofu/compare/9ffbb1c211bfb36775c5923809a18289396ee0c1...poc-linting-nolint).

In later interations, when in-provider linting will be introduced, the control information will have to be embedded into the
validation request, per resource, and the provider will be responsible with **not generating** those linting warnings.
The reason for suggesting this approach is to allow better performance when some linting rules execution could be time or resource hungry.
As a protective measure, OpenTofu can also filter out any linting diagnostic that a provider returns but which are controlled by `disable` directive.
But this is a concern that must be part of the architecturing effort of the in-provider linting design.

### Open Questions
* This is a functionality that will not be available in the JSON configuration representation. What is your take on this?
* The current proposal is kinda "naive": it doesn't bind the directive to a specific field or block from the get go. Instead, 
  right before executing the linting rule, it checks the directive entries and if it finds one **on the line above** of the construct 
  that the linting rule is scheduled to run against, it skips the execution and marks it as successfully executed without issuing any diagnostics.
  * I would be curious on what do you think about this. I incline on making the directives parsing a little bit more "binding" to the construct it is declared on.
    Meaning that during execution, the directives could be detected based on the line that it was pre-bound to during configuration parsing.
    Right now I feel that I talk myself into this approach more than the rather raw solution presented above, since this would be a also a good prerequisite 
    for the in-provider linting rules.

### Future Considerations

* The current RFC specifically states that block level directives are strictly for resource level related linting rules and does not act as a 
  an umbrella suppression mechanism for any of the possibly in-block contained linting issues. In the future, based on the feedback and based on what other
  linting functionalities would require, this might change (see also the next point in this list).
* When in-provider linting rules will be implemented, we will have to allow resource block level directive because those will be attached to the 
  linting request and the provider will have to skip running the rules that are excluded by the nolint directive.
  * This is required because the source information is not sent to the provider. The provider receives only the value of the configuration and not source information. 
    Therefore, the provider cannot tell what argument level a `disable` directive applies to.
    * Another approach would be for OpenTofu to associate the directives with the block or the argument and include the association into the request. This way it will allow
      the provider more granular control of what linting rule should be excluded for a particular argument, which is the actual intended behavior, in contrast with the initial
      idea from above, where the resource level directive for a particular rule would be applied to any issue that rule might find.

## Potential Alternatives

_[extract](https://github.com/opentofu/opentofu/blob/a7bb6584740c91579c31a984315fcd23534d7872/rfc/20260406-linting.md#new-language-annotations-implementation) from the previous linting RFC_

Another way to implement such a feature would be to introduce a new language annotation.

The way this might look:
```hcl
variable "in_string" {
  type    = string
  default = "input"
}

@disable(untyped_variables): don't really want to add a type. It's too verbose
variable "in_number" {
  default = 42
}

resource "random_id" "test" {
  @disable(core:ineffeq): we know about this and the behavior matches our use case
  prefix = var.in_string == var.in_number ? "apply this prefix" : "otherwise this one"
  @disable(core:impurefunc): this is ignored on purpose
  byte_length = tonumber(core::split("-", core::timestamp())[1])
}

ephemeral "random_password" "test" {
  length = 10
  upper  = true
}
```
To allow this, HCL would have to be updated quite heavily. The new syntax would require careful integration in the current HCL keywords and parsing.

Pros:
* Native support which is more integrated with the other blocks and concepts
* This would add a new language feature that might prove useful in the future
* More visible and discoverable compared with the comments-based annotations

Cons:
* Additional incompatibility with the predecessor project;
* If we want to also integrate `tflint` `nolint` directive parsing, we would still need to have a similar implementation with the one proposed above;  
* Implementation could prove to be quite tedious and dangerous as it could affect other parts of the HCL language.

