# Improvements for Contribution Documents

[Tracker for documentation-related issues](https://github.com/opentofu/org/issues/30)

> "For an absolute beginner, what's the best way to contribute and get involved?" [^1]

[^1]:[OpenTofu Developer Panel, KubeCon 2025](https://youtu.be/JSuKXfiFAzI?si=kjFs9U6lNJ8zHE6Y&t=50)

OpenTofu continues to grow, and as it does, maintainers often field questions involving contribution. Documentation is one of many tools maintainers use to scale communication, and documentation around contribution is no different. That's not to say maintainers should respond to inquiries with terse references to documents. In fact, projects are benefitted more when maintainers closely coordinate with the community at large: in one study, it was found that rapid, high-volume communication contributed more to a project's success than "heroic" acts of programmery[^4]. Therefore, this present RFC should not be seen as a way to offset communication, but to improve its efficiency.

Studies of current trends, both anecdotal[^2] and empirical[^3], have noted the open source software developer community at large usually makes casual contributions, which provide only a small fraction of the code relative to the contributions of the projects' maintainers. Despite the paucity of community contributions, and transitory nature of their developers, the majority of them are nontrivial and vital. They often constitute a one-time bug fix or enhancement. Community remains the lifeblood of open source software; OpenTofu is no different.

[^2]: "[Casual contributors] do not want to spend time familiarizing themselves with a project's ... contribution process." - "Working in Public", Nadia Eghbal
[^3]: [More Common Than You Think: An In-depth Study of Casual Contributors, 2016](https://ieeexplore.ieee.org/document/7476635)
[^4]: [Communication and Code Dependency Effects on Software Code Quality, 2022](https://arxiv.org/pdf/1904.09954)

In light of these facts, this RFC recommends these standards for our documents and issues:

 * Contributing documentation should be standardized, both within OpenTofu and relative to the wider open source community
   * We want to make it as smooth as possible to onboard onto OpenTofu's core and its satellite projects, aimed towards the casual contributor
 * Issue labels should be standardized, both within OpenTofu and relative to the wider open source community
 * Provide the full breadth of "contributable" repositories, and prepare those repositories for contributions
   * If someone wanted to contribute to this project, they may only know of `opentofu/opentofu`, if even that. It may not be clear that one could contribute to the language server, or website, or even brand artifacts.

## Short-Term goals

In order to best serve these standards, we recommend the following course of action:

### Determine which repositories can be contributed to

https://github.com/opentofu/org/issues/23

For lack of a better term, a "supervised" repository is one for which a non-maintainer community member can provide contributions and expect a maintainer to review and potentially incorporate. These repositories should be explicitly listed in the contribution documents as "open to volunteers."

### Contributing guidelines should be standardized across supervised repositories

https://github.com/opentofu/org/issues/25

A casual contributor should, at a glance, be able to understand all expectations and limitations for their contributions. This includes, but is not limited to, our stance on AI and the scope of prohibition on external code.

Note that none of those guidelines are specific to any development. We may have some development guidelines which are in a supervised repository's `CONTRIBUTING.md`. However, if those were moved to their own document, then `CONTRIBUTING.md` could be standardized and mostly identical across all supervised repositories.

This also means that [any fixes](https://github.com/opentofu/org/issues/26) done to some central `CONTRIBUTING.md` and `MAINTAINERS.md` documents in `org` could be referred to across all relevant repositories.

We may also want to list different ways to contribute that [may not include code](https://opensource.guide/how-to-contribute/#what-it-means-to-contribute), or code contributions ancillary to the main repository (like CI/CD).

### Add separate development guidelines in every supervised repository

https://github.com/opentofu/org/issues/24

Each `CONTRIBUTING.md` might refer to its repository's `DEVELOPMENT.md` regarding how to get a program running locally and start making contributions, as well as pointers to architecture and other relevant developer documentation. These guides should be available on any supervised repository. For non-code repositories, such as brand artifacts, it can include media standards and relevant trademark information.

### Labels and issue templates should be standardized across supervised repositories, with some exceptions

https://github.com/opentofu/org/issues/28

https://github.com/opentofu/org/issues/29

For the benefit of casual contributors, a regular list of labels with standard meanings in every repository will benefit any potential contributor in finding issues, or determining which issues to avoid. By standardizing labels, we can also recommend a filtered view of issues for each supervised repository. By complement, we can also create another filtered list of issues that are still being sorted out or issues that should only be worked on by maintainers.

Similarly, we should review issue templates across repositories and, wherever possible, ensure they are brief, comprehensible, and uncomplicated. Those issue templates should then be applied across all supervised repositories.

There are a few exceptional repositories regarding standard labels and issues. For example, the `bug` label or issue template may not be apropos to the website or brand artifacts.

Note: once labels are standardized, we can use their descriptions as a "source-of-truth" for their meaning, rather than duplicating information as we [currently do in the FAQ](https://github.com/opentofu/opentofu/blob/75bf1c2f65ad4baabd51a5e88873f805f5b2a1c7/contributing/FAQ.md#what-do-the-labels-meanhttps://github.com/opentofu/opentofu/blob/75bf1c2f65ad4baabd51a5e88873f805f5b2a1c7/contributing/FAQ.md#what-do-the-labels-mean) (see also [this issue](https://github.com/opentofu/opentofu/issues/3449) regarding other FAQ fixes)

## Long-term goals and strategy

As mentioned, this RFC acts as a method of improving communication with the community by referencing standardized contribution documents. There are many other avenues of communication, many of which have already been adopted (a Slack channel, working groups, blog posts) and some which may work in the larger outreach strategy but are unknown or not yet scalable. There's also a constant effort to determine what the structure of the community and its relationships looks like, what it could look like, and how to best serve that community. In the future, we may open up more varied roles to the community that are currently being managed by maintainers, such as making social media posts, managing issue triage, or performing legal review. These are admittedly high-trust roles, but specializing the labor involved in the OpenTofu project may be key for the next level of community engagement. In the pursuit of technical excellence, let us not forget the inherently social nature of open source.

#### Appendix: Forked repositories

The only issue from the tracker not yet mentioned in this RFC regards [forked repositories](https://github.com/opentofu/org/issues/27), which is barely related to community outreach. It is included here solely for the sake of completeness.

OpenTofu fork many repositories, but we don't have a present long-term strategy to keep these forks up-to-date. Is this a chore we should do? Are there automated tools we could use that might ease the process? What are the criteria for choosing which repositories to fork and which to contribute upstream?
