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

Once an RFC is Accepted and Merged, the Core Team typically creates a Tracking Issue to represent all of the work required for the RFC in a single place, as described in the next section.

## RFC Tracking Issues

An RFC tracking issue is a GitHub issue used to represent all of the work required to produce the result described in an accepted RFC.

An RFC is more a technical design proposal than a project plan, and so the initial creation of an RFC tracking issue effectively requires translating the RFC's goals into a set of concrete work items that would, once completed, cause the product to behave as described in the RFC. We use a separate tracking issue because an issue can be updated independently of changes to the main repository content, and so we can more easily keep it up-to-date and respond to new information by changing our implementation plan as we go.

An RFC tracking issue shold be closed once enough work has been completed to meet the goals described in the RFC, possibly including RFC amendments as described in the next section.

There is more information on RFC tracking issues in [the process RFC that proposed the use of tracking issues](./20241023-rfc-tracking-issues.md).

## Amending an RFC

An approved RFC is not frozen; approval represents that consensus has been reached and so work can begin.

During the work described in the RFC's tracking issue, the project team might find that a different design is needed, in which case they can open a new PR to amend the existing RFC. The new PR should typically be linked from the RFC's tracking issue so that observers can understand retroactively how the design evolved.

We typically consider an RFC to be frozen after its tracking issue has been closed, because the RFC then describes the result of the project that the tracking issue represented. However, if a subsequent project proposes to change a decision that was made in an older RFC it's helpful to propose an amendment to the older RFC that introduces a compact [`[!NOTE]` callout](https://github.com/orgs/community/discussions/16925) near to the affected content that notes that the decision was invalidated by a later RFC and links to that later RFC, so that future readers are less likely to be misled by stale RFC content. Other amendments to "frozen" RFCs are not recommended, but might be accepted with sufficient justification on a case-by-case basis.

For more details, refer to [RFC Amendments](./20241023-rfc-tracking-issues.md#rfc-amendments) in the Tracking Issues RFC.
