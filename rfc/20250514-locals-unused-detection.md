# Locals unused detection

Issue: [2583](https://github.com/opentofu/opentofu/issues/2583)

## Summary

This proposal introduces warnings during validation when a local value (`local`) is declared in a module but never referenced. The aim is to improve readability, maintainability, and code quality by identifying unused definitions.

Currently, OpenTofu does not emit any warning when a local value is declared but unused. This can lead to configuration bloat, making codebases harder to read and maintain. By warning on unused values, developers are encouraged to clean up stale or redundant code, enhancing the overall quality of OpenTofu configurations.

## Proposed Solution

- **Local Values**:
  If a value declared inside a `locals` block is not referenced elsewhere in the module, a warning is emitted during validation.

### User Documentation

```bash
tofu validate

│ Warning: The local "this_is_my_local" is unused.
│
│   on local.tf line 9:
│    9:     this_is_my_local="local_value"

│ Warning: The local "this_is_my_local" is unused.
│
│   on mod1/mod2/local.tf line 9:
│    9:     this_is_my_local="local_value"

Success! The configuration is valid.
```

### Technical Approach

The validation process must perform static analysis of the configuration to determine whether each declared local value is actually used. This will require traversing the module's dependency graph and checking references.

Regarding [20241118-module-vars-and-outputs-deprecation RFC](https://github.com/opentofu/opentofu/blob/main/rfc/20241118-module-vars-and-outputs-deprecation.md#silencing-deprecation-warnings-for-dependencies) we can use a dedicated flag "deprecation" to activate the detection feature to avoid weird interactions for users that don't really care about this.

The "deprecation" flag can be define the warning type that we want to have (module:all, module:local, module:none) according with the RFC.

### Future Considerations

- **Unused Provider Configurations**:
  These are out of scope for this RFC. Providers are propagated automatically and require a separate design consideration.[2537](https://github.com/opentofu/opentofu/issues/2537)

## Potential Alternatives

- **Emit errors instead of warnings**:
  This was deemed too aggressive and could introduce friction for existing users.

- **Custom scripts**:
  Currently we must to create custom script to detect unused locals. I think it's not a good solution to use custom script in addition of OpenTofu.
