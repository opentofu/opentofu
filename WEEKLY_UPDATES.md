# Weekly Updates

## 2024-06-14

Hello! It's been a busy month since the last "Weekly Update" and there is a lot to cover!

- [OpenTofu 1.7.2](https://github.com/opentofu/opentofu/releases/tag/v1.7.2) was released!
  - This release contained a few bugfixes for new functionality introduced in the 1.7 release cycle
- Recent Developments:
  - A new [RFC Process](https://github.com/opentofu/opentofu/blob/main/rfc/20240524-OpenTofu-RFC-Process.md) has been adopted!
    - This will help define larger proposals and enable more in-depth converations.
    - Existing accepted RFCs have been backfilled to preserve history.
  - Static Evaluation work has begun!
    - [RFC was accepted!](https://github.com/opentofu/opentofu/pull/1649)
    - The [first PR](https://github.com/opentofu/opentofu/pull/1718) has been opened to implement the RFC
  - Performance improvements:
    - [Write state using compact JSON representation](https://github.com/opentofu/opentofu/pull/1647)
    - [Make persist interval for remote state backend configurable](https://github.com/opentofu/opentofu/pull/1591)
  - [Support for overrides](https://github.com/opentofu/opentofu/pull/1499) in `tofu test`
  - [Allow variable to pass inside `variables` block](https://github.com/opentofu/opentofu/pull/1488)
- Upcoming 1.8.0-alpha1, as early as next week!
  - Enables use of variables in backends and module sources!
  - New [.tofu file extension](https://github.com/opentofu/opentofu/pull/1699/files)!
  - Introduce `tofu {}` block as alternative to `terraform {}` and [change understanding of required_version](https://github.com/opentofu/opentofu/pull/1716)
  - Full list in the [CHANGELOG](https://github.com/opentofu/opentofu/blob/568ff66/CHANGELOG.md)
- How can I help?
  - Use OpenTofu, report issues, and please upvote the ones that are important to you! You can see an overall ranking in the [ranking issue](https://github.com/opentofu/opentofu/issues/1496).
  - We occasionally mark issues with the `help-wanted` label, so keep an eye out for them!

## 2024-05-17

Hello there! After the 1.7.1 release last week, the core team has been focusing on a few remaining issues for 1.7.2 and starting to look forward to 1.8!

- Current Status
  - [Init-time constant evaluation](https://github.com/opentofu/opentofu/issues/1042)
    - Initial [discussion and planning](https://github.com/opentofu/opentofu/pull/1649) are happening as an extension of the RFC proces
  - [Registry UI](https://github.com/opentofu/registry/issues/450)
    - Designs and prototyping are in progress!
  - [Releases page with direct links to artifacts](https://github.com/opentofu/get.opentofu.org/pull/25) is now live https://get.opentofu.org/tofu/api.json
  - A lot of smaller tickets are being worked on, you can take a look at the [1.7.2 and 1.8.0 milestones](https://github.com/opentofu/opentofu/milestones) for details.
    - [Better error messages in for_each blocks](https://github.com/opentofu/opentofu/pull/1485/) was approved and merged!
    - [Improved integration of JSON strings in generated configs](https://github.com/opentofu/opentofu/pull/1595) also made it in!
- Issue and RFC Workflow:
  - As this project matures, the core team has started looking more closely at the current issue and RFC process.
  - The core team will likely be proposing changes to this process based on contributor feedback and rough edges we've encountered.
  - Once the proposal is ready, we'd love any past, current, or prospective contributors to weigh in and share their feedback!
- How can I help?
  - Use OpenTofu, report issues, and please upvote the ones that are important to you! You can see an overall ranking in the [ranking issue](https://github.com/opentofu/opentofu/issues/1496).
  - We occasionally mark issues with the `help-wanted` label, so keep an eye out for them!

## 2024-05-10

Hello there! Again after a break in updates, since most weeks we've had preview or stable releases, which summed up most of what happened.

We haven't slowed down, and managed to get the stable release out according to schedule, before the end of April! Since then, we've got a few bug reports and already released a follow-up 1.7.1 release. Now, we're primarily working towards the 1.8 release.

- Current Status
  - State Encryption
    - We've fixed some issues as part of 1.7.1, and have [one more important usability issue](https://github.com/opentofu/opentofu/issues/1605) that we'd like to improve on in the not-too-distant future.
  - [Init-time constant evaluation](https://github.com/opentofu/opentofu/issues/1042)
    - Got accepted by the TSC and will be the flagship feature of 1.8.
    - We're currently planning and splitting up the work to be done.
    - We'll most likely deliver it in parts, with the foundation and first features in 1.8, and later more features built on top of it in 1.9.
  - [Registry UI](https://github.com/opentofu/registry/issues/450)
    - We've been asked about this a lot, and have now started working on it.
  - A lot of smaller tickets are being worked on, you can take a look at the [1.7.2 and 1.8.0 milestones](https://github.com/opentofu/opentofu/milestones) for details.
  - Additionally, we're working on improved developer documentation on how to author end-to-end tests in OpenTofu as we hope to increase the end-to-end test coverage. We'll be opening a bunch of related issues, most of which should be open to external contribution! 
- How can I help?
  - Use OpenTofu, report issues, and please upvote the ones that are important to you! You can see an overall ranking in the [ranking issue](https://github.com/opentofu/opentofu/issues/1496).
  - We occasionally mark issues with the `help-wanted` label, so keep an eye out for them!

## 2024-04-17

Hey there! First, apologies for the lack of updates over the last two weeks, we've been a bit busy with the [Cease and Desist Letter we got](https://opentofu.org/blog/our-response-to-hashicorps-cease-and-desist/). That's fortunately all sorted now, and we're fully back to engineering work this week! All of this delayed 1.7 by around a week, but this delay also gave us some additional time to add cool unique features to provider-defined functions in OpenTofu!

Importantly, we're planning to get a 1.7.0-beta1 release out tomorrow!

Additionally, we'll have a [livestream to showcase some cool stuff](https://youtube.com/live/6OXBv0MYalY) next week on Wednesday!

- Current Status:
  - More work on Provider-defined Functions has been happening. Importantly, we have a [bunch of OpenTofu-exclusive improvements](https://github.com/opentofu/opentofu/pull/1491) that will allow us to create providers that will let you have e.g. Lua files with custom functions side-by-side with your OpenTofu configuration. We will have a [livestream on Wednesday next week](https://youtube.com/live/6OXBv0MYalY) to showcase some of this functionality!
  - The final [PR functionality-wise for for_each in import blocks](https://github.com/opentofu/opentofu/pull/1492) has now been merged. There's still some cleanup left, but the hard part is done!
  - We've [improved the migration flow to encrypted state-files](https://github.com/opentofu/opentofu/pull/1458) that are coming in 1.7. It's now safer and easier to understand. This feature is of course - and will be - optional.
  - Work is happening on multiple `tofu test`-related issues, though it might not make it's way into 1.7.
  - We've added a [public ranking of issues by vote count](https://github.com/opentofu/opentofu/issues/1496), updated daily. This way, issues important for the community will be clearly visible!
- How can I help?
  - Please test the 1.7.0-beta1 release once it's out and [give us feedback](https://github.com/opentofu/opentofu/issues/new?assignees=&labels=preview-release-feedback&projects=&template=1_7_0_alpha1_feedback.yml)! This is right now the highest priority item you can help with.

## 2024-03-27

Hello there!  We've had a fairly quiet couple days after KubeCon last week, but still managed to make progress in some key areas!

- Current Status:
  - Large numbers are no longer truncated in plans [#1382](https://github.com/opentofu/opentofu/pull/1382)
  - Debugging crashes is now much easer with enhanced stack traces [#1425](https://github.com/opentofu/opentofu/pull/1425)
  - State Encryption
    - Integration testing with TACOS identified some issues that have been fixed with remote and cloud backends [#1431](https://github.com/opentofu/opentofu/pull/1431)
    - Dumping state during a crash is more resilient [#1421](https://github.com/opentofu/opentofu/pull/1421)
    - Configuration is undergoing some polish
    - Additional key providers in flight
  - Work has been started on Provider Functions [#1326](https://github.com/opentofu/opentofu/issues/1326)
  - Work continues on import blocks with for_each support.
- How can I help?
  - Please test the 1.7.0-alpha1 release and [give us feedback](https://github.com/opentofu/opentofu/issues/new?assignees=&labels=preview-release-feedback&projects=&template=1_7_0_alpha1_feedback.yml)! This is right now the highest priority item you can help with.

## 2024-03-21

Hey there! We released [OpenTofu 1.7.0-alpha1](https://github.com/opentofu/opentofu/releases/tag/v1.7.0-alpha1) end of last week and most of the team spent the week at OpenTofu Day Europe and KubeCon. You can find the videos [on YouTube](https://www.youtube.com/watch?v=kudru6Rm1n0&list=PLnVotLM2Qsyiw_6Pd_9WxRRLdrUAs3c1c). Attendance was high, 150+ people (more than available seats).

Here's what we worked on:

- Current Status and Up Next
  - We released a [blog post](https://opentofu.org/blog/help-us-test-opentofu-1-7-0-alpha1/) and a [video](https://www.youtube.com/watch?v=36FN-DzHz6s) asking the community to test the 1.7.0-alpha1 release. 
  - State encryption
    - Compliance tests for all KMS and method implementations, as well as individual README files in the encryption package to ease future development ([#1377](https://github.com/opentofu/opentofu/pull/1377),[#1396](https://github.com/opentofu/opentofu/pull/1396))  
    - Released the first version with PBKDF2 and AWS KMS (1.7.0-alpha1)
    - Merged the GCP KMS ([#1392](https://github.com/opentofu/opentofu/pull/1392))
    - More work being done on Azure and OpenBao integrations
  - Working on making the website friendlier to newcomers to OpenTofu without previous Terraform experience
- How can I help?
  - Please test the 1.7.0-alpha1 release and [give us feedback](https://github.com/opentofu/opentofu/issues/new?assignees=&labels=preview-release-feedback&projects=&template=1_7_0_alpha1_feedback.yml)! This is right now the highest priority item you can help with.

## 2024-03-13

Hey there! The main goal this week is getting an alpha release of 1.7 out, and you can expect it by the end of the week. It will include state encryption and the new removed block as major features, so that you can take those features for a spin and give us feedback.

Additionally, next week is KubeCon, and as part of it, OpenTofu Day! If you're going to KubeCon, make sure to come and say hi!

- Current Status and Up Next
  - State encryption
    - PBKDF2 key provider has been finished and merged ([#1310](https://github.com/opentofu/opentofu/pull/1310))
    - We're working on compliance tests that will better verify whether all key providers behave correctly and have the same contract ([#1377](https://github.com/opentofu/opentofu/pull/1377))
    - The AWS KMS key provider is also being worked on, and needs a bit more fine-tuning based on the compliance tests ([#1349](https://github.com/opentofu/opentofu/pull/1349))
    - We've made some simplifications to the naming and structure of the encryption configuration ([#1385](https://github.com/opentofu/opentofu/issues/1385))
    - We've made some struct-embedding-related improvements to the gohcl package of the hcl library, and are trying to get them upstreamed (for now we've vendored a copy of the package) ([#667](https://github.com/hashicorp/hcl/pull/667))
  - We've added support for recursively calling the templatefile function ([#1250](https://github.com/opentofu/opentofu/pull/1250))
  - Work continues on import blocks with for_each support.
  - We're working on `tofu test`-related improvements for provider blocks.
  - We're preparing for OpenTofu Day, esp. Arel who together with James will be giving a presentation on the registry.
  - We've been doing some hardening on our registry (which has been working great so far) and after identifying features we need that aren't available in the pro plan, CloudFlare has agreed to sponsor us with a business plan. Thanks a lot to them!
- How can I help?
  - Once the alpha build is out, please test state encryption. Watch the [OpenTofu YouTube channel](https://youtube.com/@opentofu) for detailed instructions.
  - Use OpenTofu, report issues, and please upvote issues that are important to you, so we can better prioritize development work.

## 2024-03-07

Hello there! This week we got state encryption running on the main branch for the first time, and we are planning to release an alpha build next week with a passphrase key provider and possibly the AWS KMS integration.

- Current Status and Up Next
  - State encryption
    -  Several PRs have been merged ([#1316](https://github.com/opentofu/opentofu/pull/1316), [#1291](https://github.com/opentofu/opentofu/pull/1291), [#1295](https://github.com/opentofu/opentofu/pull/1295), [#1288](https://github.com/opentofu/opentofu/pull/1288), [#1292](https://github.com/opentofu/opentofu/pull/1292), [#1294](https://github.com/opentofu/opentofu/pull/1294))
  - Declarative remove block
    - Merged end-user documentation ([#1332](https://github.com/opentofu/opentofu/pull/1332))
  - Website & community
    - Added support for the urldecode() function in OpenTofu ([#1283](https://github.com/opentofu/opentofu/pull/1283)) 
    - Improved website accessibility ([#274](https://github.com/opentofu/opentofu.org/pull/274))
  - How can I help?
    - Once the alpha build is out, please test state encryption. Watch the [OpenTofu YouTube channel](https://youtube.com/@opentofu) for detailed instructions.
    - Use OpenTofu, report issues, and please upvote issues that are important to you, so we can better prioritize development work.

## 2024-02-22

Hello! Over the last week we've finished up a couple of features, and we're continuing to work our way towards 1.7.0.

We've also [just released 1.6.2](https://github.com/opentofu/opentofu/releases/tag/v1.6.2) with a fix for a bug in `tofu test`.

Additionally, if you're coming to KubeCon and will be there a day early, make sure to come to OpenTofu Day! It's happening on Tuesday, the 19th of March, as a KubeCon + CloudNativeCon Europe CNCF-hosted Co-located Event. You can find more details [here](https://events.linuxfoundation.org/kubecon-cloudnativecon-europe/co-located-events/opentofu-day/#about).

- Current Status and Up Next
  - State encryption
    - [First big pull request](https://github.com/opentofu/opentofu/pull/1227) has been merged.
    - The team is now working with much better parallelism on integrating it with the codebase, as well as implementing the various encryption methods and key providers.
  - Declarative remove block
    - The [PR](https://github.com/opentofu/opentofu/pull/1158) has been merged, and the feature is now finished. It will be available as part of 1.7.0.
    - We'll now work on the documentation for this feature.
  - [Passing outputs of one test case to another as variables](https://github.com/opentofu/opentofu/pull/1254) is now fixed and is available as part of 1.6.2 today.
  - Work on adding for_each to import blocks is being continued, [a draft PR is currently up](https://github.com/opentofu/opentofu/pull/1270) further refactoring the code.
- How can I help?
  - Use OpenTofu! Let us know about your experience, and if you run into any issues, please report them.
  - Other than that, the best way to help is to create issues, discuss on issues, and spread the word about OpenTofu.
  - There are some issues which are accepted and open to external contribution. [Please see the contributing guide for more details](https://github.com/opentofu/opentofu/blob/main/CONTRIBUTING.md).
    - You can look for `good-first-issue` and `help-wanted` labels to find issues we've deemed are the best to pick up for external contributors. They are generally picked up quickly, so there might not be any available when you look. Please take a look there periodically if you'd like to find an issue to contribute to.

Please let us know if you have any feedback on what we could improve, either with these updates or more generally - this very document was a result of such feedback! We're available on Slack, via GitHub issues, or even in the pull request updating this file.

## 2024-02-15

Hey there! Last week was quiet due to most of the core team is off on winter holidays, but this week we're all back!

- Current Status and Up Next
  - We've finalized most of our discussions about the initial state encryption design. [The PR will be merged soon](https://github.com/opentofu/opentofu/pull/1227).
  - The removed block project still [has a PR up](https://github.com/opentofu/opentofu/pull/1158), and is pending reviews. We're expecting to merge it soon.
  - Work on adding for_each to import blocks is being continued, no PR up right now, though the refactor mentioned last week has been merged.
  - We're refactoring our release workflow, so it properly handles future pre-release versions, with a stable release now in place. [A PR is open for this](https://github.com/opentofu/opentofu/pull/1235) and still being worked on.
  - We're working on a bug fix to passing outputs of one test case to another as variables. [See the PR here](https://github.com/opentofu/opentofu/pull/1254).
  - We're having a (positively!) surprisingly large amount of external contributors, so we're spending a nontrivial amount of time reviewing and helping contributors get their changes in. Thanks everybody!
  - We've updated our [contribution guide](https://github.com/opentofu/opentofu/blob/main/CONTRIBUTING.md) to better guide contributors and also clarify how we make decisions on feature requests.
- How can I help?
  - Use OpenTofu! Let us know about your experience, and if you run into any issues, please report them.
  - Other than that, the best way to help is to create issues, discuss on issues, and spread the word about OpenTofu.
  - There are some issues which are accepted and open to external contribution. [Please see the contributing guide for more details](https://github.com/opentofu/opentofu/blob/main/CONTRIBUTING.md).
    - You can look for `good-first-issue` and `help-wanted` labels to find issues we've deemed are the best to pick up for external contributors. They are generally picked up quickly, so there might not be any available when you look. Please take a look there periodically if you'd like to find an issue to contribute to.

Please let us know if you have any feedback on what we could improve, either with these updates or more generally - this very document was a result of such feedback! We're available on Slack, via GitHub issues, or even in the pull request updating this file.

## 2024-02-07

The last two weeks we've been focusing on working towards the 1.7.0 release, and on going through our issue backlog.

This weekend some of us have been to FOSDEM and had a stand there - it was amazing to hear in person how many people have already migrated to OpenTofu! We've also distributed a lot of OpenTofu t-shirts and stickers! 

Additionally, most of the core team is off on winter holidays this week, so it's fairly quiet.

- Current Status and Up Next
  - We've been iterating on the skeleton of the client-side state encryption project, in order to get that right. [A PR for this has been opened recently and is pending reviews](https://github.com/opentofu/opentofu/pull/1227).
  - The removed block project also [has a PR up](https://github.com/opentofu/opentofu/pull/1158), and is pending reviews.
  - As part of adding for_each to import blocks, we've refactored the import code to better handle the differences between the block implementation and the cli command implementation. [This is posted as a PR](https://github.com/opentofu/opentofu/pull/1207), reviewed, and will be merged soon.
  - We're refactoring our release workflow, so it properly handles future pre-release versions, with a stable release now in place. [A PR is open for this](https://github.com/opentofu/opentofu/pull/1235) and still being worked on.
  - We've managed to go through the whole issue backlog and have responded and/or handled most of them.
- How can I help?
  - Use OpenTofu! Let us know about your experience, and if you run into any issues, please report them.
  - Other than that, the best way to help is to create issues, discuss on issues, and spread the word about OpenTofu.
  - There are some issues which are accepted and open to external contribution. [Please see the contributing guide for more details](https://github.com/opentofu/opentofu/blob/main/CONTRIBUTING.md).
    - You can look for `good-first-issue` and `help-wanted` labels to find issues we've deemed are the best to pick up for external contributors. They are generally picked up quickly, so there might not be any available when you look. Please take a look there periodically if you'd like to find an issue to contribute to.

Please let us know if you have any feedback on what we could improve, either with these updates or more generally - this very document was a result of such feedback! We're available on Slack, via GitHub issues, or even in the pull request updating this file.

## 2024-01-23

This week we've been focusing on work related to the 1.6.1 release, the 1.7.0 release, and on going through our issue backlog.

On an unrelated note, we'll have a stand at FOSDEM and will have OpenTofu t-shirts and stickers, so if you're there, come say hi!

- Current Status and Up Next
  - The client-side state encryption project is moving forward, and [the first PR](https://github.com/opentofu/opentofu/pull/1063), laying down the groundwork, is almost ready to be merged. It's been going through extensive review, and it's the thing that's blocking us from better parallelising the work on this project. Once that is merged, [the tracking issue](https://github.com/opentofu/opentofu/issues/1030) now contains a list of subtasks that we'll be working on next in this project.
  - We're working our way towards the 1.6.1 release, which we hope to get out this week, ideally tomorrow.
    - We now support [passing variables between run blocks](https://github.com/opentofu/opentofu/pull/1129),
    - and fixed a bug that's prohibiting [using the `tofu show` command on a legacy state file](https://github.com/opentofu/opentofu/issues/1139).
    - We're fixing [some edge-cases with running tofu on 32-bit architectures](https://github.com/opentofu/opentofu/issues/1069), mostly relating to unit tests. This is currently a blocker for the inclusion of OpenTofu in the Alpine package repository.
  - Other than the client-side state encryption project, other 1.7 work is ongoing as well, with
    - a draft PR already being open and worked on to support [the new `removed` block](https://github.com/opentofu/opentofu/pull/1158),
    - and work being done on the [support for `for_each` in the `import` block](https://github.com/opentofu/opentofu/issues/1033).
  - We've been focusing on working down the issue backlog. We've already had two 1h+ meetings since last week, and are planning to continue this twice-a-week cadence until we work the backlog down. We're giving each issue a great deal of thought, so we expect this to take another few weeks.
    - Expect more accepted, responded-to, or closed issues.
    - We've also introduced a bunch of labels to better signal the status of each issue.
- How can I help?
  - Use OpenTofu! Let us know about your experience, and if you run into any issues, please report them.
  - Other than that, the best way to help is to create issues, discuss on issues, and spread the word about OpenTofu.
  - There are some issues which are accepted and open to external contribution. [Please see the contributing guide for more details](https://github.com/opentofu/opentofu/blob/main/CONTRIBUTING.md).
    - You can look for `good-first-issue` and `help-wanted` labels to find issues we've deemed are the best to pick up for external contributors. They are generally picked up quickly, so there might not be any available when you look. Please take a look there periodically if you'd like to find an issue to contribute to.

Please let us know if you have any feedback on what we could improve, either with these updates or more generally - this very document was a result of such feedback! We're available on Slack, via GitHub issues, or even in the pull request updating this file.


## 2024-01-16

Hey there! Long time no see in these weekly updates! We've skipped them during the holidays and the release week, but now we'll be back to the regular cadence.

Much has happened over the last few weeks, with the biggest news being the [OpenTofu Stable 1.6 Release](https://github.com/opentofu/opentofu/releases/tag/v1.6.0) last week!

Since the stable release, we've been working to get 1.6.1 and 1.7.0 into your hands. We've also seen immense community interest, with members of the community contributing great changes, and others integrating it into third party tooling. This is all amazing, and we really appreciate it!

- Current Status and Up Next
  - The top priority is making sure the client-side encryption project is moving forward. This is a topic where the core team (owned by @janosdebugs) is collaborating with @StephanHCB, and we hope to have the first PR in this week. This PR will contain the "skeleton" of the solution, and let us start working on integrations (encryption methods) with more parallelism. Track the issue [here](https://github.com/opentofu/opentofu/issues/1030).
  - Second-biggest priority is the 1.6.1 release. There's a [GitHub Milestone](https://github.com/opentofu/opentofu/milestone/4) for it, and we're aiming to have it out next week, ideally.
    - Here we're done with [interpolating locals into import blocks](https://github.com/opentofu/opentofu/issues/1084),
    - and also [performance improvements for provider acceptance tests](https://github.com/opentofu/opentofu/issues/1044).
    - Pending is support for [passing variables between test run blocks](https://github.com/opentofu/opentofu/issues/1045).
  - The current plan for the 1.7.0 release is to have it out whenever we're done with the client-side state encryption. Thus, expect a release February-March.
  - We've accepted numerous community contributions, you can check [the 1.7 changelog](https://github.com/opentofu/opentofu/blob/3b4069e697259021b97a11fc9263e4316ea1b8c4/CHANGELOG.md) for details.
  - We'll be focusing on working down the issue backlog this and next week. Expect more accepted or responded-to issues. We'll also be introducing a bunch of labels to better signal the status of each issue.
- How can I help?
  - Use OpenTofu! Let us know about your experience, and if you run into any issues, please report them.
  - Other than that, the best way to help is to create issues, discuss on issues, and spread the word about OpenTofu.
  - There are some issues which are accepted and open to external contribution. [Please see the contributing guide for more details](https://github.com/opentofu/opentofu/blob/main/CONTRIBUTING.md).
    - You can look for `good-first-issue` and `help-wanted` labels to find issues we've deemed are the best to pick up for external contributors. They are generally picked up quickly, so there might not be any available when you look. Please take a look there periodically if you'd like to find an issue to contribute to.

Please let us know if you have any feedback on what we could improve, either with these updates or more generally - this very document was a result of such feedback! We're available on Slack, via GitHub issues, or even in the pull request updating this file.

## 2023-12-14

Hey there!

This week's been mostly about delivering that last 1% of work before we can publish a release candidate. Other than that, we've also started planning work for past the stable release, so targeting the 1.7 release.

The steering committee has also gathered (you can find the details [here](https://github.com/opentofu/opentofu/blob/main/TSC_SUMMARY.md#2023-12-11)) and planned the official date for the stable release to be the 10th of January.

- Current Status and Up Next
  - We've added GPG key signing to Debian and RPM archives that we're distributing in upcoming releases (thanks to BuildKite for sponsoring PackageCloud!)
  - We've worked on changing the default provider namespace to `opentofu`, but ended up rolling that back, and (softly) deprecating the whole concept of namespace-less provider references altogether, due to the edge-cases involved.
  - The registry now has an improved development workflow with a proper dev environment. It's also seen big runtime optimizations so refreshing providers and modules is now ~50% faster.
  - We're working on the final documentation improvements before the stable release.
  - We're working on an improved convenience script that will support all available installation methods.
  - We'll soon be starting work on 1.7 features.
- How can I help?
  - Since the beta release is out, right now the best way to help is to take the beta for a test drive and see if there are any bugs / issues.
    - Most importantly, as mentioned above, please try running `tofu init` with your projects to double-check everything you use is available in the registry.
  - **With the release candidate around the corner, it's the perfect time to start implementing OpenTofu support in 3rd party tooling!**
  - Other than that, the best way to help is to create issues, discuss on issues, and spread the word about OpenTofu.
  - There are some occasional minor issues which are accepted and open to external contribution, esp. ones outside the release-blocking path. [Please see the contributing guide for more details](https://github.com/opentofu/opentofu/blob/main/CONTRIBUTING.md).
    - You can look for a label, `good-to-pick-up`, to find issues we've deemed are the best to pick up for external contributors.
  - We have multiple engineers available full-time in the core team, so we’re generally trying to own any issues that are release blockers - this way we can make sure we get to the release as soon as possible.
  - The amount of pending-decision-labeled issues on the repository might be a bit off-putting. The reason for that is that right now we’re prioritizing the stable release. Only after we have a stable release in place do we aim to start actually accepting enhancement proposals and getting them implemented/merged. Still, we encourage you to open those issues and discuss them!
    - Issues and Pull Requests with enhancements or major changes will generally be frozen until we have the first stable release out. We will introduce a milestone to mark them as such more clearly.

Please let us know if you have any feedback on what we could improve, either with these updates or more generally - this very document was a result of such feedback! We're available on Slack, via GitHub issues, or even in the pull request updating this file.

## 2023-12-05

Hey there, time for a weekly update!

There was no update last week, but the important bit was that we've released the *first beta release*!

Going into more details:

- Current Status and Up Next
  - Registry
    - The [stable registry implementation](https://github.com/opentofu/registry) is now the active one on `registry.opentofu.org`. It's receiving meaningful usage and working very well.
    - The experience of using CloudFlare R2 with Caching for this has been excellent. It's snappy for both cached and non-cached responses, with a very good cache hit-ratio, that's bound to improve too.
    - **You can help make this more stable!** If you'd like to help us ensure a good experience right from the stable release, please try running `tofu init` on your codebases, and make sure that all providers and modules you're using are available. We did our best to make it a drop-in replacement, but we might've missed a provider or two. We're constantly monitoring 404 responses on our side and will back-fill anything that's missing.
    - We're mostly working on improvements to the flow of contributing new providers and modules here, to make sure you can easily add any new ones you make. The milestone tracking the stable release requirements is [here](https://github.com/opentofu/registry/milestone/2).
  - Tofu Migration
    - We're making sure to polish the migration path to OpenTofu so that community members don't hit any issues on the way. Primarily this involves documentation around the installation and migration process.
  - [There was a bug in the global schema provider cache handling](https://github.com/opentofu/opentofu/issues/929) that we've introduced a while ago, and now fixed. It was causing slowdowns when using older versions of providers. The fix was part of the 1.6.0-beta2 release.
- How can I help?
  - Since the beta release is out, right now the best way to help is to take the beta for a test drive and see if there are any bugs / issues.
    - Most importantly, as mentioned above, please try running `tofu init` with your projects to double-check everything you use is available in the registry.
  - Other than that, the best way to help is to create issues, discuss on issues, and spread the word about OpenTofu.
  - There are some occasional minor issues which are accepted and open to external contribution, esp. ones outside the release-blocking path. We’re also happy to accept any minor refactors or linter fixes. [Please see the contributing guide for more details](https://github.com/opentofu/opentofu/blob/main/CONTRIBUTING.md).
    - You can look for a label, `good-to-pick-up`, to find issues we've deemed are the best to pick up for external contributors (there are none available at this moment).
  - We have multiple engineers available full-time in the core team, so we’re generally trying to own any issues that are release blockers - this way we can make sure we get to the release as soon as possible.
  - The amount of pending-decision-labeled issues on the repository might be a bit off-putting. The reason for that is that right now we’re prioritizing the stable release. Only after we have a stable release in place do we aim to start actually accepting enhancement proposals and getting them implemented/merged. Still, we encourage you to open those issues and discuss them!
    - Issues and Pull Requests with enhancements or major changes will generally be frozen until we have the first stable release out. We will introduce a milestone to mark them as such more clearly.

Please let us know if you have any feedback on what we could improve, either with these updates or more generally - this very document was a result of such feedback! We're available on Slack, via GitHub issues, or even in the pull request updating this file.

## 2023-11-21

Hello!

Another week another update! This week has been very steady. We've been moving forward with the stable registry, and there's been no surprises so far.

Additionally, a new engineer has joined the OpenTofu Core Team and will be 100% dedicated to OpenTofu development. Welcome, @janosdebugs!

- Current Status and Up Next
  - Registry
    - The [stable registry implementation](https://github.com/opentofu/registry-stable) is evolving and mostly working end-to-end.
      - You can find the detailed design documents [here](https://github.com/opentofu/opentofu/issues?q=is%3Aopen+is%3Aissue+label%3Arfc+label%3Af-registry), if you'd like to read into the details.
    - We'll be using CloudFlare R2 to host the registry (our repository emits the whole registry as a set of static files). This week we've migrated our opentofu.org DNS zone to CloudFlare, which is a prerequisite for using R2 with the domain. Thanks to CloudFlare for sponsoring a pro plan!
    - We're productionizing the code, making sure it's stable, and will start hydrating the registry soon.
  - As usual, a bunch of various bug-fixes.
  - The installation docs have been drastically improved, and we're in the process of writing docs for the testing feature.
- How can I help?
  - Since the alpha release is out, right now the best way to help is to take the alpha for a test drive and see if there are any bugs / issues.
  - Other than that, the best way to help is to create issues, discuss on issues, and spread the word about OpenTofu.
  - There are some occasional minor issues which are accepted and open to external contribution, esp. ones outside the release-blocking path. We’re also happy to accept any minor refactors or linter fixes. [Please see the contributing guide for more details](https://github.com/opentofu/opentofu/blob/main/CONTRIBUTING.md).
    - You can look for a label, `good-to-pick-up`, to find issues we've deemed are the best to pick up for external contributors (there are none available at this moment).
  - We have multiple engineers available full-time in the core team, so we’re generally trying to own any issues that are release blockers - this way we can make sure we get to the release as soon as possible.
  - The amount of pending-decision-labeled issues on the repository might be a bit off-putting. The reason for that is that right now we’re prioritizing the alpha and stable release. Only after we have a stable release in place do we aim to start actually accepting enhancement proposals and getting them implemented/merged. Still, we encourage you to open those issues and discuss them!
    - Issues and Pull Requests with enhancements or major changes will generally be frozen until we have the first stable release out. We will introduce a milestone to mark them as such more clearly.

Please let us know if you have any feedback on what we could improve, either with these updates or more generally - this very document was a result of such feedback! We're available on Slack, via GitHub issues, or even in the pull request updating this file.

## 2023-11-15

Hey there!

This last week has seen work on multiple avenues. More concretely, we've been steadily working our way down the stable release milestone. With the current pace, we expect the stable release to be out between mid-December and mid-January, preceded by a release candidate and a beta release.

- Current Status
  - Registry
    - Multiple detailed design documents have been written this week, and there are two more to go. [You can track them here](https://github.com/opentofu/opentofu/issues?q=is%3Aopen+is%3Aissue+label%3Arfc+label%3Af-registry).
    - There is now [a new repository](https://github.com/opentofu/registry-stable) where we're working on the upcoming stable registry. It's not accepting contributions right now and the code is messy, but you can take a look there if you'd like to track progress. We're working on having an end-to-end working version as soon as possible (which will unblock a beta release).
  - We've introduced apt and yum repositories for OpenTofu via PackageCloud (thanks to BuildKite for sponsoring it!). See more in the [installation instructions](https://opentofu.org/docs/intro/install).
  - Multiple bugs have been fixed or are being worked on, related to the S3 state backend, variable validation blocks, refinements, repeat executions of tofu init, and expired gpg signing keys.
- Up Next
  - Working on the registry with as much developer-parallelism as possible at any given time.
  - Documentation for the testing feature.
  - Getting the e2e test suite for state backends running.
- How can I help?
  - Since the alpha release is out, right now the best way to help is to take the alpha for a test drive and see if there are any bugs / issues.
  - Other than that, the best way to help is to create issues, discuss on issues, and spread the word about OpenTofu.
  - There are some occasional minor issues which are accepted and open to external contribution, esp. ones outside the release-blocking path. We’re also happy to accept any minor refactors or linter fixes. [Please see the contributing guide for more details](https://github.com/opentofu/opentofu/blob/main/CONTRIBUTING.md).
    - **New!** We've introduced a new label, `good-to-pick-up`, that should help you find issues we've deemed are the best to pick up for external contributors. However, all such issues have promptly been picked up, so there aren't any available at this time.
  - We have multiple engineers available full-time in the core team, so we’re generally trying to own any issues that are release blockers - this way we can make sure we get to the release as soon as possible.
  - The amount of pending-decision-labeled issues on the repository might be a bit off-putting. The reason for that is that right now we’re prioritizing the alpha and stable release. Only after we have a stable release in place do we aim to start actually accepting enhancement proposals and getting them implemented/merged. Still, we encourage you to open those issues and discuss them!
    - Issues and Pull Requests with enhancements or major changes will generally be frozen until we have the first stable release out. We will introduce a milestone to mark them as such more clearly.

Please let us know if you have any feedback on what we could improve, either with these updates or more generally - this very document was a result of such feedback! We're available on Slack, via GitHub issues, or even in the pull request updating this file.

## 2023-11-07

Hey!

Big week for OpenTofu! The technical steering committee met and chose a registry design. We'll now be writing more detailed designs and then proceed with implementing the registry.

Moreover, two engineers have joined the OpenTofu Core Team and will be 100% dedicated to OpenTofu development. Welcome, @kislerdm and @cam72cam!

- Current Status and Up Next
  - The technical steering committee has gathered last week, and you can find the meeting notes in the [TSC_SUMMARY.md](TSC_SUMMARY.md) file in the repository root.
  - The technical steering committee has chosen the [Homebrew-like design](https://github.com/opentofu/opentofu/issues/741) for implementation.
    - Now we'll have multiple designs for subcomponents of the registry, [you can track them here](https://github.com/opentofu/opentofu/issues?q=is%3Aopen+is%3Aissue+label%3Arfc+label%3Af-registry), via GitHub search.
    - Once we're all on the same page design-wise, we'll start implementation. There is already a [fully-working PoC of the base Homebrew-like design](https://github.com/opentofu/opentofu/issues/741#issuecomment-1786697966).
  - There are some minor bugs that have been added as stable release blockers. The new core team joiners will focus on them as part of their onboarding process.
    - You can track the progress towards the [stable release milestone here](https://github.com/opentofu/opentofu/milestone/3).
- How can I help?
  - Since the alpha release is out, right now the best way to help is to take the alpha for a test drive and see if there are any bugs / issues.
  - Other than that, the best way to help is to create issues, discuss on issues, and spread the word about OpenTofu.
  - There are some occasional minor issues which are accepted and open to external contribution, esp. ones outside the release-blocking path. We’re also happy to accept any minor refactors or linter fixes. [Please see the contributing guide for more details](https://github.com/opentofu/opentofu/blob/main/CONTRIBUTING.md).
  - We have multiple engineers available full-time in the core team, so we’re generally trying to own any issues that are release blockers - this way we can make sure we get to the release as soon as possible.
  - The amount of pending-decision-labeled issues on the repository might be a bit off-putting. The reason for that is that right now we’re prioritizing the alpha and stable release. Only after we have a stable release in place do we aim to start actually accepting enhancement proposals and getting them implemented/merged. Still, we encourage you to open those issues and discuss them!
    - Issues and Pull Requests with enhancements or major changes will generally be frozen until we have the first stable release out. We will introduce a milestone to mark them as such more clearly.

Please let us know if you have any feedback on what we could improve, either with these updates or more generally - this very document was a result of such feedback! We're available on Slack, via GitHub issues, or even in the pull request updating this file.

## 2023-11-02

Hey!

This week was still mostly spent on discussing and implementing proof of concepts for some stable registry designs, as well as work on the S3 backend.

- Current Status
  - OpenTofu Day will be happening as a KubeCon + CloudNativeCon Europe CNCF-hosted Co-located Event, learn more [here](https://events.linuxfoundation.org/kubecon-cloudnativecon-europe/co-located-events/opentofu-day/#registration-details)!
  - The technical steering committee is meeting today (2023-11-02) to pick a stable registry design.
    - There are [three registry RFCs right now](https://github.com/opentofu/opentofu/issues?q=is%3Aopen+is%3Aissue+label%3Arfc+label%3Af-registry)
  - [Extensive work is happening](https://github.com/opentofu/opentofu/issues/700) around updating the S3 state backend configuration to be in line with the AWS provider configuration. Only two pull requests left here and we'll be done.
  - [A fix](https://github.com/opentofu/opentofu/pull/773) for the [interesting bug report](https://github.com/opentofu/opentofu/issues/763) mentioned last week in the update has been finished and merged.
- Up next
  - The technical steering committee will pick a stable registry design later today, and the core engineering team will focus most of its time on that in the upcoming weeks (2023-11-06 and onwards).
- How can I help?
  - Since the alpha release is out, right now the best way to help is to take the alpha for a test drive and see if there are any bugs / issues.
  - Other than that, the best way to help is to create issues, discuss on issues, and spread the word about OpenTofu.
  - There are some occasional minor issues which are accepted and open to external contribution, esp. ones outside the release-blocking path. We’re also happy to accept any minor refactors or linter fixes. [Please see the contributing guide for more details](https://github.com/opentofu/opentofu/blob/main/CONTRIBUTING.md).
  - We have multiple engineers available full-time in the core team, so we’re generally trying to own any issues that are release blockers - this way we can make sure we get to the release as soon as possible.
  - The amount of pending-decision-labeled issues on the repository might be a bit off-putting. The reason for that is that right now we’re prioritizing the alpha and stable release. Only after we have a stable release in place do we aim to start actually accepting enhancement proposals and getting them implemented/merged. Still, we encourage you to open those issues and discuss them!
    - Issues and Pull Requests with enhancements or major changes will generally be frozen until we have the first stable release out. We will introduce a milestone to mark them as such more clearly.

Please let us know if you have any feedback on what we could improve, either with these updates or more generally - this very document was a result of such feedback! We're available on Slack, via GitHub issues, or even in the pull request updating this file.

## 2023-10-24

Hey!

This week was again spent on discussing and PoC'in stable registry designs, as well as work on the S3 backend.

- Current Status
  - There are [two registry RFCs right now](https://github.com/opentofu/opentofu/issues?q=is%3Aopen+is%3Aissue+label%3Arfc+label%3Af-registry), and both will most likely have a PoC by the end of the week.
  - [Extensive work is happening](https://github.com/opentofu/opentofu/issues/700) around updating the S3 state backend configuration to be in line with the AWS provider configuration.
  - OpenTofu is available on Homebrew Core, so you can just `brew install opentofu` to use it!
  - A [bug report](https://github.com/opentofu/opentofu/issues/763) has surfaced an interesting issue when migrating from Terraform to OpenTofu with an existing state file. An initial fix [has already been implemented](https://github.com/opentofu/opentofu/pull/773) and will be further polished and released soon.
- Up next
  - The steering committee will pick a registry RFC in the week of 30th Oct - 3rd Nov.
    - Intense implementation work will start on the 6th of November.
    - This should let us have a stable registry in place around mid-November - mid-December.
- How can I help?
  - Since the alpha release is out, right now the best way to help is to take the alpha for a test drive and see if there are any bugs / issues.
  - You can also submit and discuss RFCs related to the stable registry.
  - Other than that, the best way to help is to create issues, discuss on issues, and spread the word about OpenTofu.
  - There are some occasional minor issues which are accepted and open to external contribution, esp. ones outside the release-blocking path. We’re also happy to accept any minor refactors or linter fixes. [Please see the contributing guide for more details](https://github.com/opentofu/opentofu/blob/main/CONTRIBUTING.md).
  - We have multiple engineers available full-time in the core team, so we’re generally trying to own any issues that are release blockers - this way we can make sure we get to the release as soon as possible.
  - The amount of pending-decision-labeled issues on the repository might be a bit off-putting. The reason for that is that right now we’re prioritizing the alpha and stable release. Only after we have a stable release in place do we aim to start actually accepting enhancement proposals and getting them implemented/merged. Still, we encourage you to open those issues and discuss them!
    - Issues and Pull Requests with enhancements or major changes will generally be frozen until we have the first stable release out. We will introduce a milestone to mark them as such more clearly.

Please let us know if you have any feedback on what we could improve, either with these updates or more generally - this very document was a result of such feedback! We're available on Slack, via GitHub issues, or even in the pull request updating this file.

## 2023-10-17

Hello there!

This past week was mostly spent on creating stable registry designs, which is a prerequisite to building a stable provider/module registry and releasing a stable version of OpenTofu.

- Current Status
  - The [registry replacement issue](https://github.com/opentofu/opentofu/issues/258) has now been updated to include a list of stable registry requirements.
    - There is now time until the 27th October to submit and discuss RFCs.
    - You can find relevant RFCs by using [GitHub Issue search](https://github.com/opentofu/opentofu/issues?q=is%3Aopen+is%3Aissue+label%3Arfc+label%3Af-registry).
    - After the 27th October the Technical Steering Committee will pick a single RFC which will then be implemented.
  - A [setup-opentofu](https://github.com/opentofu/setup-opentofu) GitHub Action is now available, it's a great way to take OpenTofu for a spin on GitHub Actions. Thanks a lot to @kislerdm for contributing this!
  - Work on TF 1.6 compatibility has been done.
  - We've closed a lot of issues that were stable release blockers, you can take a look at the current stable milestone status [here](https://github.com/opentofu/opentofu/milestone/3).
- Up next
  - The main TF 1.6 compatibility issue left, on which work has already been started and will continue for a while is the new S3 state backend configuration schema. You can track this issue [here](https://github.com/opentofu/opentofu/issues/700).
  - Until the 27th October deadline, work and discussion on RFCs will happen, as well as the implementation of PoCs for some of these RFCs.
- How can I help?
  - Since the alpha release is out, right now the best way to help is to take the alpha for a test drive and see if there are any bugs / issues.
  - You can also submit and discuss RFCs related to the stable registry.
  - Other than that, the best way to help is to create issues, discuss on issues, and spread the word about OpenTofu.
  - There are some occasional minor issues which are accepted and open to external contribution, esp. ones outside the release-blocking path. We’re also happy to accept any minor refactors or linter fixes. [Please see the contributing guide for more details](https://github.com/opentofu/opentofu/blob/main/CONTRIBUTING.md).
  - We have multiple engineers available full-time in the core team, so we’re generally trying to own any issues that are release blockers - this way we can make sure we get to the release as soon as possible.
  - The amount of pending-decision-labeled issues on the repository might be a bit off-putting. The reason for that is that right now we’re prioritizing the alpha and stable release. Only after we have a stable release in place do we aim to start actually accepting enhancement proposals and getting them implemented/merged. Still, we encourage you to open those issues and discuss them!
    - Issues and Pull Requests with enhancements or major changes will generally be frozen until we have the first stable release out. We will introduce a milestone to mark them as such more clearly.

Please let us know if you have any feedback on what we could improve, either with these updates or more generally - this very document was a result of such feedback! We're available on Slack, via GitHub issues, or even in the pull request updating this file.

## 2023-10-11

Hey!

Last week we've released the [first alpha release](https://github.com/opentofu/opentofu/releases/tag/v1.6.0-alpha1) (and [another one](https://github.com/opentofu/opentofu/releases/tag/v1.6.0-alpha2) the next day). This has been a huge milestone for the OpenTofu community, and means that you can actually download OpenTofu and play with it. It also marks the beginning of the stable registry design discussions.

Just to give you some metrics about the release:
- The alpha releases of OpenTofu have over 2k downloads.
- Some registry stats
  - Uptime: 100% 
  - Max RPS: 105 
  - Total Requests: 78869 
  - Average latency: 19.15ms 
  - Total unique providers used: 194

We'll also have [our first office hours](https://opentofu.org/blog/opentofus-new-office-hours) today! Make sure you don't miss that.

- Current Status
  - First Alpha Release
    - We've managed to get the alpha release out, including [a working registry](https://github.com/opentofu/registry).
  - Second Alpha Release
    - A [bug was reported](https://github.com/opentofu/opentofu/issues/655) to OpenTofu and later also to Terraform. We've deemed it high priority and possibly impacting the ability of the community to test OpenTofu with existing codebases, so we've worked on fixing it asap. The root cause was in the [hcl library](https://github.com/hashicorp/hcl/issues/629). We've [made a fix](https://github.com/opentofu/opentofu/pull/659) in OpenTofu, released a new alpha, and afterward [the fix got accepted to the hcl library](https://github.com/hashicorp/hcl/pull/630).
- Up next
  - Stable Registry Design (see [the tracking issue](https://github.com/opentofu/opentofu/issues/258))
    - Even though there's a registry included with the alpha release, it's not stable. It was mostly a "get it working" design, and we will now be working on a stable design. 
    - Right now we're working on a list of requirements, which will most likely be shared tomorrow in the tracking issue above.
    - Then, there will be an approximate 2 weeks to submit and discuss RFCs, after which the steering committee will choose the final design, which we'll begin implementing right away.
    - Please discuss in the issue and incoming RFCs, we want to hear all your voices! Please also feel free to submit your own RFCs.
- How can I help?
  - Since the alpha release is out, right now the best way to help is to take the alpha for a test drive and see if there are any bugs / issues.
  - Other than that, the best way to help is to create issues, discuss on issues, and spread the word about OpenTofu.
  - There are some occasional minor issues which are accepted and open to external contribution, esp. ones outside the release-blocking path. We’re also happy to accept any minor refactors or linter fixes. [Please see the contributing guide for more details](https://github.com/opentofu/opentofu/blob/main/CONTRIBUTING.md).
  - We have multiple engineers available full-time in the core team, so we’re generally trying to own any issues that are release blockers - this way we can make sure we get to the release as soon as possible.
  - The amount of pending-decision-labeled issues on the repository might be a bit off-putting. The reason for that is that right now we’re prioritizing the alpha and stable release. Only after we have a stable release in place do we aim to start actually accepting enhancement proposals and getting them implemented/merged. Still, we encourage you to open those issues and discuss them!
    - Issues and Pull Requests with enhancements or major changes will generally be frozen until we have the first stable release out. We will introduce a milestone to mark them as such more clearly.

Please let us know if you have any feedback on what we could improve, either with these updates or more generally - this very document was a result of such feedback! We're available on Slack, via GitHub issues, or even in the pull request updating this file.

## 2023-10-02

Hey!

Another week, another update! This last week has been all about the alpha release, which is planned to be released this week, ideally tomorrow (Tuesday).

- Current Status
  - Alpha release is blocked only by the initial registry work
    - [There's a PR open](https://github.com/opentofu/opentofu/pull/379) for using the OpenTofu registry as the default in OpenTofu.
    - There's been extensive work on caching, discussed in [a GitHub issue](https://github.com/opentofu/registry/issues/97) and [a Slack channel](https://opentofucommunity.slack.com/archives/C05UH44L449).
      - [One PR](https://github.com/opentofu/registry/pull/103) introduced a better version listing cache in DynamoDB, to reuse the cache across provider listing and provider downloading.
      - [Another PR](https://github.com/opentofu/registry/pull/105) introduced http-level caching for all requests to GitHub, which generally avoids making the same request twice in quick succession.
    - There's a bug in the provider mirroring scripts which makes them not properly trigger GitHub Actions. We're working on a fix.
    - [There's a bug](https://github.com/opentofu/registry/issues/121) with module downloading, for which both tags in the form of `x.y.z` and `vx.y.z` are supported, while we only support one of them.
- Up next
  - The alpha release is planned for this week, using our alpha registry.
  - As described last week, once the alpha release is out, we'll be kicking off discussions about the stable registry design.
    - One such issue [has already been opened](https://github.com/opentofu/opentofu/issues/619) and explores the problem space of handling signatures and public keys.
- How can I help?
  - Right now the best way to help is to create issues, discuss on issues, and spread the word about OpenTofu. Additionally, the alpha release is coming out this week. We'd appreciate you taking it for a test drive and letting us know about any issues you encounter.
  - There are some occasional minor issues which are accepted and open to external contribution, esp. ones outside the release-blocking path. We’re also happy to accept any minor refactors or linter fixes. [Please see the contributing guide for more details](https://github.com/opentofu/opentofu/blob/main/CONTRIBUTING.md).
  - We have multiple engineers available full-time in the core team, so we’re generally trying to own any issues that are release blockers - this way we can make sure we get to the release as soon as possible.
  - The amount of pending-decision-labeled issues on the repository might be a bit off-putting. The reason for that is that right now we’re prioritizing the alpha and stable release. Only after we have a stable release in place do we aim to start actually accepting enhancement proposals and getting them implemented/merged. Still, we encourage you to open those issues and discuss them!
    - Issues and Pull Requests with enhancements or major changes will generally be frozen until we have the first stable release out. We will introduce a milestone to mark them as such more clearly.

Please let us know if you have any feedback on what we could improve, either with these updates or more generally! We're available on Slack, via GitHub issues, or even in the pull request creating this very file.

## 2023-09-26

Hello!

We’ve received some feedback that it’s currently hard to track the status of OpenTofu and what is currently being worked on. We hear you, and we’ll do better. We’re slowly moving discussions from our internal slack to public GitHub issues, to make them more inclusive. In addition, we’ll now be publishing an update like this once a week to give you a breakdown of where we’re at, what we’re currently working on, how you can best help, and what’s up next.

- Current Status
  - Right now we’re primarily working towards the first alpha release. There are two remaining blockers here:
    - Finishing the renaming to OpenTofu. There's only [a single PR](https://github.com/opentofu/opentofu/pull/576) left here.
    - Preparing the alpha registry replacement to be ready for use. The main current issue here is [#97](https://github.com/opentofu/registry/issues/97), described in a bit more detail below.
  - The [current registry design](https://github.com/opentofu/registry) is a glorified GitHub redirector.
    - This is by no means the final design! It’s meant to be a “get it working” design for the alpha release. Soon (~once the alpha release is out) we’ll start public discussions regarding the “stable” design for the registry, and we’re not ruling out any options here yet.
    - Right now the main issue is caching. Since the registry is a GitHub redirector, it is affected by GitHub rate limits. The rate limit is 5k requests per hour. We already do extensive caching here, but we can still do better and want to do better prior to the alpha release. [This issue contains more details](https://github.com/opentofu/registry/issues/97) about the work being done here.
    - Also, even though OpenTofu alpha will [skip signature validation](https://github.com/opentofu/opentofu/issues/266) **when the key is not available**, we’re collecting provider signing keys for the registry to serve. So, if you’re a provider author, please make sure to create a Pull Request to the registry repo and add your public key. [Here's the latest example of such a PR](https://github.com/opentofu/registry/pull/95).
    - We’ve also made a mirror of all official HashiCorp providers so that they’re hosted on the OpenTofu GitHub, like any other 3rd-party provider. This included getting all historic versions to build (which was quite a challenge!) but we’re done with it and the end result is that out of 2260 provider versions we only have 16 failures, which are generally old and broken versions, so you should be able to easily work around this.
      - The reason for this is that the registry being a GitHub redirector needs all artifacts on GitHub, while HashiCorp hosts these providers directly on their registry and doesn't include them in GitHub releases.
      - Long-term we’re planning to host these most-used providers on Fastly.
- Up next
  - Early next week we’re planning to make available an alpha release of OpenTofu, with a fully drop-in working registry for providers and modules.
  - With the above, we’ll also kick off the discussion around the stable registry.
    - We’ll publish an initial list of requirements for the registry in the [original registry issue](https://github.com/opentofu/opentofu/issues/258). The list itself will be open to changes based on further discussion.
    - Based on those requirements, we will be creating RFC’s for possible solutions. If you’d like to propose a solution, feel free to post an RFC too!
    - After having discussions there and possibly doing some PoC’s, the technical steering committee will make the final call which approach to pursue.
- How can I help?
  - Right now the best way to help is to create issues, discuss on issues, and spread the word about OpenTofu.
  - There are some occasional minor issues which are accepted and open to external contribution, esp. ones outside the release-blocking path. We’re also happy to accept any minor refactors or linter fixes. [Please see the contributing guide for more details](https://github.com/opentofu/opentofu/blob/main/CONTRIBUTING.md).
  - We have multiple engineers available full-time in the core team, so we’re generally trying to own any issues that are release blockers - this way we can make sure we get to the release as soon as possible.
  - The amount of pending-decision-labeled issues on the repository might be a bit off-putting. The reason for that is that right now we’re prioritizing the alpha and stable release. Only after we have a stable release in place do we aim to start actually accepting enhancement proposals and getting them implemented/merged. Still, we encourage you to open those issues and discuss them!
    - Issues and Pull Requests with enhancements or major changes will generally be frozen until we have the first stable release out. We will introduce a milestone to mark them as such more clearly.
  - Once we make the alpha release available next week, the best way to help will be by test-driving that release and creating GitHub issues for any problems you find.

Please let us know if you have any feedback on what we could improve, either with these updates or more generally! We're available on Slack, via GitHub issues, or even in the pull request creating this very file.
