# Setting up your own repo for testing the release process

## Github Secrets:
* Generate GPG Key:
  - `gpg --full-generate-key`
  - `gpg --list-secret-keys --keyid-format=long`
  - `gpg --armor --export-secret-keys <ID> | base64`
  - Add gpg environment in github project settings
  - Paste full output of export command into `GPG_PRIVATE_KEY`
* Add go-release pro key as `GORELEASER_KEY`
* Login to ubuntu-one account (snapcraft)
  - `snapcraft export-login snapfile`
  - `snapcraft register <username>-opentofu`

## Repo Changes:

### release/.goreleaser.yaml
* Update `ghcr.io/opentofu/opentofu` to `ghcr.io/your_user_name_here/opentofu`.
* Change snapcrafts -> name to the snap name you registered above.

### .github/workflows/release.yml
* Change any "large-runners" to "ubuntu-latest"
* Disable packagecloud by adding "&& false" to the if condition

## Caveats:

### Snapcraft:

Snapcraft "classic" is no longer supported.  Expect this step to fail with a clear error.

We should upgrade our snapcraft packaging to use a more modern format.

#### Helpfull stuff:
* https://dashboard.snapcraft.io/
* https://snapcraft.io/<your_snap>/releases
* `snapcraft revisions <your_snap>`
* `snapcraft releases <your_snap>`
* `snapcraft status <your_snap>`
    
