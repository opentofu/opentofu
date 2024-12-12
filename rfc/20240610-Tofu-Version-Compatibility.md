# Tofu Version Compatibility

Issue: https://github.com/opentofu/opentofu/issues/1708 <!-- Ideally, this issue will have the "needs-rfc" label added by the Core Team during triage -->

As OpenTofu and Terraform diverge, the concept of "software version" for compatibility becomes murky.

The "software version" is used in a few locations:
* Feature compatibility with config files in the root project and modules
* Provider compatibility checks (deprecated but still used)
* State compatibility ("terraform_version" field)

At the time of writing this RFC, OpenTofu 1.7.x has very similar features to Terraform 1.8.x (though not identical). As the "software version" requirements are updated in the previous examples, the exact requirements become murky. A module may require features from "Terraform 1.7.4", but would be perfectly happy with "OpenTofu 1.6.5" (as an example).

> [!NOTE]
> As we are unable to inspect the Hashicorp Terraform Repository in depth, all version compatibility must be based on Release Notes / Changelog Entries as well as user reports.

## Proposed Solution

As we [introduce support](20240529-OpenTofu-Specific-Code-Override.md) for `.tofu` files, we open up the ability to add new language features without worrying as much about compatibility in shared modules. If we can differentiate between "OpenTofu Version" and "Terraform Version", the above scenarios become much easier.

As the [provider compatibility check is deprecated](https://github.com/hashicorp/terraform-plugin-framework/blob/8a09cef0bb892e2d90c89cad551ce12a959f1a2d/provider/configure.go#L28-L32) and the state's "terraform_version" field is not currently used in OpenTofu, we will not discuss them in this RFC.

### User Documentation

Currently the "software version" (both Terraform and Tofu) is defined in the config as `terraform -> required_version`. This represents a valid range of versions that the configuration is compatible with. When OpenTofu is compiled, the "software version" is baked into the binary and used for all version checks (as well as`tofu -version`).

Example:
```hcl
terraform {
    required_version = "1.7.4"
}
```
This configuration currently requires either Terraform 1.7.4 or OpenTofu 1.7.4 which are wildly different things.

Let's look at a project or module which plans on supporting both tools (for the time being). They will start to introduce `.tofu` files as they start utilizing additional functionality in OpenTofu. How would they go about setting version requirements?

```hcl
# version.tf
terraform {
    required_version = "1.7.4"
}
```
```hcl
# version.tofu
tofu {
    required_version = "1.6.5"
}
```

When loaded by:
* Terraform, the `version.tofu` file would be ignored and the "Terraform Required Version" would be 1.7.4.
* OpenTofu, the `version.tofu` file would be loaded instead of `version.tf` and the "OpenTofu Required Version" would be 1.6.5.


What happens if only the `version.tf` file exists in this module with no tofu overwrite? This is a common case for modules who have not yet considered explicit OpenTofu migration.

As a user, I would like to know if any incompatible terraform_version has been specified as part of my project's configuration and modules. I would also assume any version requirement under 1.6 would be identical between Terraform and OpenTofu.

```hcl
terraform {
    required_version = 1.7.4
}
```
```shell
$ TF_LOG=debug tofu init
Initializing the backend...
Initializing modules...
Warning: Configuration requests terraform version >= 1.6!
  Please update the configuration to specify the required OpenTofu Version.  More information is available in the debug log.
Debug: terraform required_version = 1.7.4 in main.tf:421
```
This could also include some information on how to remedy this situation, perhaps linking to the docs.

If an incompatible (>1.5.5) version is detected, OpenTofu will ignore the "Terraform Required Version".

### Technical Approach

The "required_version" field is accessed via `sniffCoreVersionRequirements(body)` in internal/configs/parser_config.go and stored in `configs.File.CoreVersionConstraints`. These are then merged into `configs.Module.CoreVersionConstraints` and are checked in `configs.Module.CheckCoreVersionRequirements()`. This check function is called in a variety of locations moving up the stack, but is as high as we need to concern ourselves with for this RFC. We will need to make sure the warning won't be emitted multiple times due to the spaghettified command package.

We have two very similar paths here:
* Split CoreVersionConstraints into two fields: `TofuVersionConstraints` and `TerraformVersionConstraints`
* Add a flag into each VersionConstraint that understands if it is Tofu or Terraform specific.

Either path is easy to both implement and thoroughly test.

The `configs.Module.CheckCoreVersionRequirements()` function is where we will need to emit the warnings and corresponding debug log entries.

As discussed in the `.tofu` RFC, we will want to quickly work on support for this feature.

### Open Questions

Should there be an option (config, CLI, or ENV) to:
* Disable the version checking altogether?
* Don't check terraform required_versions
* Treat both Tofu and Terraform version as identical instead of using our equivalent "terraform version" guess?
* ~~Do we show the guessed terraform version in `tofu -version`, maybe with a `-verbose` flag?~~ This goes against the Technical Steering Committee's strong recommendation against a version compatibility table.

### Future Considerations

If we run into providers expecting a particular terraform version (even though this is deprecated), we may want to use the "Terraform Version Equivalent" in that API call.

## Potential Alternatives

Don't implement this and assume that all module and project authors will adopt the .tofu extension and keep the respective required_versions up to date.

Implement an option to disable required_version checking.

~~Try to guess the equivalent OpenTofu version for a specified Terraform version.~~ This goes against the Technical Steering Committee's strong recommendation against a version compatibility table.

