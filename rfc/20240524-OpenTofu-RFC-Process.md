# OpenTofu RFC Process

Issue: <N/A>

As the OpenTofu project evolves, the community has been proposing more advanced concepts and ideas that frequently need significant discussion and iterations of feedback. Our Current RFC Process is cumbersome and hard to follow due to the following limitations: single user can edit, pages of comments are overwhelming, sub discussions are not easily possible.

As we move forward, a new more transparent and flexible RFC process is needed. We start by investigating the other successful open source communities with similar goals.
* https://github.com/rust-lang/rfcs/blob/master/0000-template.md
* https://peps.python.org/pep-0001/
* https://github.com/openshift/enhancements

These popular and well tested solutions all follow a similar format:
1. Single file to track an RFC committed to the repo
2. Markdown format used in most cases
3. Public GitHub Pull Request serves as a dynamic avenue for discussions and threads therein
4. Easily proposed feedback in the form of `suggestions` and pull requests
5. Template provided as a starting point, but not a rigid requirement

## Proposed Solution

OpenTofu should adopt a Pull Request based RFC process and should introduce the process with the understanding that it will be modified as we gain experience with it.

### User Documentation

#### Readers of RFCs

Community Members use RFCs in two main ways:
1. Implementing a concept that has been discussed
2. Learning about the OpenTofu Project and understanding previous decisions made

Both of these users will be looking for a single location, which contains organized RFC documents which have been approved.

We therefore propose that Markdown files which contain RFCs are located within the `./rfc` folder in the main OpenTofu Repository. This folder will contain files that follow the format of `./rfc/${isodate}-${rfc title}.md`, which allows easy searching and sorting of accepted RFCs. Additionally, RFCs that have not yet been accepted will exist as Pull Requests labeled with `rfc`.

In the case that a single MD file is not sufficient for describing a RFC, a folder named `./rfc/${isodate}-${rfc title}` should be created to contain supplementary information such as diagrams or detailed technical explorations. These supplementary files should be linked to from the main Markdown file for the RFC.

RFCs should link to the issue(s) that originally required the more in-depth process that an RFC provides. Additionally, issues which are created to track the implementation of an RFC will link to that RFC. This allows anyone encountering each of these distinct pieces to easily gather a view of the whole process.

#### RFC Authors and Reviewers

The following will be split between [CONTRIBUTING.md](../CONTRIBUTING.md) and [rfc/README.md](./README.md):

The process starts with an issue (`enhancement` or `bug`) is submitted to the repository. The `needs-rfc` label is added during the Core Team Triage process and indicates that an in-depth discussion and consensus is needed. This facilitates conversations and discussions around complex issues that are hard to have in the GitHub Issue format.

If interested, a Community Member will take the following actions to submit an RFC:
1. Copy the [./rfc/yyyymmdd-template.md](link) to `./rfc/${isodate}-${rfc title}.md` on a branch in their fork of the OpenTofu Repository
2. Edit the newly created Markdown file and fill in the template fields
3. Submit a Pull Request in the OpenTofu Repository, linked to the open issue(s)
   - A Draft Pull Request is recommended if early feedback or help is needed to fully fill out the template
4. The Community Members discuss the RFC in detail until all open questions are resolved
5. The majority of the Core Team Approves the RFC Pull Request
   - The Team Lead has the ability veto an RFC or to escalate to the Technical Steering Committee
   - If a consensus is not reached, the Pull Request is closed.
   - The Core Team may ask for a new RFC or may close the original issue entirely.
6. The RFC is Merged, and the Core Team creates issues in the relevant repositories to track the work required to implement the RFC.

> [!NOTE]
> The subsequent proposal [RFC Tracking Issues](./20241023-rfc-tracking-issues.md) has introduced some additional structure for the final step, with specific guidance on where to create and how to use an RFC tracking issue.

### Technical Approach

No automation is proposed as part of this process, however we are using the RFC Template proposed in this PR to "dog-food" the process.

No additional labels are required in the repository.

Updates to the CONTRIBUTING.md are required as described above and the existing RFC template will need to be removed.

### Open Questions

* Should we use ISODATE or assigned SERIAL in the RFC file name
  - ISODATE could either be proposed date or accepted date
  - SERIAL is clearly linear, but requires additional processing once the PR is Accepted.
  - For now we are using proposed date, but automation could easily be added to amend the RFC once it has been merged.
* Should we backport some or all of the current RFCs accepted into OpenTofu to additionally validate this process?

### Future Considerations

#### Moving to a separate repository

For now, we are proposing to keep all of the RFC files in the main OpenTofu Repository. We believe that this helps with discoverability and co-locates it with the majority of the ongoing development effort. This does have some downsides however, primarily in waiting on the required GitHub actions (testing) that are run for all pull requests. If we ever decide to change this location to either RFCs in a single separate repository or per-repository, the specific history can easily be pulled over using standard git commands.

As mentioned in [Open Questions](#Open-Questions), we may build automation using GitHub Actions to post-process RFCs once they are accepted and merged.

## Potential Alternatives

The main alternative to this process is to keep the existing process. It does work in some capacity despite the limitations described above.

There are more "formal" RFC processes such as the [IETF RFCs](https://en.wikipedia.org/wiki/List_of_RFCs). We don't believe we need that level of formality and detail at this juncture.
