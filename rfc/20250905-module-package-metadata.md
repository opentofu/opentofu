# Module Package Metadata 

This RFC introduces the concept of Module Package Metadata to allow module authors to clearly define information about the modules defined within the package.

This is primarilly motivated by the [Module Installation Specifications RFC](../rfc/20250905-module-installation-specifications.md) and it's primary issue https://github.com/opentofu/opentofu/issues/1086 

## Proposed Solution

To define the initial format for a module-package.meta.hcl file in the root of installed module packages. This would be useful to specify that a module is designed for use in OpenTofu and potentially opt-into new functionality.

It could also help improve the discoverability of modules and their documentation.

### User Documentation

```hcl
# module-package.meta.hcl

package {
    description = "My Awesome Modules!"
}

module "mymod" {
    path = "./modules/mymod"
    description = "My Awesome Module"
}

module "othermod" {
    path = "./modules/othermod"
    description = "My Awesome Other Module"
}
```

The first consumer of this will likely be the Module Installation Specifications linked above and would add a read-only field with defaults.

### Technical Approach

This file will be a standard HCL file and use common conventions within the `internal/configs` package.

The [ModuleInstaller](https://github.com/opentofu/opentofu/blob/a88a1f004ebf45df643bad7b5dff72220477c2dc/internal/initwd/module_install.go#L162) will inspect packages as they are installed and attatch the corresponding metadata to the a new field in [configs.Configs](https://github.com/opentofu/opentofu/blob/a88a1f004ebf45df643bad7b5dff72220477c2dc/internal/configs/config.go#L33).

### Open Questions

* Should we use "path" instead of "source" to make it clear it can be a relative directory only?
* Should we make it easy to expose reading this file as a library for things like the [registry UI](https://search.opentofu.org)?

### Future Considerations

* Should any of the fields in here be able to specify required terraform/tofu version?
* How do we version this file?

## Potential Alternatives

* Instead of having a seperate package metadata file, each module could have files that define it's metadata.  Either in the standard `.tofu` files or a new file like `module-meta.hcl`.
