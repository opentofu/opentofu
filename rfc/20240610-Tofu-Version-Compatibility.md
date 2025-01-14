# Tofu Version Compatibility

Issue: https://github.com/opentofu/opentofu/issues/1708

As OpenTofu and Terraform diverge, the concept of "software version" for compatibility becomes murky.

The "software version" is used in a few locations:
* Feature compatibility with config files in the root project and modules
* Provider compatibility checks (deprecated but still used)
* State compatibility ("terraform_version" field)

As a concrete example, OpenTofu 1.7.x has very similar features to Terraform 1.8.x (though not identical). As the "software version" requirements are updated in the previous examples, the exact requirements become murky. A module may require features from "Terraform 1.7.4", but might be perfectly happy with "OpenTofu 1.6.5".

> [!NOTE]
> As we are unable to inspect the HashiCorp Terraform Repository in depth, all version compatibility must be based on Release Notes / Changelog Entries as well as user reports.

## Proposed Solution

We aim to address the scenario in which a module may be written for a given Terraform version and has a specific version requirement, while a OpenTofu user may wish to use the module and bypass the "Terraform" version requirement.

Note: Since the [provider compatibility check is deprecated](https://github.com/hashicorp/terraform-plugin-framework/blob/8a09cef0bb892e2d90c89cad551ce12a959f1a2d/provider/configure.go#L28-L32) and the state's "terraform_version" field is not currently used in OpenTofu, we will consider them out of scope for this RFC.

First, some definitions:
* A "Terraform Version" is represented by a `terraform { required_version = "requirement" }` block within a `.tf` file.
* A "Tofu Version" is represented by a `terraform { required_version = "requirement" }` block within a `.tofu` file.

Currently in OpenTofu, the "Terraform Version" and "Tofu Version" are treated identically. As mentioned above, the version requirement maps to different feature sets in the two different projects.

The only way around this problem as of today is to create a .tofu file for each `terraform {}` block in a project and module.  This is the "ideal" scenario as modules can explicitly choose which feature sets they want when used within the two projects.

An example of the version split is seen [here](https://github.com/GoogleCloudPlatform/cloud-foundation-fabric/pull/2771) and "resolves" the original issue that spawned this RFC and discussion.  Unfortunately, it took half a year for the module that the user relies upon to update.

This situation leaves users in the lurch, with the only option available being to fork and modify every module with an incompatible "Terraform Version" constraint.

The solution proposed in this RFC is to add the ability for a project author to disable "Terraform Version" requirements all together and rely solely on "Tofu Version".  As this is a potentially dangerous option and requires users to understand the features in both, it should be an explicit opt-in feature.

To help with clarity, an alternative to the `terraform{}` block is proposed, the `tofu{}` block. Functionally speaking, it is identical to the `terraform{}` block and will in most cases be treated as such.  With this in place, we can amend our definitions above with: A "Tofu Version" is also represented by a `tofu { required_version = "requirement" }` block within a `.tf` or `.tofu` file.  We also introduce the term `project{}` block to refer to either interchangeably.


From the perspective of a module, root or child, there are two different ways in which "Terraform Version" should be ignored:
1) Disabled for all child modules of the current module.  This represents someone who has lots of child modules that have not yet added "Tofu Versions" and they are comfortable with the risk of bypassing the "Terraform Version" in all of them (and their children).
2) Disabled for a specific child module of the current module.  This represents someone who is pulling a small number of child modules that have not yet added "Tofu Versions" and they would like to limit the blast radius of ignoring the "Terraform Version" check in this limited area of the current module.

For the first scenario, a flag called "bypass_terraform_version_requirement" is added to the `product{}` block.  If set to true, all child modules of the current module will only error on "Tofu Versions" and instead produce warnings for "Terraform Versions".

For the second scenario, a flag called "bypass_terraform_version_requirement" is added to the specific `module{}` block.  If set to true, that specific module and all of it's children will only error on "Tofu Versions" and instead produce warnings for "Terraform Versions".


### User Documentation

When adopting OpenTofu in a project or module, considerable care should be taken when inspecting project (terraform/tofu) version requirements.  Even though these projects have a shared ancestry and may follow similar paths, the specific version numbers can mean very different things.  A version represents an arbitrary feature set, which between the two projects will likely diverge further and further over time.

The ideal situation is that every child module that is used in a root module declares both a Terraform and OpenTofu version based on the specific feature sets it needs.  As of today this is not yet practical to rely upon and can make migrating to OpenTofu more of a headache than is ideal.

To work around this limitation, we introduce the "bypass_terraform_version_requirement" field to both the `teraform{}` and `module{}` blocks.  Additionally, for clarity we introduce the `tofu{}` block as a synonym of the `terraform{}` block and refer to them both as the `project{}` block.

Some definitions before we get into behavior:
* "Terraform Version" refers to `terraform{ required_version = "requirement" }` present in a `.tf` file
* "Tofu Version" refers to `terraform{ required_version = "requirement" }` present in a `.tofu` file OR `tofu{ required_version = "requirement" }` in either a `.tf` or `.tofu` file


#### Project scoped behavior:

By setting `bypass_terraform_version_requirement = true` in the `project{}` block, all "Terraform Version" requirements in child modules of the current module will be treated as warnings.

For example:
```hcl
# main.tofu

tofu {
    required_version = ">=1.5.5"
    bypass_terraform_version_requirement = true
}

module "childA" {
    source = "something/i/dont/want/to/fork"
    input = "A"
}

# childB...Y

module "childZ" {
    source = "something/i/dont/want/to/fork"
    input = "Z"
}
```

```hcl
# something/i/dont/want/to/fork/versions.tf
terraform {
    required_version = ">=1.42.10"
}
```

Running `tofu init` will produce:
```

Initializing the backend...
Initializing modules...

Warning: Ignored terraform version requirement
  on: something/i/dont/want/to/fork/versions.tf in module "childA"
  <suggestion to fix or fork that module>

....


Warning: Ignored terraform version requirement
  on: something/i/dont/want/to/fork/versions.tf in module "childZ"
  <suggestion to fix or fork that module>

```


#### Product scoped behavior:

By setting `bypass_terraform_version_requirement = true` in a `module{}` block, all "Terraform Version" requirements in the requested module and it's children will treated as warnings.

For example:
```hcl
# main.tofu

tofu {
    required_version = ">=1.5.5"
}

module "child" {
    source = "something/i/dont/want/to/fork"
    bypass_terraform_version_requirement = true
}
```

```hcl
# something/i/dont/want/to/fork/versions.tf
terraform {
    required_version = ">=1.42.10"
}

module "inner_child" {
    source = "somethingelse/i/dont/want/to/fork"
    bypass_terraform_version_requirement = true
}
```

Running `tofu init` will produce:
```

Initializing the backend...
Initializing modules...

Warning: Ignored terraform version requirement
  on: something/i/dont/want/to/fork/versions.tf in module "child"
  <suggestion to fix or fork that module>


Warning: Ignored terraform version requirement
  on: somethingelse/i/dont/want/to/fork/versions.tf in module "child.inner_child"
  <suggestion to fix or fork that module>

```

### Technical Approach

The "required_version" field is accessed via `sniffCoreVersionRequirements(body)` in internal/configs/parser_config.go and stored in `configs.File.CoreVersionConstraints`. These are then merged into `configs.Module.CoreVersionConstraints` and are checked in `configs.Module.CheckCoreVersionRequirements()`. This check function is called in a variety of locations moving up the stack, but is as high as we need to concern ourselves with for this RFC. We will need to make sure the warning won't be emitted multiple times due to the spaghettified command package.

We have two very similar paths here:
* Split CoreVersionConstraints into two fields: `TofuVersionConstraints` and `TerraformVersionConstraints`
* Add a flag into each VersionConstraint that understands if it is Tofu or Terraform specific.

Either path is easy to both implement and thoroughly test.

The `configs.Module.CheckCoreVersionRequirements()` function is where we will need to emit the warning diagnostics.

Additionally, introducing "bypass_terraform_version_requirement" to the `module{}` block is introducing a new "reserved" field that must not be used by module authors. Given it's unique naming and lack of search results on Github, I believe that it is safe to introduce.

### Open Questions

#### How should existing "Terraform Versions" be treated?

This RFC proposes that the handling of unsatisfied "Terraform Version" requirements is enabled unless otherwise specified.  This keeps consistency with existing functionality and proposes a workaround for those blocked during the migration process.

@apparentlymart brought up the idea that as "Terraform Version" greater than 1.5.x is effectively meaningless, it could be ignored all together (outside of the root module).

This idea brings about the question of what is the true utility of "Project Version" and how is it used and relied upon today.  In general, it is bumped when new language features or fixes are added.


As an example, let's consider what happens when a new feature is introduced.  The `required_version` is bumped to the latest version of the project.
```hcl
terraform {
    # Updated for extended moved support
    required_version = ">=1.9.0"
}
moved {
    # Moved now supports migrating between resources (if supported by the provider)
    from = some_provider.resource
    to = some_provider.other_resource
}
```
Note that this does not introduce any new configuration options and instead makes the existing functionality more flexible.  When run with Terraform 1.9.0, this will behave as expected as the feature required was introduced in that version".  When run in previous versions of Terraform, it would have produced an error message detailing that this operation is not supported.  OpenTofu 1.9.0 produces the same error message.  Eventually this will be supported in OpenTofu and a corresponding .tofu file could be added to ensure compatibility.

What was the worst case scenario if the `required_version` was ignored by OpenTofu? A user would see a error message detailing why the operation is not valid.  A charitable user would look at the release notes, issue tracker, or https://cani.tf for feature support.  A uncharitable user would complain about OpenTofu publicly and advocate against it, grumbling about footguns.  That could potentially be softened by adding a warning that the required version check is undefined for "Terraform Version" and is being skipped.


A more dangerous example is for complex bugs, such as https://github.com/opentofu/opentofu/issues/1616. In that scenario, handling of sensitive attributes was broken in both Terraform and OpenTofu prior to the fork.  Terraform seems to fixed the issue in the 1.8 series, and the issue was reported from someone migrating from that version.
```hcl
terraform {
    # 1.8.2 and beyond correctly handle sensitive attributes
    required_version = ">=1.8.2"
}
resource "random_password" "password1" {
  length           = 12
  special          = false
}
```

This bug could not only impact the planning of an apply, but could expose incorrect sensitive attribute detection to third party tools.  With the complexity of this project, bugs like these are bound to exist and require fixes over time. "Product Version" is one of the few ways for module authors to require fixes.

In the case that OpenTofu silently ignored the required_version, a user could be using third party tooling to expose secrets.  If the module explicitly was asking for a "Terraform Version" that was unmet, and the user of the module did not know that the safety check was bypassed, they could both be soured against OpenTofu.  If a warning were added, clarifying that OpenTofu does not understand "Terraform Versions" past 1.5.x and the requirements should be manually reviewed / module updated, a user would at least have a chance at detecting that something was wrong and deciding to ignore it or not. 


To make the decision between making "Terraform Version" produce only a warning and the more manual approach of a flag, we must consider those two examples and their outcomes carefully.

### Future Considerations

If we run into providers expecting a particular terraform version (even though this is deprecated), we may want to use this flag to work around it?

## Potential Alternatives

Don't implement this and assume that all module and project authors will adopt the .tofu extension and keep the respective required_versions up to date.

~~Try to guess the equivalent OpenTofu version for a specified Terraform version.~~ This goes against the Technical Steering Committee's strong recommendation against a version compatibility table and is more confusing for end users.

