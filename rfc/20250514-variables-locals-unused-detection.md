# Variables and Locals unused detection

Issue: [2583](https://github.com/opentofu/opentofu/issues/2583)

## Summary

This proposal introduces warnings during validation when an input variable (`variable`) or a local value (`local`) is declared in a module but never referenced. The aim is to improve readability, maintainability, and code quality by identifying unused definitions.

Currently, OpenTofu does not emit any warning when an input variable or local value is declared but unused. This can lead to configuration bloat, making codebases harder to read and maintain. By warning on unused values, developers are encouraged to clean up stale or redundant code, enhancing the overall quality of OpenTofu configurations.

## Proposed Solution

* **Input Variables**:
  During validation, if an input variable is declared in a module but never referenced, a warning is emitted—except when the variable is marked as `deprecated`. In that case, the warning is only emitted if a value is assigned to the variable by a caller.

* **Local Values**:
  If a value declared inside a `locals` block is not referenced elsewhere in the module, a warning is emitted during validation.

### User Documentation

```bash
tofu validate
[Warning]  variable xxx defined but not used inside code
[Warning]  variable yyy defined but not used inside code
[Warning]  local zzz defined but not used inside code
Success! The configuration is valid.
```

### Technical Approach

The validation process must perform static analysis of the configuration to determine whether each declared input variable or local value is actually used. This will require traversing the module's dependency graph and checking references.

### Future Considerations

* **Unused Provider Configurations**:
  These are out of scope for this RFC. Providers are propagated automatically and require a separate design consideration.[2537](https://github.com/opentofu/opentofu/issues/2537)

## Potential Alternatives

* **Emit errors instead of warnings**:
  This was deemed too aggressive and could introduce friction for existing users.

* **Custom scripts**:
  Currently we must to create custom script to detect unused variables. I think it's not a good solution to use custom script in addition of OpenTofu.
