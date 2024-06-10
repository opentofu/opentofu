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

As we introduce support for `.tofu` files, we open up the ability to add new language features without worrying as much about compatibility in shared modules. If we can differentiate between "OpenTofu Version" and "Terraform Version", the above scenarios become much easier.

We must also consider "partial knowledge" of version information. Ideally the configuration would clearly define which terraform version and tofu version are required. If that is not the case, we will have to provide a "best guess" to fill in the blanks.

As the [provider compatibility check is deprecated](https://github.com/hashicorp/terraform-plugin-framework/blob/8a09cef0bb892e2d90c89cad551ce12a959f1a2d/provider/configure.go#L28-L32) and the state's "terraform_version" field is not currently used in OpenTofu, we will not discuss them in this RFC.

### User Documentation

Currently the "software version" (both Terraform and Tofu) is defined in the config as `terraform -> required_version`. This represents a valid range of versions that the configuration is compatible with. When OpenTofu is compiled, the "software version" is baked into the binary and used for all version checks (as well as`tofu -version`).

Example:
```hcl
terraform {
    required_version = 1.7.4
}
```
This configuration currently requires *both* Terraform 1.7.4 and OpenTofu 1.7.4 which are wildly different things.

Let's look at a project or module which plans on supporting both tools (for the time being). They will start to introduce `.tofu` files as they start utilizing additional functionality in OpenTofu. How would they go about setting version requirements?

```hcl
# version.tf
terraform {
    required_version = 1.7.4
}
```
```hcl
# version.tofu
tofu {
    required_version = 1.6.5
}
```

When loaded by:
* Terraform, the `version.tofu` file would be ignored and the "Terraform Required Version" would be 1.7.4.
* OpenTofu, the `version.tofu` file would be loaded instead of `version.tf` and the "OpenTofu Required Version" would be 1.6.5.


What happens if only the `version.tf` file exists in this module with no tofu overwrite? This is a common case for modules who have not yet considered explicit OpenTofu migration.

As a user, I would expect that the "Terraform Required Version" would have an equivalent "OpenTofu Required Version". This is not a 1-1 mapping, but is a reasonable stop-gap solution.

As this is a not a 1-1 version mapping and a "best effort" solution, I would expect to see a warning:

```shell
$ tofu init
...
Warning: Using v1.7.x in 'terraform -> required_version' as equivalent to current tofu version 1.6.5!
At file/line: ...
...
```

This could also include some information on how to remedy this situation, perhaps linking to the docs.

### Technical Approach

OpenTofu internally should know both it's own "software version" and what "terraform version" it is similar to.

When we update the VERSION file in OpenTofu, we should also include the "terraform version" in the same file, or a similar file. OpenTofu's version package should then be able to supply both versions upon request.

The "required_version" field is accessed via `sniffCoreVersionRequirements(body)` in internal/configs/parser_config.go and stored in `configs.File.CoreVersionConstraints`. These are then merged into `configs.Module.CoreVersionConstraints` and are checked in `configs.Module.CheckCoreVersionRequirements()`. This check function is called in a variety of locations moving up the stack, but is as high as we need to concern ourselves with for this RFC.

We have two very similar paths here:
* Split CoreVersionConstraints into two fields: `TofuVersionConstraints` and `TerraformVersionConstraints`
* Add a flag into each VersionConstraint that understands if it is Tofu or Terraform specific.

Either path is easy to both implement and thoroughly test.

#### Version Compatibility

As features don't change much between patch versions of Tofu and Terraform, we can use "Major.Minor.999999" as the "Terraform Version". This means that we are only tracking similarities between Major/Minor releases.

This can be shown to the user as "Major.Minor.x" and solves the problem of new patch releases in Terraform after a compatible OpenTofu release has already gone out.

### Open Questions

Should there be an option (config, CLI, or ENV) to:
* Disable the version checking altogether?
* Treat both Tofu and Terraform version as identical instead of using our equivalent "terraform version" guess?
* Do we show the guessed terraform version in `tofu -version`, maybe with a `-verbose` flag?

### Future Considerations

If we run into providers expecting a particular terraform version (even though this is deprecated), we may want to use the "Terraform Version Equivalent" in that API call.

## Potential Alternatives

Don't implement this and assume that all module and project authors will adopt the .tofu extension and keep the respective required_versions up to date.

Implement an option to disable required_version checking.

