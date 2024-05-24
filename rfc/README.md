# The OpenTofu RFC process

If a larger body of work is needed for an enhancement in the [main OpenTofu repository](https://github.com/opentofu/opentofu) or other OpenTofu repositories, the core team often asks for an RFC to be written on how a feature should be designed and implemented. We do this in order to make sure everyone is on the same page how a feature should be implemented. This repository holds past RFCs and anyone can contribute RFCs.

We generally recommend [opening an enhancement issue](https://github.com/opentofu/opentofu/issues/new/choose) before going through the effort of writing an RFC. Not all enhancements require an RFC, but the core team may call for an RFC to be written by adding the `needs-rfc` label.

## Filing an RFC

Filing and getting an RFC merged is intentionally a longer process. We use pull requests to fully define and discuss technical solutions to a feature request. 

Before filing an RFC, ideally open an enhancement issue in the [main repository](https://github.com/opentofu/opentofu) in order to discuss if the feature has enough community support to warrant working on an RFC. This step is optional, but highly recommended.

To file an RFC, please create a PR by copying the contents of the [template](template/) into a new folder. The name of the folder does not matter at this stage, the core team will assign it an RFC number when it is accepted.

> [!NOTE]
> It's ok to file an incomplete RFC. Please submit it as a draft pull request to get early feedback.

## Working on an RFC

RFCs take a longer time than simple issues. Core team members and community members will comment on your pull request and request changes or clarifications. The main goal of the RFC is to serve as a documentation to even the casual reader on what this feature does and how it will be implemented.

## Merging the RFC

Mark an RFC as ready for review once all technical details of the implementation are clear. Once you mark it as ready for review, the core team will take an in-depth look at it. If more clarifications are needed, the core team will move the RFC back to draft, otherwise it will be merged, indicating that it is open for implementation. For each merged RFC, the core team will open one or more implementation issues in the main repository to track the progress of work.

Just before the RFC is merged, rename the folder according to this pattern:

```
{RFCNUMBER}-{TOPIC}
```

For example:

```
0001-new-rfc-process
```

## After an RFC is merged

RFCs are not set in stone. If you realize that parts of the implementation won't work, feel free to amend the RFC in a subsequent PR. Doing so will serve as an important point of discussion if something doesn't go according to plan.

Once your feature is implemented (by you or someone else), consider if it is worth copying the RFC into the [docs](https://github.com/opentofu/opentofu/tree/main/docs) folder of the OpenTofu codebase.
