# OpenTofu Specific Code Override

Issue: https://github.com/opentofu/opentofu/issues/1275 

## Motivation
Today, OpenTofu uses the `.tf` extension files as the main configuration files. This is the result of forking Terraform ~1.5.5. As time goes by, the OpenTofu code starts to diverge from the original Terraform codebase in different ways:
1. OpenTofu adds new features that are not supported by Terraform, like [State Encryption](https://opentofu.org/docs/language/state/encryption/) and [Dynamic Provider-Defined Functions](https://opentofu.org/docs/intro/whats-new/#provider-defined-functions).
2. OpenTofu and Terraform are likely to have similar features with different implementations and syntax usage. For example, OpenTofu's [Removed block](https://opentofu.org/docs/intro/whats-new/#removed-block).
3. In the future, Terraform might introduce new features that are not supported by OpenTofu.

This divergence between OpenTofu and Terraform configuration syntax imposes challenges on the following personas:
1. Module maintainers who want their modules to be compatible with both OpenTofu and Terraform.
2. Users who want to use OpenTofu, but are missing IDE and 3rd party tools support for OpenTofu configuration files.
3. 3rd party tools maintainers who want to support both OpenTofu and Terraform.

## Proposed Solution

In addition to supporting `.tf` extension files as we do today, OpenTofu will support `.tofu` extension files. When two files have the same name, one with the `.tf` extension and one with the `.tofu` extension, OpenTofu will load only the `.tofu` file and ignore the `.tf` file. Hence, creating a mechanism that overrides specific files. This way the `.tofu` extension files will be used for OpenTofu specific features and syntax and the `.tf` extension files can be used for Terraform compatible features and syntax. Allowing users and module maintainers to use OpenTofu and Terraform in the same project without conflicts. 

For example, If I have two files:
```hcl
// vars.tf

variable "region" {
  type = string
}
```

```hcl
// vars.tofu

variable "region" {
  type = string
  deprecated = "Reason this was deprecated"
}
```
Terraform would read all files including `vars.tf` and ignore `vars.tofu`.
OpenTofu would read all files, but replace `vars.tf` with `vars.tofu` and will include the additional deprecated attribute.


> [!NOTE]
> This solution should not be confused with the [Override Files feature](https://opentofu.org/docs/language/files/override/) that is already supported in OpenTofu. The Override Files feature is used to override specific portions of an existing file, while the proposed solution here will override the **entire file**. We chose to implement it this way because Terraform may introduce features in the future that OpenTofu cannot parse, making specific overrides insufficient.

## User Documentation

### Supported Scenarios
As we know, `.tf` are not the only files that OpenTofu can load, in here we will drill down into all the supported scenarios:
1. This is the basic scenario we mentioned before, if a `.tofu` file exists, we’ll load it instead of the `.tf` file.
2. For [JSON Configuration](https://opentofu.org/docs/language/syntax/json/), if a `.tofu.json` file exists, we’ll load it instead of the `tf.json` file. 
3. For [test files](https://opentofu.org/docs/cli/commands/test/), if `.tofutest.hcl`/`.tofutest.json` files exist, we’ll load them instead of the `.tftest.hcl`/`.tftest.json` files. 
4. For [override files](https://opentofu.org/docs/language/files/override/), if `_override.tofu`/`_override.tofu.json` files exist, we’ll load them instead of the `_override.tf`/`_override.tf.json` files. To further clarify, if a `main.tofu` file exists with the override file `main_override.tf`, we’ll load them both (unless `main_override.tofu` exists, and then we'll load it instead of `main_override.tf`).
5. For [variable definitions files](https://opentofu.org/docs/language/values/variables/#variable-definitions-tfvars-files), if a `.tofuvars` file exists, we’ll load it instead of the `.tfvars` file.
6. For [JSON variable definitions files](https://opentofu.org/docs/language/values/variables/#variable-definitions-tfvars-files), if a `.tofuvars.json` file exists, we’ll load it instead of the `.tfvars.json` file.

### User Stories
As part of this RFC we'll address the following user stories, and try to understand the user experience and how the suggested solution will impact our users:
1. I’m a module developer who wants to write a module supporting TF and Tofu (I'm writing my module from zero / extend an existing TF module to support Tofu).
2. I’m a module developer/user who wants to write a module/configuration supporting only Tofu.
3. I’m a TF user who wants to switch to Tofu and start using new Tofu features, without losing the ability to go back to TF (or making sure it is as simple as possible to switch back). Maybe I’m just experimenting and assessing whether I want to move to Tofu.
4. I’m a Tofu user who wants to write Tofu code as efficiently as possible, using language servers and extensions in my IDE and external tools.

#### I’m a module developer who wants to write a module supporting TF and Tofu
**Today:**  
As a module developer, I want to start supporting OpenTofu in my module, so my users can use it with both OpenTofu and Terraform. Today, my module automatically supports both Terraform and OpenTofu for features that were released until ~1.5.5 for both projects. The problem starts when I want to use new features that were released in versions > ~1.5.5:
1. If I want to use a new OpenTofu feature that is not supported by Terraform (for example, [urldecode function](https://opentofu.org/docs/language/functions/urldecode/)), I can't use it in my module because it will break the Terraform compatibility.
2. If I want to use a compatible feature that was released in both OpenTofu and Terraform, I might still have an issue because the versions that include this feature in both projects might be different (for example, [Provider-Defined Functions](https://opentofu.org/docs/language/functions/#provider-defined-functions) was released in OpenTofu v.1.7.0 and in Terraform 1.8.0). This causes me a problem when setting the `required_version` [setting](https://opentofu.org/docs/language/settings/#specifying-a-required-opentofu-version) as I can't define two different minimum versions for OpenTofu and Terraform in the same module.

**With this solution implemented:**  
1. If I want to use a specific OpenTofu feature in one of my module files, I can create two copies of the file - a `.tf` file and a `.tofu` file with very similar content. The only difference is that the `.tofu` file will contain OpenTofu specific features that are not supported by Terraform. This way, I can use the new features without breaking my module's compatibility with Terraform.
2. If I want to set a minimum version for Terraform and OpenTofu for my module, I can create a`.tf` file and a `.tofu` file both containing:
    ```hcl
    terraform {
      required_version = ">= x.x.x"
    }
    ```
   In the `.tf` file I'll put my minimum Terraform version, and in the `.tofu` file I'll put my minimum OpenTofu version. This way, I can set different minimum versions for OpenTofu and Terraform in the same module.

#### I’m a module developer/user who wants to write a module/configuration supporting only Tofu
**Today:**  
Unfortunately, I don't have a simple way to do it today. I cannot limit my modules/configurations to be loaded only by OpenTofu.

**With this solution implemented:**  
If I include only `.tofu` files in my module/configuration, I can be sure it'll be supported only by OpenTofu. This way, I can use OpenTofu-specific features without worrying that others might run the module/configuration by mistake with Terraform instead of OpenTofu.

#### I’m a TF user who wants to switch to Tofu and start using new Tofu features, without losing the ability to go back to TF
**Today:**  
I can [migrate to OpenTofu](https://opentofu.org/docs/intro/migration/) very easily, and mostly keep my configuration the same way it is with minimal required [code changes in some cases](https://opentofu.org/docs/intro/migration/terraform-1.8/#step-5-code-changes). But for using OpenTofu-specific configuration I still need to change my existing configuration files.

**With this solution implemented:**  
I have another option for the migration process. Instead of changing my existing `.tf` configuration files to try OpenTofu-specific features, I can create a new `.tofu` file with the same content as the `.tf` file and start using OpenTofu-specific features in the `.tofu` file. This way, I can experiment with OpenTofu-specific features without changing my existing `.tf` files. If I decide to go back, I can simply delete the `.tofu` file and keep using the `.tf` file.

#### I’m a Tofu user who wants to write Tofu code as efficiently as possible, using language servers and extensions in my IDE and external tools
**Today**:  
I can write OpenTofu code in my IDE, but I don't have complete support for new OpenTofu features. For example, my IDE will not recognize the syntax of [State Encryption](https://opentofu.org/docs/language/state/encryption/) and will show a warning when I try to use it. This makes it harder for me to write OpenTofu code efficiently and without errors. In addition, I can't use some 3rd party tools that support Terraform, because they don't support OpenTofu.

**With this solution implemented:**  
This problem will not be solved as part of the technical solution in this RFC, but in my opinion we should also address these issues as a part of this RFC. This is an opportunity to standardize the way IDEs and 3rd party tools support OpenTofu. If we want to support `.tofu` files, we should also make sure that IDE plugins and 3rd Party Tools can support the new files.

### Language Server and IDE plugins
We want to give our users the best possible experience when they work with OpenTofu in their IDE. For a comfortable experience our users need features like code completion, syntax highlighting and validation, jump to definition, and others when they work with OpenTofu. If we add support for `.tofu` files, we need to make sure that the IDE plugins that support OpenTofu will also support the new file extensions.

#### VSCode
Today, [VSCode](https://code.visualstudio.com/) supports Terraform using [Terraform Language Server](https://github.com/hashicorp/terraform-ls) and [Terraform Extension](https://marketplace.visualstudio.com/items?itemName=HashiCorp.terraform). The VSCode Extension is using the Terraform Language Server as part of its implementation.
Both projects were already forked by a member of the community - [OpenTofu Language Server](https://github.com/gamunu/opentofu-ls) and [OpenTofu VSCode Extension](https://github.com/gamunu/vscode-opentofu). By looking at the projects, it looks like some work has been done, but support is still lacking for features that were introduced as part of OpenTofu 1.7. We should talk to the maintainer and see how we can contribute and make sure the plugins are up-to-date for supporting the latest OpenTofu features. In addition, we should make sure that the plugins will support the new file extensions that we are going to introduce.

#### JetBrains
Currently, for Terraform, [Jetbrains IDEs](https://www.jetbrains.com/ides/) has the [Terraform and HCL plugin](https://plugins.jetbrains.com/plugin/7808-terraform-and-hcl) that is not using the LSP protocol. The license of the plugin is [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0), based on the Marketplace link mentioned before, although I didn't find a clear license in the [source code repository](https://github.com/JetBrains/intellij-plugins/tree/f99102c5d7f762f3d86a547b69da17bedb0eaf93/terraform).
We should talk to the maintainer. We might be able to contribute to the plugin to support OpenTofu, or more likely, create a new plugin that supports only OpenTofu (and the new file extensions suggested in this RFC).
In case we want to create a new plugin, we might want to consider that on 2023, Jetbrains started [supporting LSP for Plugin Developers](https://blog.jetbrains.com/platform/2023/07/lsp-for-plugin-developers/) and we can use the [OpenTofu Language Server](https://github.com/gamunu/opentofu-ls) for the development of the new plugin. Although, it can only support paid IntelliJ-based IDEs, as explained [here](https://blog.jetbrains.com/platform/2023/07/lsp-for-plugin-developers/).

#### Configurable plugin to support `.tf` and `.tofu` files based on user preference
In both Jetbrains IDEs and VSCode, plugins can be configured by the end user in the Settings menu. Possibly, we can add an optional setting to the plugin that allows it to recognize `.tf` files as OpenTofu files. This option can be very helpful for users switching to OpenTofu from Terraform, and want to get quick support for OpenTofu in the IDE without changing the file extension to `.tofu`. On the other hands, people who want to use both Terraform and OpenTofu in the same project can turn this setting off, and the plugin will not go over the `.tf` files. This setting should be per project and not for the whole IDE (which should be possible based on [Jetbrains documentation](https://www.jetbrains.com/help/idea/configuring-project-and-ide-settings.html) and [VSCode documentation](https://code.visualstudio.com/docs/getstarted/settings)).

#### LSP and Other IDEs
The [OpenTofu Language Server](https://github.com/gamunu/opentofu-ls) can be used with other IDEs that support the [Language Server Protocol](https://en.wikipedia.org/wiki/Language_Server_Protocol).
There are different websites that provide a list of LSP implementations (Like, [this list by Microsoft](https://microsoft.github.io/language-server-protocol/implementors/servers/) and [Langserver.org](https://langserver.org/)) and we should enlist the OpenTofu Language Server, so users will be able to easily find it.

### 3rd Party Tools
Today, some 3rd party tools officially support OpenTofu. Many of them are listed in the [awesome-opentofu repository](https://github.com/virtualroot/awesome-opentofu). Implementing the solution suggested in this RFC might affect 3rd party tools maintainers and they will have to create adjustment support the new `.tofu` extension files. As part of this RFC we should map those tools, understand if they are impacted by this change, and reach out to the maintainers and notify them about this change or contribute.

Furthermore, this change could potentially incentivize other tools to also support OpenTofu. With a clear strategy for managing code divergence in place, maintainers of those tools may gain a better understanding of how to provide support for OpenTofu. Additionally, they can begin by accommodating the new file extensions that we will be introducing in this RFC.

## Technical Approach
The technical change required to implement this RFC is fairly simple:
1. We need to change the `internal/configs` package that loads `.tf` files to also load `.tofu` files (and all other file extensions mentioned in the [Supported Scenarios](#Supported-Scenarios) section). 
2. We need to ignore the `.tf` files when a `.tofu` file exists with the same name.
3. We need to add DEBUG logs about which files were loaded and which were ignored, so users can understand what is happening in their project if issues occurs.
4.Support the new files as part of the `fmt` [command](https://opentofu.org/docs/cli/commands/fmt/) - extend the relevant code in [here](https://github.com/opentofu/opentofu/blob/7a713ccd833c10a315e2397da80923215da45332/internal/command/fmt.go#L32).

We have a working [proof of concept](https://github.com/opentofu/opentofu/issues/1328) and the implementation will be very similar to it. So the change in the code is very minimal and isolated to specific locations in the codebase.

## Open Questions
2. if someone starts a new project (tofu only), do we recommend going with `.tofu` files?

## Cons
A major drawback of this solution is that it leads to code duplication for users who need to stay compatible with both OpenTofu and Terraform (such as module maintainers). If These users want to use an OpenTofu-specific feature, they must create and manage two copies of the same file, one with the `.tf` extension and one with the `.tofu` extension. Potentially, in the future, they might have multiple duplicated files in their project. It can be confusing and error-prone, and might cause configuration drifts between the Terraform and OpenTofu configuration over time.

## Future Considerations
1. Maybe in the far future, when migration from Terraform to OpenTofu might impose a challenge on our users, we can add a command/tool that automatically converts `.tf` files to `.tofu` files in the project. This way, users will have to put a very small effort in migrating to OpenTofu. A good example for a similar tool is the [Java to Kotlin Converter](https://kotlinlang.org/docs/mixing-java-kotlin-intellij.html#converting-an-existing-java-file-to-kotlin-with-j2k).
2. I don't think we need it now, maybe when we'll have more OpenTofu-specific feature, but we can create Best Practices around how to work with `.tofu` files.

## Potential Alternatives
1. One proposition was that the `.tofu` file will not override the `.tf` file completely, but will instead override specific blocks. This way, we can avoid code duplication. But this solution is not sufficient for the future, when Terraform might introduce features that OpenTofu cannot parse.
2. We thought about using a different file extension, like `.otf`, but we decided to use `.tofu` because the `.otf` extension is already used to represent [OpenType](https://en.wikipedia.org/wiki/OpenType) font files.

## General Break Down to Tasks
In my opinion, this is an opportunity to standardize the way IDEs and 3rd party tools support OpenTofu. So, we should not only pursue technical tasks, but also include peripheral tasks to establish a better experience for our users. Here are the tasks that should be done as part of this RFC:
1. Approve this RFC by the core-team and the community.
2. Implement the technical change. Including tests and documentation.
3. IDEs:
   * Approach the maintainers of the OpenTofu VSCode Extension and OpenTofu Language Server, notify about this change, and see if we can help.
   * Decide on a solution for the JetBrains IDEs plugin.
4. 3rd party tools:
   * Notify all relevant impacted tools maintainers about this change.
   * Consider if we want to approach maintainers of tools that don't officially support OpenTofu and see if they are willing to support OpenTofu.

## Issues waiting for this RFC
* [Allow variables to be marked as deprecated to communicate variable removal to module consumers](https://github.com/opentofu/opentofu/issues/1005).
* [Needs OpenTofu image version compatible with terraform version 1.7.x](https://github.com/opentofu/opentofu/issues/1708)