# Backport Merged Pull Request

## Overview

This feature automates the process of creating pull requests for backporting merged changes to a designated target branch.

### How to Use

1. **Label Your Pull Request**: To initiate a backport, simply add a label to your pull request in the format `backport <target_branch>`, where `<target_branch>` is the branch you want to backport changes to (eg. `backport v1.x`). The backport labels can be added to both open and closed pull requests.

2. **Automatic Backport Pull Request Creation**: After the pull request is merged and closed, or if the backport label is added to an already closed pull request, backport GitHub Action will execute the `.github/scripts/backport/main.sh` script, which will automatically create a new pull request with the changes backported to the specified `<target_branch>`. A comment linking to the new Backport Pull Request will be added to the original pull request for easy navigation.

### Handling Merge Conflicts

- In some cases, the automatic backport process may encounter merge conflicts that cannot be resolved automatically.
- If this occurs, a comment will be posted on the original pull request indicating the conflict and providing the commit SHA causing the issue.
- When a merge conflict arises, you'll need to manually backport your changes to the target branch and resolve any conflicts as part of the manual pull request process. [Please see the contributing guide for more details](https://github.com/opentofu/opentofu/blob/main/CONTRIBUTING.md#backporting)

### Note

- **Use Clear Labels**: Ensure the `backport <target_branch>` label is correctly formatted.