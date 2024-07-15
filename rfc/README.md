# The OpenTofu RFCs

This folder contains Request For Comment (RFC) documents that have been discussed, reviewed, and accepted. They represent a proposal of changes to one or more of the repositories in the OpenTofu organization.

RFCs are primarily created by the OpenTofu Community in response to one or more GitHub issues that require a more in-depth discussion than the GitHub Issue process can provide. They are organized by date to help show the progression of concepts over time.

## Authoring an RFC

When an Issue is given the `needs-rfc` label, any community member may propose an RFC by following these steps:
1. Copy the [yyyymmdd-template.md](./yyyymmdd-template.md) to `./rfc/${isodate}-${rfc title}.md` on a branch in their fork of the OpenTofu Repository
2. Edit the newly created Markdown file and fill in the template fields
3. Submit a Pull Request in the OpenTofu Repository, linked to the open issue(s)

> [!NOTE]
> It's ok to file an incomplete RFC. Please submit it as a draft pull request to get early feedback.

Once an RFC is submitted, community members discuss the RFC in detail until all open questions are resolved

To Accept an RFC, the majority of the OpenTofu [Core Team](../MAINTAINERS) must approve the Pull Request. If a consensus is not reached, the Pull Request is closed and the Core Team may ask for a new RFC or close the original issue entirely.

Once an RFC is Accepted and Merged, the Core Team creates issues in the relevant repositories to track the work required to implement the RFC.

## Amending an RFC

RFCs are not set in stone, Approval signifies that an initial consensus has been reached and work can be started. If you realize that parts of the implementation won't work, feel free to amend the RFC in a subsequent PR. Doing so will serve as an important point of discussion if something doesn't go according to plan.
