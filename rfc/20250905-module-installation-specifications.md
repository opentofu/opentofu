# Module Installation Specifications

Primary Issue: https://github.com/opentofu/opentofu/issues/1086 
Related Issues:
* https://github.com/opentofu/opentofu/issues/586
* https://github.com/opentofu/opentofu/issues/1199
* https://github.com/opentofu/opentofu/issues/2719

Many users in both the OpenTofu ecosystem and it's predecessor have requested that module installation packages are de-duplicated (symlink) to reduce space on disk and install time.  They have also requested that module contents be "locked" to prevent local modification by other programs on the same system and ensure consistency. Both of these are reasonable requests, but bump into complexity around historical edge cases.

Modules in OpenTofu and it's predecessor have access to the directory in which a module package is installed via `${paths.module}`. In most cases, this allows modules to package data such as configuration and template files used by the resources in the module. However, given that this exposes the filesystem paths directly, modules have the ability to modify their contents.  In practice this is not a common occurrence and not recommended, but it does exist and is supported for historical reasons.

It is therefore not possible to implement the features requested above for any module which is self modifying.

The easiest solution is to simply forbid modules from self modifying and introduce new concepts to fill that gap.  Some discussion has taken place in https://github.com/opentofu/opentofu/pull/2049 to introduce those concepts. Unfortunately, simply forbidding self modifying modules violates our 1.0 Compatibility Promise.

Any solution proposed must allow modules to opt-in or opt-out of declaring itself read-only.

## Caveats

### Current Behavior

All remote modules are downloaded and copied into their respective module path locations in `.terraform/modules`. Explicitly, modules with identical remote sources are downloaded once and then copied multiple times to provide a unique directory for every usage of that module.

All local modules (specified via a relative path, "./my-module") are symlinked directly into .terraform/modules.

This means that if you move a module from local to remote or vice-versa, the behavior around multiple usages of a self modifying module can change in unexpected ways.

Additionally, any usage of for_each or count *anywhere* in the module tree complicates reasoning about self modifying modules significantly.

#### Example:
```hcl
# main.tofu
module "top" {
  count = 5
  source = "git://git.source/module"
}

# git.source/module/mod.tofu
module "inner" {
    source = "git://git.source/self-modifying"
    # Module Author Note: This "inner" module self modifies and can not be called used with for_each or count. Duplicate this block if you need another instance.
}
```
In this example, the author of git.source/module/ has introduces a self modifying module and has done so with an internal comment about it's safety. However, there is nothing to prevent the caller of that module from embedding it within a for_each or count itself.

`module.top[0..4].inner` will all have an identical values for `${module.path}` and will likely break in subtle and confusing ways.

### Plan Files

Plan files include the entire source tree. Any solution proposed should take this into account.

For example, a global module installation cache directory would require additional work in building the planfile as the module sources could live outside of `.terraform/modules`

### Inheritance

In many scenarios, users of modules do not own those modules and are pulling them from a different organization which owns them. Therefore, it could be argued that it is necessary to allow modules to mark both themselves *and modules they rely on* as either read-only or self-modifying.

A common example is organizations like https://github.com/terraform-aws-modules/ https://github.com/GoogleCloudPlatform/?language=hcl. Ideally, we would be able to convince them to add support for whatever solution we propose here. In practice, we can not count on this happening and even if it does, it will not impact historical releases.

## Proposed Solution

We propose to build on top of the [Module Package Metadata RFC](../rfc/20250905-module-package-metadata.md) by introducing fields that allow module authors to specify if a module package, it's modules, and their dependencies are safe to treat as read-only or if they are unsafe and self modify.

We also propose that with the mere existence of a module package metadata being defined, OpenTofu should treat the module package, it's modules, and their dependencies as read-only by default.  This would minimize the disruption for the majority of users, while making it fairly easy to exclude modules that self modify.

### User Documentation

There are two different scenarios to be considered from an OpenTofu user's perspective:

#### Module Author

When describing a module package (which contains modules), an author may create a module-package.meta.hcl file in the root of the package.

Without a file present, OpenTofu does not make any assumptions about the contents of this package.

An empty file implies that all modules discovered in this package and all the modules referenced in this package are read-only as creating this file "opts-in" to tofu's default behavior.

Within the file, a module author can specify the status of each module within the package: Read-Only or Self-Modifying. It can also specify that the dependencies of each package are Read-Only or Self-Modifying.

Example with multiple modules:
```hcl
# module-package.meta.hcl
module "mymod" {
    path = "./modules/mymod"
    read-only = {
        self = false
        dependencies = true
    }
    < other metadata fields >
}

module "othermod" {
    path = "./modules/othermod"
    # read-only defaults to true
    < other metadata fields >
}
```

Example with single module in the root of the package
```
# module-package.meta.hcl
module "main" {
    path = "."
    read-only = {
        self = false
        dependencies = false
    }
    < other metadata fields >
}
```

#### Project Author (Root Module)

In practice, there is very little structural difference between a project/root module and a module package. Therefore, a module-package.meta.hcl file present in the root module will have the same functionality as described for module authors.  It will allow users to quickly and easily opt into this functionality.

Example:
```hcl
# module-package.meta.hcl
module "local-helper" {
    path = "./helpers/helper"
    read-only = { self = false }
}

# main.tofu
module "mymod" {
    source = "./helpers/helper"
    attr = "value"
}

module "mymod2" {
    source = "./helpers/helper"
    attr = "value2"
}
```

In this example, both module.mymod and module.mymod2 exist within their own distinct `module.path` directories.

### Technical Approach

When installing a module package, a [configs.ModuleRequest](https://github.com/opentofu/opentofu/blob/a88a1f004ebf45df643bad7b5dff72220477c2dc/internal/configs/config_build.go#L262) is created that contains all the information needed for the module installer.  This includes the source (module package and module) as well as the location of the module within the configuration tree.  An additional field can be added to the request as an override for "read-only" if a module package meta file is not found.

The [ModuleInstaller](https://github.com/opentofu/opentofu/blob/a88a1f004ebf45df643bad7b5dff72220477c2dc/internal/initwd/module_install.go#L162) will then inspect the package it's installing and the request to see what mode it should be operating in. This is based on the work that is described in the Module Package Metadata RFC where the module package metadata is attached to the configs.Module.

The `.terraform/modules` directory will be enhanced to include installed packages that can be either copied from or symlinked depending on the given settings. Alternatively a new directory could (should?) be created to differentiate installed module packages and their usage in .terraform/modules.

A prototype using an alternate approach to module metadata shows how some of these changes could work in practice: https://github.com/opentofu/opentofu/compare/d9193a5964a359bb72e245a43e3201fe053623a8...018c286dafccb04a7548ac4271350c935f289d6f

### Open Questions

* Should the `dependencies` field be more complex than a boolean true/false?  In practice, we are talking about an edge case.
* Should we allow modules to refuse to be used in a for_each or count context if they self modify?

### Future Considerations

* How to implement TF_MODULE_CACHE_DIR, which this should make easier.
* How to lock modules, now that we should know which modules could be locked (limited to read-only)

## Potential Alternatives

* Consider the small number of self modifying modules small enough to not warrant this complexity and decide to break them
* Add a field to each module call to allow it to declare that it is not read-only.  Depending on the default, it would either need to be added to every call within all projects or only to projects that have self modifying modules that would be broken by this default.
