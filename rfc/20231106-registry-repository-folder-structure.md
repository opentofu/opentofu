# Registry: Repository Folder Structure

Issue: https://github.com/opentofu/opentofu/issues/827

> [!NOTE]  
> This RFC was originally written by @RLRabinowitz and was ported from the old RFC process. It should not be used as a reference for current RFC best practices.

This proposal is part of the [Homebrew-like Registry Design](https://github.com/opentofu/opentofu/issues/741). It's a prerequisite for all other parts of the design, as most of them will rely on the folder structure of the main repository

The folder structure of the registry repository should allow for easy addition / change / removal of provider and module metadata, so it could later be transformed and hosted for the CLI to access. The design would take into account the sheer amount of files that could be part of this repository

## Proposed Solution

### Folder Structure

The folder structure would include a `modules` and `providers` folders. Each should be sharded by the first character of the namespace of the provider/module, to help with GitHub UI performance when scrolling through providers or viewing commits. This sharding is based on how `homebrew-core` has sharded its Formulae, More info in https://github.com/opentofu/opentofu/issues/741#issuecomment-1777544250

```
├── modules
│   ├── ≈ (first letter of the module namespace)
│   │   ├── bridgecrewio (module namepsace that starts with "b")
│   │   │   ├── apigateway-cors
│   │   │   │   ├── aws.json 
│   │   │   ├── bridgecrew-read-only
│   │   │   │   ├── aws.json 
│   ├── c (first letter of the module namespace)
│   │   ├── claranet (module namepsace that starts with "c")
│   │   │   ├── detectors
│   │   │   │   ├── signalfx.json 
├── providers
│   ├── a (first letter of the module namespace)
│   │   ├── aliyun (module namepsace that starts with "a")
│   │   │   ├── alibabacloudstack.json
│   │   │   ├── alicloud.json
│   ├── o (first letter of the module namespace)
│   │   ├── oracle (module namepsace that starts with "o")
│   │   │   ├── oci.json
```

There will be a folder for `modules` and a folder for `providers`

#### Providers

- The main `providers` folder should contain folders named with single characters (either letters or digits). Those represent the **first character of the provider namespace**
- In each "single character" folder (for example `a`) there will be folders per each provider namespace that starts with "a" (For example - `aliyun`)
- In each provider namespace folder (for example `aliyun`), there will be JSON files named with the name of the provider with `.json` suffix (for example - `alicloud.json`)
- In each provider namespace folder, there might also be a folder `keys` containing the public keys of GPG keys used to sign the provider artifacts (This part will be adjusted/elaborated once we have a design on managing GPG keys in the repository)

##### JSON file content

```json
{
  "repository": "https://github.com/aliyun/terraform-provider-alibabacloudstack",
  "versions": [
    {
      "version": "1.0.19",
      "protocols": [
        "5.0"
      ],
      "shasums_url": "https://github.com/aliyun/terraform-provider-alibabacloudstack/releases/download/v1.0.19/terraform-provider-alibabacloudstack_1.0.19_SHA256SUMS",
      "shasums_signature_url": "https://github.com/aliyun/terraform-provider-alibabacloudstack/releases/download/v1.0.19/terraform-provider-alibabacloudstack_1.0.19_SHA256SUMS.sig",
      "targets": [
        {
          "os": "windows",
          "arch": "amd64",
          "filename": "terraform-provider-alibabacloudstack_1.0.19_windows_amd64.zip",
          "download_url": "https://github.com/aliyun/terraform-provider-alibabacloudstack/releases/download/v1.0.19/terraform-provider-alibabacloudstack_1.0.19_windows_amd64.zip",
          "shasum": "f6644af3c6c5d41819315e7a80466f403d185c11fbf3d25bd4fad7cf208a3033"
        },
        {
          "os": "linux",
          "arch": "386",
          "filename": "terraform-provider-alibabacloudstack_1.0.19_linux_386.zip",
          "download_url": "https://github.com/aliyun/terraform-provider-alibabacloudstack/releases/download/v1.0.19/terraform-provider-alibabacloudstack_1.0.19_linux_386.zip",
          "shasum": "e7f8e9327fc706865dbcbbaa5adda2f350beae904bee386a9ee452280126d9db"
        }
      ]
    },
    ...
  ]
}
```

- `repository` - (optional) Used to help the automatic version bump process, to tell what repository to use when attempting to make API calls. Defaults to `https://github.com/<NAMESPACE>/terraform-provider-<PROVIDER_NAME>`. (Might not be necessary. This part will be adapted once we create a design doc for existing provider version update)
- `versions` - Contains each provider version
  - `version` - The provider version. Should always be in [semver](https://semver.org/) format (without a `v` prefix)
  - `protocols` - The list of supported plugin protocol of the provider version. This data could be inferred from the `_manifest.json` artifact, if it exists (otherwise, default to `5.0`)
  - `shasums_url` - Download link to the SHASUMs URL for the provider version artifacts
  - `shasums_signature_url` - Download link to the SHASUMs signature artifact
  - `targets` - Entity per released platform of this provider version
    - `os` and `arch` make up the platform
    - `filename` - The name of the artifact to download
    - `download_url` - The download URL of the artifact
    - `shasum` - The shasum of the artifact

#### Modules

- The main `modules` folder should contain folders named with single characters (either letters or digits). Those represent the **first character of the module namespace**
- In each "single character" folder (for example `b`) there will be folders per each module namespace that starts with "b" (For example - `bridgecrewio`)
- In each module namespace folder (for example `aliyun`), there will be folders named with the "name" of the module (for example - for module `bridgecrewio/apigateway-cors/aws` it's `apigateway-cors`)
- In each module name folder (for example `apigateway-cors`), there will be a JSON file named with the system of the module, with `.json` suffix (for example - for module `bridgecrewio/apigateway-cors/aws` it's `aws`)

##### JSON file content

```json
{
  "versions": [
    {
      "version": "v1.0.0"
    },
    {
      "version": "v1.0.1"
    },
    ...
  ]
}
```

The JSON file only contains the module tags that could be used as versions of the provider.
The versions should always be in [semver](https://semver.org/) format, optionally prefixed with a `v` (like `v1.1.1`), based on the actual tag in the GitHub repository of the module

### Justification for this folder structure

- Sharding the folders will help dealing with sheer amount of files that the repository will end up having, making the GitHub UI nicer and more responsive
- This simple folder structure makes it easy to see all of the relevant metadata of each provider and module easily

### Open Questions


### Future Considerations


## Potential Alternatives

