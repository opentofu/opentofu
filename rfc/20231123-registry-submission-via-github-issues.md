# Adding a new Provider/Module to Registry via GitHub issues

Issue: https://github.com/opentofu/opentofu/issues/916 

> [!NOTE]  
> This RFC was originally written by @roni-frantchi and was ported from the old RFC process. It should not be used as a reference for current RFC best practices.

This proposal is meant to complement the selected new [Homebrew-inspired registry](https://github.com/opentofu/opentofu/issues/741.), and as an alternative to https://github.com/opentofu/opentofu/issues/895.  

## Proposed Solution

### User Documentation

Have packages submitted via GitHub [issue forms](https://docs.github.com/en/communities/using-templates-to-encourage-useful-issues-and-pull-requests/syntax-for-githubs-form-schema):  
![1](https://github.com/opentofu/opentofu/assets/3658029/5a5cf27f-854b-451c-8bc0-872005912b9b)
![3](https://github.com/opentofu/opentofu/assets/3658029/ed2dd7d5-be71-4020-ae14-fa87db2df6e7)

### Technical Approach

1. Have packages submitted via GitHub [issue forms](https://docs.github.com/en/communities/using-templates-to-encourage-useful-issues-and-pull-requests/syntax-for-githubs-form-schema)
1. Have a GitHub action that upon matched GitHub issue creation/edit will use the submitted information to:
1. Validate the information provided
1. When valid  
    1. Create a new branch where the submitted data is captured in the the proper sharded directory structure/metadata-json
    1. Create a Pull Request for said branch [description will say closes: #issue to auto-close that issue when merged]
    1. Create Pull Request for GPG key submission
1. When invalid  
    1. Will comment back on issue mentioning author asking them to fix what isn’t valid
    1. Upon edit GitHub action will trigger once more

### Open Questions

#### Pros
- A neat, familiar and secure user interface for proposing modules/provider
- Completely automated whenever possible
- Issues provide submission visibility, allow for conversations to occur in context and on issue when manual admin/core team intervention is required
- No CLI or additional installs required to submit a provider, less maintenance on client-side issues/distribution by core team members
- No GitHub tokens or other means of CLI based authentication required
- Abstracts away any codebase implementation detail like directory sharding logic/code intricacies otherwise required when submitting new provider/module/key directly
- Visible history/timeline for submission/acceptance/deny

#### Cons
- Will contribute to GitHub action cost (though probably nothing significant) 
- Since the PRs are generated, git history won't track to a _signed_ commit by submitter
- Forks may cause some loss of audit history as Issues do not transfer with forks

### Future Considerations

#### Reuse the approach for when revising _existing_ packages
The approach described on the PRD is for submitting new Provider or Module, but it can also be extended to say, submitting a GPG key rotation, ask for removal or replacement of faulty version or forcefully bump a new version beyond that cron based version bump - all in the same familiar manner.  

#### GitHub CLI
I have also had the notion of being able to do so from GitHub CLI by going:
```
gh issue create -R 'opentofu/registry-stable' -T provider.yml
```

and following form prompt, but it seems like [issue forms aren’t supported yet by CLI](https://github.com/cli/cli/issues/5865), but perhaps would be a good CLI approach patching into the same mechanism later once GitHub supports forms in its CLI.  

## Potential Alternatives

