# RFC Tracking Issues

> [!NOTE]
>
> This is a _development process_ RFC rather than a feature RFC. It proposes a change to the workflow we follow for RFCs, rather than a software change in any of the OpenTofu codebases.

An earlier RFC proposed the current [OpenTofu RFC Process](https://github.com/opentofu/opentofu/blob/de9fb7ccca5d02b7b675a036993bc5edcbd28c05/rfc/20240524-OpenTofu-RFC-Process.md), which has done a good job of laying the groundwork for designing new features "in the open" and leaving behind a record of our past decisions.

As we continue movement from the project's bootstrapping phase into a sustained development phase, we're interested in further exposing the design and development process to the community, both so that the community can better understand what the core team is working on (or not working on) and why, and so that the community can hopefully participate more directly in this process over time.

This proposal is a reaction to three main observations about the current process, which are slightly overlapping but independent:

- OpenTofu features are rarely truly isolated from each other, and so choosing to implement one feature in a particular way will often constrain how some other feature can be implemented.

    The "Future Considerations" section of our current RFC template encourages discussion of how each proposal might constrain future development, but it's challenging to complete that section without awareness of what other features are being considered for the future, and what stage of design and development they are currently in.

    The core team _does_ have some awareness of what might be considered over the next few releases, but the horizon of that information is currently relatively short -- only one or two releases ahead -- and features that are not planned for very-near-term development don't tend to be well represented in locations that the community can easily consume, since we currently don't tend to write RFCs unless a feature is imminently planned for implementation.

- The current RFC process focuses primarily on the early life of a proposal where it moves from being proposed to accepted, but the initial proposal was vague about what ought to happen after an RFC has been accepted, and in particular how it's represented during the period between it being accepted and it being implemented and shipped in a final release.

    If we aim to develop a longer-term product vision and involve the community in its construction then it would be helpful to develop a library of accepted RFCs that describe likely future features _even if they are not immediately ready to implement_. These accepted-but-not-yet-scheduled RFCs can then serve both as more concrete references when writing the "Future Consideration" section of other RFCs, and as a potential entry point for community members who might be interested in taking on larger-scale feature development work.

    However, approving RFCs without immediately scheduling implementation creates some ambiguity: "accepted" no longer necessarily means "implementation can begin", and instead we need to represent some additional concepts like one feature being blocked on the completion of another, or implementation being intentionally put on hold to allow some related refactoring to be completed first, etc.

- Finally, there is the classic GitHub Issues problem that since anyone can open a new issue and control its lifecycle, community-created issues are good for gathering raw feedback but not very good for describing a curated roadmap.

    The original process document ends with "The RFC is Merged, and the Core Team creates issues in the relevant repositories to track the work required to implement the RFC" but in practice we have mainly been using original end-user-created enhancement request issues as a proxy for tracking the work, which can work to a point but causes some discoverability challenges in more complex cases:

    - End-user feature requests tend to focus on a particular use-case that might be solved in a number of different ways, while an RFC describes a specific concrete solution. The relationship between use-cases and RFCs is many-to-many; a particular use-case might cause multiple different alternative technical proposals to fix it, or a use-case might need to be met over the course of a series of incremental changes. A single RFC might also be aiming to meet multiple separate feature requests at once, and that's very desirable when it's possible!

    - End-user feature requests can be closed by the person who opened them at any time, which is disruptive if the issue has begun to be used as an RFC implementation tracking issue instead of its original purpose of capturing a use-case. Ideally we'd track RFC implementation using issues that are created by and managed by the core team so that we can control their lifecycle independently of the possibly-many feature requests that inspired them.

    - Although we can create links between issues and PRs, when an issue has already been heavily-used to discuss refinements of a use-case and negotiate requirements the comments discussing the implementation tend to get buried and hard to find, mixed in with other different kinds of discussion unrelated to implementation. GitHub issues is not well-suited to long discussions with many branching threads, so it's helpful for each issue to have one specific purpose and for there to be a clear definition of what would cause it to be considered as "completed", and therefore closed.

Overall then, this proposal aims to add a little more structure to the post-approval lifecycle for an RFC, hopefully creating a predictable pattern that the core team and the community can both rely on to understand what lifecycle phase each RFC is in, how each RFC relates to potentially-many feature request issues that inspired it, and which pull requests are related to each RFC.

This proposal _does not_ significantly change the earlier stage of the authoring and approval of an RFC, except for making it explicit that it's okay for an RFC to be approved and merged even when there is no immediate plan to implement it, since we'll now have a new way to represent that situation.

This proposal is also primarily discussing the process for _feature implementation_ RFCs, and not for other kinds of RFCs such as development process RFCs. Non-feature RFCs are currently far less common and so we don't have as much experience with them and thus it would be premature to try to make their process significantly more prescriptive.

(The ideas in this proposal are _broadly_ based on some similar ideas from the Rust project's RFC process, although the proposed details are simpler because the OpenTofu project's current scale is considerably smaller than Rust's.)

## Proposed Solution

As noted above, this proposal effectively expands on what is currently the last step in the existing RFC process document:

> 6. The RFC is Merged, and the Core Team creates issues in the relevant repositories to track the work required to implement the RFC.

Under this proposal, immediately after merging an accepted RFC the core team creates a single "tracking issue" that represents the work to implement that RFC. The tracking RFC title is always the prefix "RFC tracker: " followed by the same title used in the RFC document itself. For now, tracking issues belong in the main `opentofu/opentofu` repository regardless of which codebases the RFC proposes to modify, though [we might change that in future](#moving-rfcs-and-their-tracking-issues-into-a-separate-repository).

The tracking issue description includes the following information:

- A link to the RFC text itself, and the PR(s) where the document was previously discussed.
- Links to each of the feature request issues that were identified at the beginning of the RFC text, just to make backlinks appear on those other issues linking to the tracking issue.
- A checkbox list of blockers that must be cleared before implementation could begin, or at least before any implementation could be merged. If the blockers are work described by other RFCs, these items link directly to each other RFC's tracking issue.

    (It may be appropriate to begin a partial implementation that then remains unmerged until blockers are cleared, but the author must understand that their initial implementation may need significant revisions or replacement once the blockers have been resolved.)
- A checkbox list summarizing the work that must be completed before the RFC is considered fully implemented. This includes both the direct development work and other related tasks such as updating the documentation and coordination with other projects that might be run concurrently.

    In particular, any pull requests that are intended as progress towards the completion of the features described in the RFC should be directly linked as checklist items so that there is a single place to find work that was already completed and work that's awaiting review, to help avoid accidental duplicate or conflicting work between people working asynchronously.

An RFC tracking issue is tagged with the same "rfc" label we currently use for RFC pull requests, since the tracking issue represents the remaining work related to a specific RFC. [A label search for "rfc"](https://github.com/opentofu/opentofu/labels/rfc) would therefore return both the not-yet-approved RFCs represented as pull requests and the accepted-but-not-completed RFCs represented as tracking issues.

An RFC tracking issue should also have exactly one of the following labels summarizing its current status:

- "rfc-blocked": There are any blockers listed in the description that are not yet resolved, and so final implementation should not begin.
- "rfc-ready": All blockers are resolved so implementation would could potentially begin, but implementation work is not yet assigned to anyone. Anyone in the community could potentially volunteer to work on an RFC in this stage, though they should discuss their intention first and wait for transition into the next phase before writing any code that's intended to be merged.
- "rfc-implementing": One or more specific individuals have signed up to implement what's described in the RFC and are actively working on it. At this stage the tracking issue should also be assigned to each of the assigned individuals.

While the issue is in the "ready" or "implemented" phases it's possible that the folks working on it will learn of new blockers that were not originally anticipated, in which case they can make a judgement call about whether the blockers are significant enough to return to the "rfc-blocked" state or if they can be resolved promptly.

The tracking issue conventionally "belongs to" some specific individual who is acting as a project owner for the implementation. In the short term that is likely to be a core team member, but in the long run could potentially be a community member who is willing and able to take on that role. That individual is ultimately responsible for keeping the main issue description and labels updated as the status changes, which implies that the individual must either be the author of the issue or must have "triage" access to the repository under [GitHub's access control scheme](https://docs.github.com/en/organizations/managing-user-access-to-your-organizations-repositories/managing-repository-roles/repository-roles-for-an-organization). In practice the project owner may of course delegate some or all of these responsibilities to others; the requirement is only that there be a single person accountable for knowing the overall status of the work and able to communicate that with other stakeholders.

Discussion about the implementation of the RFC should happen primarily in the comments on the tracking issue; the overall goal is that anyone who is interested in the project can find everything related to it either directly in the tracking issue or by following links from the tracking issue. Any linked feature request issues can continue to collect additional use-case examples and any other relevant information participants wish to share, without those becoming mixed in with the implementation-specific discussion in the RFC tracking issue.

An RFC tracking issue is "closed as completed" (in GitHub's terms) once all of the code required for it has been completed and merged into the appropriate release branch. When closing the tracking issue the project owner, possibly after discussion with other stakeholders, should decide whether the completion of the RFC has completely satisfied each of the linked feature request issues and close them if so. It's okay for an RFC to only partially solve a problem, if there's a reasonably-clear path to fully solving the problem in future work, in which case the project owner should describe the situation in a comment on the original use-case issue and leave it open to represent the need for future work.

In some (hopefully-unusual) cases we might choose to abandon work on an RFC after it was accepted. In that case, the tracking issue is "closed as not planned" with a closing comment explaining the decision and, if appropriate, what new information could potentially cause that issue to be reopened. The project owner should typically also add a note to the related feature request issues explaining that the specific RFC has been abandoned. The feature request issue can potentially remain open if new RFCs are welcomed, or closed with an explanation if the team has concluded that it's _the use-case itself_ that is invalid or unwanted, rather than the specific technology proposed to meet it.

### RFC Amendments

The "idealized" process treats technical design and RFC authoring as an entirely separate phase before implementation begins, but in practice the boundary between these phases is considerably more permeable: we often use implementation prototypes to inform the design included in RFC, and we also sometimes learn new information during implementation (or otherwise after initial RFC approval) that justifies revising the previously-accepted design.

Under this proposal, a decision to revise an RFC would be represented either as a blocker or as an item of work in the RFC's tracking issue, and then a new PR linked to the tracking issue proposes the amendments directly to the existing document, replacing previous content as appropriate. If the proposed amendements are approved then the new text becomes the authority on what work is planned and the project owner should update the tracking issue as needed to reflect the new set of work.

Once an RFC's tracking issue has been closed to represent that work on it is complete, the RFC content is frozen as a historical record of that work. If we later plan other work in the same area -- which might in some cases conflict with what was previously proposed and accepted -- we should write a new RFC that describes its proposed changes relative to the outcome of the previous RFC.

One exception to the frozen state: if a new RFC is proposed that materially changes a decision made and recorded in an earlier accepted RFC, the PR for the new RFC should add a note in each relevant part of the older RFC that mentions that the decision has been changed and links to the new RFC, so that it's clear to a future reader of the old RFC that part of it has been obsoleted retroactively. Consider using [GitHub Markdown's "alert" extensions](https://github.blog/changelog/2023-12-14-new-markdown-extension-alerts-provide-distinctive-styling-for-significant-content/) to clearly separate the later-added content from the original frozen content, and add the new markers in the same PR that introduces the new RFC so that the changes are more strongly connected in the history.

The "chains" of linked RFCs that might be caused by the frozen state could potentially become quite long over time for product areas that experience frequent change, but thankfully older RFCs tend to become less and less relevant over time. Any significant feature of the product or its codebases should also be documented in some separate location in a "how it currently is" shape, rather than a "how we changed it over time" shape, so that it isn't necessary to piece together information on currently-available features by trawling through the RFC history unless the historical change decisions _themselves_ are the point of interest.

Some existing locations for longer-lived supporting documentation include:

- [The OpenTofu docs](https://opentofu.org/docs/), maintained in the `website` directory in the root of the main OpenTofu repository, for end-user oriented documentation.
- The `docs` directory in the root of the main OpenTofu repository, for developer-oriented descriptions of system architecture and the design of non-trivial features.
- The `CONTRIBUTING.md` file and the RFC `README.md` file in the OpenTofu repository, for contributor oriented documentation about the development process rather than the code itself.
- The documentation comments directly within our Go packages, attached to individual symbols and whole packages, for developer-oriented descriptions of fine-grain details specific the implementation of certain features.

The tracking issue for an RFC should include work items for documation artifacts that the implementation team intends to create or update before the project is concluded, to reduce the risk of this step being forgotten. It is unlikely that any change large enough to justify an RFC does not also justify either an end-user-oriented or developer-oriented documentation change.

### Building a Roadmap

Part of the motivation for this proposal is to develop a library of forward-looking RFCs that we can use to plan both near-term and longer-term work, and do so in a form that the community can interact with.

The topic of how to build and share a public roadmap is a large one beyond the scope of this proposal. The OpenTofu Core team currently maintains an informal set of GitHub issues at various stages of development which serves as a _de-facto_ roadmap for now, and so this proposal will focus only on slightly adjusting the use of that.

The primary proposed change is to recognize "research and design" (moving from feature request to RFC) as a separate activity from "implementation" (moving from RFC to shipped code, docs, etc) and differentiate them on the de-facto roadmap:

- When representing concepts like "most popular requests" that might suggest research and design work, we are collecting a set of community-submitted feature request issues that don't yet have accepted RFCs.
- When representing planned or committed implementation work though, the tracking system should refer instead to _RFC Tracking Issues_ because they represent a concrete set of work with a clear definition of "complete", and are under the direct control of the project owner.

The exact details of where these sets of issues should be captured and how they should be shared for community consumption are deferred to a later RFC. For now, the core team will continue to track them in the same informal way aside from the change of using RFC Tracking Issues instead of feature request issues when tracking already-committed implementation work.

### Flexibility is Key

This RFC proposes replacing an organic, ad-hoc process with a more rigid process. However, it's important to keep in mind that any process like this is a means to an end rather than an end in itself.

Although this proposal is hopefully applicable to _most_ feature development work involving RFCs, participants remain free to make exceptions or adjustments to this process if they believe it justified for a particular situation. Those making such exceptions are encouraged to document the reason for them in some way, and to keep in mind the goals that motivated this proposal to make a best effort to preserve those goals even when following a different strategy. But overall: if you find that any part of the process is hindering more than it's helping, adjust as needed!

Of course, if we find that we're making the same exceptions and adjustments repeatedly then that might suggest that we need to revise the process further in a new RFC.

### Open Questions

#### Reusing existing issue label names?

We already have some existing issue labels that could potentially be used instead of the "rfc-" prefixed ones proposed above:

* "rfc-blocked" could use the existing label "blocked", which seems to carry the same meaning while being non-RFC-specific.
* "rfc-ready" could use the existing label "help wanted", which we seem to have used to represent "open to community contributions" in the past, or "accepted" which seems to have a similar meaning.
* "rfc-implementing" doesn't seem to be analogous to any existing label, though we might decide that it's reasonable to represent this state just by having at least one person assigned to the tracking issue, rather than using any specific label.

There are some other existing labels that could potentially be applied to RFC tracking issues:

* "core team" represents work that the core team has reserved for implementation by its own members, rather than by community contributors, for some special reason. Perhaps these special reasons would sometimes apply to RFC implementation.
* "good first issue" represents work that might be accessible to newcomers to the codebase, although it's not clear whether anything that has significant enough scope to require an RFC would generally be accessible to newcomers.
* "pending-decision" and "pending-steering-committee-decision" could both represent specific reasons why a tracking issue is also labeled as "rfc-blocked" or "blocked.
* "upstream-fix-required" could be used if one of the unresolved blockers listed in an RFC issue requires a change to an upstream codebase.

Reusing existing labels might make it easier to include RFC tracking issues in other processes that are not RFC-specific, such as general issue triage.

#### Moving RFCs and their tracking issues into a separate repository

The original RFC process proposal included the following "Future Consideration":

> For now, we are proposing to keep all of the RFC files in the main OpenTofu Repository. We believe that this helps with discoverability and co-locates it with the majority of the ongoing development effort. This does have some downsides however, primarily in waiting on the required GitHub actions (testing) that are run for all pull requests. If we ever decide to change this location to either RFCs in a single separate repository or per-repository, the specific history can easily be pulled over using standard git commands.

The fact that an RFC tracking issue's description must be editable by the project owner means that either the project owner must directly own the tracking issue, or they must have at least "triage" access to the repository containing those issues under [GitHub's access control scheme](https://docs.github.com/en/organizations/managing-user-access-to-your-organizations-repositories/managing-repository-roles/repository-roles-for-an-organization).

If non-core-team members could potentially act as project owners in future then having the owner create the tracking issue is a potential workaround but would be challenging if the project owner needed to step down and be replaced by another community member, so it's quite possible that in the hypothetical future state with non-core-team project owners we will want to give community project owners triage access to the repository, which would be an easier tradeoff to make if the RFCs and their tracking issues were in a separate repository with its own set of approved collaborators.

This particular question is unlikely to be important in the near future because we don't yet have any defined process for a non-core-member to drive a larger feature development project. It seems safe to defer this question until later, although at that point we may need to bulk-migrate existing open tracking issues to a new repository using GitHub's existing issue transfer tools.

#### Labelling automation

Some other projects like Rust use automation to drive the labelling and other lifecycle management of tracking issues, which can help reduce the number of people who need direct "triage" access to a repository by having them take those actions only indirectly via the automation.

At our current volume of RFCs, particularly when many of them are being driven directly by the OpenTofu core team, such an investment does not seem justified. If we start to find the issue management overhead too great in the future then we can look to other open source projects for inspiration on what tooling might be valuable.

### Future Considerations

#### Milestone-based vs. release-train-based release cycle

So far OpenTofu has been following what I'll describe as a "milestone-based" release cycle, where the plan for a release prioritizes scope over delivery date: a release typically has a set of "headline features" and the final release is made only when those features are ready, unless the team chooses to make an exception.

There are definite benefits to such an approach: it makes it easier to coordinate marketing activities around releases, and to choose projects that complement each other well for each release. It's also a relatively low-cost approach when work is being done primarily by a core team working closely together with opportunities for regular synchronous communication, and the ability to commit to meet a deadline.

However, this approach is more challenging for community-driven work. Community-driven work is done at the contributor's convenience rather than the core team's convenience, and so instead of the core team planning releases around that work it's more appropriate to ask the contributors to plan their work around the releases, which I think is best achieved by releasing on a documented regular schedule with a "release train" approach: if your work is complete before the pre-announced cutoff date then it can be included in the release, but if not then it can just wait for the next release.

Existing large open source projects with successful community development seem to aim for a compromise in practice. For example, the [Go Release Cycle](https://go.dev/wiki/Go-Release-Cycle) currently defines two fixed development periods per year that each end in a release preparation period. Work that is not completed before the pre-scheduled "freeze" waits for the next release, which is also at a pre-scheduled time. However, the schedule is defined loosely in whole weeks rather than exact dates, and I believe (from external observation) that releases are sometimes intentionally held back slightly later to provide _some_ flexibility for unexpected problems while still remaining relatively predictable.

I believe this RFC is largely orthogonal to the question of whether our releases are planned prioritizing scope or prioritizing time. A nice benefit of having a relatively-structured way of tracking project work is that we can either consider a particular set of open RFC tracking issues as "release blockers" for the milestone-based model, or we can use the tracking issues as (effectively) independent "roadmaps" for each project, tracking each one's development progress separately across potentially-many release cycles in the release-train model.

#### Experimental Features

As we take on more ambitious projects, it's often helpful to be able to merge only part of a feature either to reduce the scope of each change or to gather early feedback on work in progress. An RFC tracking issue could potentially be a good venue to solicit early testing and feedback of a larger feature.

The OpenTofu codebase has some existing mechanisms for withholding certain features unless the user is running a special build that was configured to include experimental features. This mechanism was loosely based on the Rust project practice of shipping early versions of new features initially only in nightly builds, to allow users to try out early versions of features they feel enthusiastic about while making it clear that those features are incomplete and still subject to change.

The OpenTofu build process does not currently activate experiments for any kind of release, so this capability is unused. In future we could potentially choose to issue nightly builds (or development snapshots on some other schedule) with experiments enabled and then the team working on a project can encourage those who are monitoring their RFC's tracking issue to try the incomplete work in a non-production setting so that they can encourage feedback much earlier in the development process, rather than feedback mostly arriving only at the last moment during the prerelease period with beta and release candidate builds where it's often too late to make significant design changes.

(The experiments mechanism is currently largely limited only to language features decoded by the `configs` package and CLI features handled by the `command` and `backend` packages, due to how the experiment flag is "plumbed in" to existing packages, but it can potentially be expanded to other contexts if needed as long as the experimental code can be suitably isolated from non-experiment-enabled builds.)

## Potential Alternatives

* Change nothing: continue with the current informal process where we decide on a case-by-case basis what level of tracking is appropriate for each RFC.

    It's possible that the level of structure proposed in this document is too much for the current stage of our project and that it'd be better to wait until we have more experience and a larger community before discussing what new structures we might need.

    However, members of the core team have already noted that it's hard to keep track of the state of development of larger projects that they aren't directly working on; I have to assume that this is even harder for community members who don't necessarily have frequent synchronous communication with the core team.

* Transform feature request issues into tracking issues once the RFC is settled.

    It's arguable that by the time an RFC has been approved the need for continued discussion about use-cases and requirements is significantly reduced, and so we could simply switch to using a specific feature request issue _as_ the tracking issue once an RFC has been approved and merged.

    This is technically possible as long as the project owner has "triage" access to the repository, since they can then unilaterally change the original issue title and description to be suitable for implementation tracking rather than requirements-gathering. However, it seems quite rude to drastically repurpose an issue someone else created, particularly if that causes text that they didn't write to be attributed to their GitHub user account.

    This would also lead to some ambiguity in the more complex situations where a specific use-case generates multiple RFCs or a single RFC aims to fix multiple use-cases. It remains to be seen how often that will arise, but we've already seen at least one example in [Dynamic Provider Instances and Instance Assignment](https://github.com/opentofu/opentofu/pull/2088) which proposes an additional set of capabilities building on what was already discussed in another RFC, but where we'd likely want to leave the new RFC "on hold" at least until the work of the original RFC has been completed and we've receieved initial feedback about it.

* Use the RFC pull request itself to track implementation, instead of a separate "tracking issue".

    This proposal assumes that we want to merge an RFC, and thus close the PR that proposed it, to represent "approval". We could instead represent approval as a new label and use the RFC PR itself to track similar information as this document proposes for a separate tracking issue.

    However, that implies that we'd delay merging the RFC document into the repository until all of the work on it is complete. That process does admittedly make the RFC Amendments process less clunky -- authors can just push more revisions to the same PR as work proceeds -- but it means that the approved-but-not-shipped RFCs will be discoverable only through the list of open pull requests and not via the source code repository itself.

## Implementation

Since this is a process-only proposal there are no code changes required and no tracking issue required.

However, if this proposal accepted then we will add some external-contributor-oriented information about the usage of and purpose of RFC tracking issues to [the RFC process README](https://github.com/opentofu/opentofu/blob/de9fb7ccca5d02b7b675a036993bc5edcbd28c05/rfc/README.md), including adjustments to the existing section on "Amending an RFC" to discuss the new "frozen" state introduced by this proposal.
