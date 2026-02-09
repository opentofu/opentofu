**Release Support Policy**
========================

## **Release Cycle**

We do not currently have a fixed release cycle.

## **Support Duration**

The support period will be documented in [CHANGELOG.md](CHANGELOG.md) for the corresponding version by OpenTofu Maintainers.

The chosen duration is informed by constraints such as:
- The support duration of the corresponding Go major release
- Support duration of external libraries that OpenTofu depends upon

## **Documentation**

For each minor version (e.g., 1.6, 1.7) and the development version (corresponding to the main branch), detailed release notes will be provided.

## **Compatibility**

To check the compatibility of OpenTofu with Terraform, refer to the ([Migration guide](https://opentofu.org/docs/intro/migration/))

## **Nightly Builds**

Nightly builds are currently being trialled experimentally, each build will be removed after 30 days and are not intended for usage in production environments ever.

A list of available nightly builds can be found at `https://nightlies.opentofu.org/nightlies/latest.json`.
