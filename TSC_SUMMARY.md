# Technical Steering Committee (TSC) Summary  

The Technical Steering Committee is a group comprised of people from companies and projects backing OpenTofu. Its purpose is to have the final decision on any technical matters concerning the OpenTofu project, providing a model of governance that benefits the community as a whole, as opposed to being guided by any single company. The current members of the steering committee are:

- Igor Savchenko ([@DiscyDel](https://github.com/DicsyDel)) representing Scalr Inc.
- Roger Simms ([@allofthesepeople](https://github.com/allofthesepeople)) representing Harness Inc.
- Roni Frantchi ([@roni-frantchi](https://github.com/roni-frantchi)) representing env0
- Wojciech Barczynski ([@wojciech12](https://github.com/wojciech12)) representing Spacelift Inc.
- Zach Goldberg ([@ZachGoldberg](https://github.com/ZachGoldberg)) representing Gruntwork, Inc.

## 2024-12-10

### Attendees

- Christan Mesh ([@cam72cam](https://github.com/cam72cam)) (OpenTofu Tech Lead)
- Roger Simms ([@allofthesepeople](https://github.com/allofthesepeople))
- Igor Savchenko ([@DiscyDel](https://github.com/DicsyDel))
- Wojciech Barczynski ([@wojciech12](https://github.com/wojciech12))
- Zach Goldberg ([@ZachGoldberg](https://github.com/ZachGoldberg))

### Agenda

Internal housekeeping around hiring and marketing. No voting.

## 2024-11-26

### Attendees

- Christan Mesh ([@cam72cam](https://github.com/cam72cam)) (OpenTofu Tech Lead)
- Roger Simms ([@allofthesepeople](https://github.com/allofthesepeople))
- Igor Savchenko ([@DiscyDel](https://github.com/DicsyDel))
- Roni Frantchi ([@roni-frantchi](https://github.com/roni-frantchi))
- Wojciech Barczynski ([@wojciech12](https://github.com/wojciech12))
- Zach Goldberg ([@ZachGoldberg](https://github.com/ZachGoldberg))

### Agenda

- Internal housekeeping around hiring and marketing. No voting.
- Shall we bring the stack concept to OpenTofu?

### Discussion:
- Shall we bring the stack concept to OpenTofu?
https://github.com/opentofu/opentofu/issues/931
https://github.com/gruntwork-io/terragrunt/issues/3313#issuecomment-2469025204 
Action Item: observe the issues ^
Breaking down the state is a real issue for our users.

## 2024-11-19

### Attendees

- Christan Mesh ([@cam72cam](https://github.com/cam72cam)) (OpenTofu Tech Lead)
- Roger Simms ([@allofthesepeople](https://github.com/allofthesepeople))
- Igor Savchenko ([@DiscyDel](https://github.com/DicsyDel))
- Wojciech Barczynski ([@wojciech12](https://github.com/wojciech12))
- Zach Goldberg ([@ZachGoldberg](https://github.com/ZachGoldberg))

### Agenda

Internal housekeeping around hiring and marketing. No voting.

## 2024-10-22

### Attendees

- Christan Mesh ([@cam72cam](https://github.com/cam72cam)) (OpenTofu Tech Lead)
- Roger Simms ([@allofthesepeople](https://github.com/allofthesepeople))
- Oleksandr Levchenkov ([@ollevche](https://github.com/ollevche))
- Wojciech Barczynski ([@wojciech12](https://github.com/wojciech12))
- Zach Goldberg ([@ZachGoldberg](https://github.com/ZachGoldberg))

### Agenda

No concerns expressed, we publish the Registry Policy. You will find it in the registry github repository: https://github.com/opentofu/registry.

## 2024-10-15

### Attendees

- Christan Mesh ([@cam72cam](https://github.com/cam72cam)) (OpenTofu Tech Lead)
- Roger Simms ([@allofthesepeople](https://github.com/allofthesepeople))
- Oleksandr Levchenkov ([@ollevche](https://github.com/ollevche))
- Wojciech Barczynski ([@wojciech12](https://github.com/wojciech12))
- Yousif Akbar ([@yhakbar](https://github.com/yhakbar)) (On behalf of Zach Goldberg)

### Agenda

#### Registry Policy

Vote - as the topic is important for the project, we opt for a unanimous vote. We will ask Env0 to voice their concerns. If needed, discuss the next meeting. But we will vote to move the topic forward. The deadline for expressing the is 22 October/the next TSC meeting:

- Policy: https://github.com/opentofu/registry/blob/main/POLICY.md (to be published on 23 October).
- An initial vote was taken to escalate to the requirement of a unanimous vote, which passed.
- Assuming we have unanimous consent (no pushback by the deadline), we will move forward with publishing the current policy as written on 2024/10/15 by the next TSC meeting.
- The Tech Lead will respond to questions on the policy for existing issues/PRs and not take any action other than referring to the policy. We will need time to review actions already taken and potential future actions. Any significant communication will be cleared in the tsc+core Slack channel.
- Vote: all TSC attending the meeting voted YES.

### Registry Updates

The core team reached out to the TSC for feedback/opinion on how to update the provider/module registry. Should we discover new providers (Github scrapping), rely on the community or authors to submit them, or take more balanced action? It is a mid priority currently.

The core team considers the following options with regular cadence:

- Option A:
    - Add the providers’ metadata to our registry
    - Optionally contact the maintainers to see if they could submit a GPG key
- Option B:
    - Contact maintainers of popular provider authors to add their provider and key to the registry.
    - Could be limited to large clouds/services to reduce the scope
    - There is a higher likelihood of getting more keys in the registry
- Option C:
    - Don’t do anything and expect users to submit providers they need or bug the provider authors themselves.

The TSC recommends a split between Options B and C (especially for less popular providers/modules when polled earlier this week. Having a template for reaching out to the providers/module authors could help us communicate consistently.

#### Discussion

Discussion:

- YousifA: for B, it would be good to have a template (polite and with instructions) that we can use to reach module/provider authors. C → everything else.
- ChristianM: I like to use stars/forks to see what providers/modules are popular
- RogerS: B, Can we get the same results from the community? The community could create a list of popular modules/providers missing from the OpenTofu registry. We would contact the authors.
- WojciechB: For B, I would prefer that the core team contact the authors of popular modules/providers instead of the community members.
- OleksandrC: C, we can use it as an opportunity to build relationships with large companies/collaborations/blogposts (see IntelliJ).
- Christian: this task has medium priority.

### Migration from Notion to Google Docs

It will be on hold till Igor Savchenko ([@DiscyDel](https://github.com/DicsyDel)) is back from holidays.

## 2024-10-09

### Attendees

- Christan Mesh ([@cam72cam](https://github.com/cam72cam)) (OpenTofu Tech Lead)
- Roger Simms ([@allofthesepeople](https://github.com/allofthesepeople))
- Oleksandr Levchenkov ([@ollevche](https://github.com/ollevche))
- Wojciech Barczynski ([@wojciech12](https://github.com/wojciech12))
- Yousif Akbar ([@yhakbar](https://github.com/yhakbar)) (On behalf of Zach Goldberg)

### Agenda

### Getting Better About Communicating TSC Meetings

The committee discussed the need to improve communication about TSC meetings and the decisions made during them.

Going forward, a new process will be implemented to ensure that the community is kept informed about the TSC's activities:

- At the beginning of each meeting, the TSC will review an asynchronously authored summary of the previous meeting to ensure
that it accurately reflects the decisions made, and the thought process behind decisions is communicated effectively.
- Within 24 hours of the meeting, the TSC will publish the summary of the previous meeting to the OpenTofu community.
- Before the next meeting, someone from the TSC will volunteer to author the summary of the current meeting, and the process will
repeat.

The objective of this process is to ensure that the community is kept informed about the TSC's activities and decisions in a timely manner.

### Registry Policy

The committee continued discussions regarding this, and progress was made towards determining the best way to handle the policy.

The discussions included considerations like the following:

- How broad or specific the policy should be.
- How to handle the policy's enforcement.
- What the impact of the policy will be on the community.

## 2024-10-01

### Attendees

- Christan Mesh ([@cam72cam](https://github.com/cam72cam)) (OpenTofu Tech Lead)
- Roger Simms ([@allofthesepeople](https://github.com/allofthesepeople))
- Igor Savchenko ([@DiscyDel](https://github.com/DicsyDel))
- Yousif Akbar ([@yhakbar](https://github.com/yhakbar)) (On behalf of Zach Goldberg)

### Agenda

#### Static Evaluation Sensitivity Bug

- Christian: I'm working on a draft to report a security issue with static evaluation of variables.
    - It can lead to variables marked sensitive being exposed, due to the fact that static
      evaluation of sensitive variables in module sources, versions, etc might
      result in sensitive values being written to disk.
    - What is the best way to tackle breaking this behavior? Should it be removed in a patch release?
- Igor: This is an issue, but breaking behavior in a patch release is not ideal. 
    - It might be best to fix it in a minor release.
    - There's risk that some users consider a breaking change like this really surprising.
- Yousif: I agree with Igor. The behavior should be addressed in a minor release.
    - In the interim, would it be possible to emit a warning when users are using sensitive variables in contexts
      that might expose them?
    - Users could then be made aware of the issue and take steps to mitigate it before the fix is released.
    - We could also consider adding a flag to opt-in to allowing sensitive variables in these contexts.
- Christian: I'll look into adding a warning, but I'm not sure there's a sensible reason to use sensitive variables in these contexts.
- Igor: Many community members asked for this functionality to be able to include tokens for fetching private modules.
    - They'll rely on the ability to use sensitive variables in contexts where they might be exposed in `.terraform.lock.hcl` files.
- Christian: That's a good point. Users might need a mechanism to opt-in to existing behavior.
   - I'll report this issue, then communicate the plan to address it with a warning in a patch, and fix it in a minor release.

#### OpenTofu Registry Policy

This topic is complex, and the committee is working to finalize a policy that will be acceptable to all parties.

To avoid harassment of any committee members, the comments made by individual members will not be attributed to them in the minutes.

It was discussed that the policy should be clear on what the OpenTofu Steering Committee must do by law,
and how much flexibility the committee has in making decisions.

The committee agreed to revisit the topic in the following meeting.

## 2024-08-20

### Attendees

- Christan Mesh ([@cam72cam](https://github.com/cam72cam)) (OpenTofu Tech Lead)
- Igor Savchenko ([@DiscyDel](https://github.com/DicsyDel))
- Roger Simms ([@allofthesepeople](https://github.com/allofthesepeople))
- Wojciech Barczynski ([@wojciech12](https://github.com/wojciech12))
- Zach Goldberg ([@ZachGoldberg](https://github.com/ZachGoldberg))

### Agenda

#### Shall we stop using Notion?

- Igor: do we need notion?
- Zach: reluctantly second motion, easy to do stuff not in public.  Pushes us toward public on github.
- Igor: migrate this to private space, keep private/public for sensitive information
- Igor: we probably have a week or two.  Worst case pay for a month and then migrate.

##### Decision

Vote: unanimous yes

#### Sanctions Russia vs registry access

- Add note to README during PR documenting this discussion in TSC_SUMMARY
- Block Russian IP Blocks from accessing our registry in Cloudflare

##### Decision

Vote: unanimous yes

#### PackageCloud 

PackageCloud provides free deb/rpm hosting for OpenTofu.

- We said we would do a case study
- They are asking to be listed as sponsors
- Igor: Ok with updating sponsors (cloudflare as well)

##### Decision

Christian: will write up a "case study" ([examples](https://buildkite.com/case-studies)) and post for TSC review.

#### State Backend Improvements

- Continuing the discussion from 2024-07-24, we need to start planning how we want to support new backends, community backends, and modifications to backends
- Christian: gave overview of backend related issues in OpenTofu and their relative :+1: counterparts
   - Backends as Plugins (32 :+1:)
   - Modifications of existing backends (72 :+1:)
   - Support for new backends (40 :+1:)
- Community is already working around this with the HTTP backend and helper binaries (20+ easily found in github search)
- Potential Paths:
   - Improve http backend or create httpng or similar with workspace support
   - Create GRPC protocol and extend existing registry (Backends as Plugins)
   - Document and recommend remote or cloud backend
- Christian: Proposal
   - Stage 1: Improve HTTP Backend and provide library + compliance tests to community to foster adoption
   - Stage 2: Support backend plugins using above protocol (HTTP or gRPC, library makes it trivial for authors)
   - Stage 3: Migrate internal backends to plugin
- Christian: Proposes that we start with Stage 1 as a low-risk evaluation of the concept. Re-evaluate Stage 2/3 based on Stage 1 feedback and adoption.
- Igor: remote/cloud protocol not within our control. How simple can we make the http protocol?
- Wojciech: Likes the staged plan, hedges risk. Also likes proper support for the http backend and thinks TACOS will migrate.
- Igor: HTTP works through proxies, which may be a big advantage
- Zach: Fan of the PoC to demonstrate value, don’t know until you built it

##### Decision:

**Christian will prepare RFC for Stage 1 and send to TSC + Community via GitHub PR**

## 2024-08-13

### Attendees

- Christan Mesh ([@cam72cam](https://github.com/cam72cam)) (OpenTofu Tech Lead)
- Igor Savchenko ([@DiscyDel](https://github.com/DicsyDel)) 
- Roger Simms ([@allofthesepeople](https://github.com/allofthesepeople))
- Roni Frantchi ([@roni-frantchi](https://github.com/roni-frantchi))
- Wojciech Barczynski ([@wojciech12](https://github.com/wojciech12))
- Zach Goldberg ([@ZachGoldberg](https://github.com/ZachGoldberg))

### Agenda

#### Open Governance

- Christian: Where do we publish our governance documents?
- Christian: Do we need to amend our governance documents before publishing them?
- Have we defined how we manage TSC membership?
- Igor: Usually in a github repo w/ amendments and meeting notes
    - Also look at other CNCF projects
    - LF to review before publishing
    - TSC meeting to make amendments
- Christian: side conversations with OpenBao, potentially de-duplicating effort
- Roni: document existing process
    - first draft posted in internal chat
    - meeting after define how/where to publish
- Igor: envoy gov doc
- **Zach: Gruntworks employee to make first draft** for further iteration (a good example – https://github.com/envoyproxy/envoy/blob/main/GOVERNANCE.md)

#### Initial Conversation on Guides

We have users asking for getting started guides ([Issue #1838](https://github.com/opentofu/opentofu/issues/1838), and others).  They don’t want to switch between the Terraform and OpenTofu docs.  We have also had at least one company ask if we are interested in documentation services, though that conversation is premature.

We need to:

- Define if/where we want to start by including guides in OpenTofu

  - We could start with a similar layout to terraform, just with less examples.
  - Alternatively, we could come up with a layout that makes sense to us.

- Determine who should be in charge of creating and maintaining them
   - First few could be created by the core team
   - Continued by teams at the tacos with existing experience?
   - Continued by external contractor/company?
   - Continued by community?  Hard to tell if GPT/ripped from elsewhere.

##### Discussion

- Igor: what’s the worst case scenario here? Try to involve community if possible

- **Roger: volunteer Harness teams**
- **Core team will setup the layout in the next few weeks when capacity allows**
- Roni: team owns documentation, many tutorials & guides will come from the community
  - TACOS already market new features and create guides
  - Reach out to existing tf courses to mention/use OpenTofu

- Who should reach out to existing tf courses?
  - **Roni: delegate to Arel / core team, well known to the community**
  - Worked well with other integrations (jetbrains for example)
  - Env0 marketing may also reach out

#### Open Source ARD how to choose between TF and OpenTofu

Additional community question: Is there a copy and pastable open source ADR on Decide on HCL tool with pros and cons of each opentofu vs terraform that can be sold internally to an organization?

- Wojciech: could be a good blog post
- Christian: we have a year of experience and can talk about our strengths
- Wojciech: re-use spacelift articles
- Roger: emphasize the longevity of the project
- Zach: lots of FUD (providers) + CDKTF
- **Wojciech: Will create initial draft with SpaceLift and bring to TSC**

## 2024-07-30

### Attendees

- Christan Mesh ([@cam72cam](https://github.com/cam72cam)) (OpenTofu Tech Lead)
- Igor Savchenko ([@DiscyDel](https://github.com/DicsyDel)) 
- Roger Simms ([@allofthesepeople](https://github.com/allofthesepeople))
- Wojciech Barczynski ([@wojciech12](https://github.com/wojciech12))
- Zach Goldberg ([@ZachGoldberg](https://github.com/ZachGoldberg))

### Agenda

#### TSC meeting schedule

Move to Tuesday 13:15 EST / 19:15 CET / 20:15 GMT+3

#### CDK-TF Compatibility Commitment

[Issue #1335](https://github.com/opentofu/opentofu/issues/1335)

As OpenTofu matures, many people who use existing tooling want to make sure that a switch will not break their workflow (now or in the future).
We are being asked what our long term approach to CDK-TF is.

Options:

- Do not make any statement or commitment
- Say that we will attempt to keep compatibility where possible but not make any commitment
- Keep compatibility where possible and commit to a fork if the projects need to diverge

This is one of several pieces of tooling where we are being asked questions about stability and long term commitments, with [setup-opentofu](https://github.com/opentofu/setup-opentofu/) being another frequently requested item.  In that case we have people offering to help support it once we commit to hosting it / managing the review/release process.

#### Discussion

- Christian – a lot of questions about CDK-TF, what is our policy on the tools built on top of Terraform. Shall we fork?
- Igor – what is the license of CDKTF?
- Christian: MPL
- Igor – we do not have time and resources to fork the CDKTF
- Igor – we could ask the community for help
- Wojciech – would love to see it a community project first and bring it later. It has a lot of potential.
- Christian – Accept community PRs and issues to keep OpenTofu compatible with CDKTF but no commitment to a fork.
- Igor - setup-opentofu helps drive adoption, we should evaluate making it a core team priority (small codebase / time investment)

#### Decision

The core team has not had bandwidth to take on CDKTF, but will accept community support efforts.

## 2024-07-24

### Attendees

- Christan Mesh ([@cam72cam](https://github.com/cam72cam)) (OpenTofu Tech Lead)
- Igor Savchenko ([@DiscyDel](https://github.com/DicsyDel)) 
- Roger Simms ([@allofthesepeople](https://github.com/allofthesepeople))
- Wojciech Barczynski ([@wojciech12](https://github.com/wojciech12))
- Zach Goldberg ([@ZachGoldberg](https://github.com/ZachGoldberg))

### Agenda

As has been discussed in past TSC meetings, backends are an important and sensitive part of OpenTofu. We have been hesitant to make any significant changes to backends for a variety of reasons, compatibility being the major reason.

#### Backends

Problems we face:

- Backends use libraries which are out of date or that are heavily dependent on HC
    - Azure backend is a piece of work…
    - We inherit any S3 backend bugs from HC’s libraries, which we are hesitant to patch
- Backends would benefit from restructuring
    - HTTP backend should support workspaces (frequently requested)
    - Azure backend needs to support auth from this decade
- New Backends are requested often
    - Support for additional clouds (Oracle being the largest)
    - Workaround is http (no workspaces, custom service required) or s3 (no locking / buggy)
    - Adding backends into opentofu bloats our test matrix and increases our maintenance burden
- Remote and Cloud “backends” are not properly documented or maintained
    - Some TACOS support a subset of cloud/remote features
    - Is dependent on HC’s go-tfe library, built for their cloud offering
    - Code is a copy-paste nightmare
    - We don’t know how to maintain this properly from our end
- Backends are tied to specific tofu version
    - Any bugfixes or workarounds for a given cloud/service must be rolled into an opentofu release.

Potential Solution: Backend Plugins

Advantages:

- Existing backends could be moved into their own repo and be versioned as 1.0
    - Maps 1-1 with existing configurations, requiring no changes to users
    - Potentially simpler collaboration with AWS/Azure/GCP/etc…
- New backends can be authored by independent teams (Oracle, etc…) with minimal involvement of core team
- Users can specify when they want to upgrade their backend configurations/services separate from tofu
- Encapsulates backends behind a simple API for compliance testing
- Allows forks to be maintained by others without having to fork all of opentofu
- Reduces tofu’s primary code and binary size (less spidering dependencies)
- Prototyped by Marcin
- Potential to collaborate with HC on a set of shared backends?

Disadvantages:

- Requires significant developer time / testing
- How to handle protocol upgrades
- How to handle validation/verification of downloaded backend during `tofu init`

Potential Solution: Extend http backend to support workspaces (or make new version)

Advantages:

- Simple HTTP API to implement
- Simple Authentication mechanism
- Has some ecosystem adoption already

Disadvantages:

- Restricted to HTTP transport layer (potential problems with large state files)
- Requires dedicated service backed by some other storage layer / abstraction (leaky)
- Complex auth (cloud environment vars / service tokens) are difficult to support properly

Potential Solution: Accept new backends and start to make large changes to existing backends

Advantages:

- Works with code / process we already have
- Requires closer collaboration with other organizations (Oracle)

Disadvantages:

- Requires documenting backend changes heavily in every OpenTofu release and forcing migrations
- Adds direct dependencies on external teams / cloud infrastructure
- Dramatically increases testing matrix / testing load
- Contributes to code / binary bloat

At this point we need a decision from the TSC on which paths we should be investigating and creating detailed RFCs for.

###### Discussion

- Roni: What are the upvotes for fixes modifications vs new backends?

- Igor: Cautious about end user requests (tech debt / architecture concerns).  Prioritize healthy code base alongside new features.

- Zach: Wants an overview of our top feature requests in something like a ghant chart to help understand impact of large projects like this

- Wojciech: Likes idea of accepting new backends, with codeowners from top clouds, long term extract protocol?

- Igor: Opposite view, oracle specifically we can just say No with clear explanation.  Limited developer capacity.  Users won’t know if it’s a “tofu” or “oracle” problem.  Initial blame will be on OpenTofu. Potential for conflicts in what opentofu wants for state management vs what oracle wishes

- Christian: I think it’s not a significant technical challenge to define and lock down the interface

- Roni: Many not bring a lot of value to our customers and users.  Potentially not worth the investment for backends as plugins?

- Christian: Potentially invest in http backend further to make that our initial “backend interface” with examples.

- Wojciech: Likes the idea of http backend being the interface

- Roni: Env0 built on top of remote backend protocol, was not terribly difficult.  Kuba suggested introducing “one new backend” which is or “official” solution (OCI perhaps).  Strongly prefers remote interface over pluggable backends.

- Christian: How does auth work for remote and cloud / our OCI solution.

- Igor: This is a reasonably solved problem that solves most scenarios

- Roni: remote backend is quite popular and very functional at Env0.

- Igor: Scalr forces remote/cloud backend. Migration from tfe is currently tricky, needs a taco.  TACOS + tofu could together build migration path away from tfe.

- Roni: Migration from cloud/remote tfe block to whatever tofu’s preferred solution.  Could cloud/remote be the preferred solution?  Focus on backend migration to tofu as a smooth transition.

- Igor: Scalr has significant test harness against cloud/remote backend.  Offers help from devs in Scalr.

- Roni: At least one core team member has experience on Env0’s remote backend

- Christian: Core team can take this discussion and produce comparisons between options discussed here and breakdown of issue voting. Try to have this prepared for a week or two from now.

- James: Compare to features we already done for comparison

##### Recommendation

- Preferable one officially supported backend with all necessary functionality, for example, http backend.
- Continue discussion in the core team and TSC.

## 2024-07-10

### Attendees

- Christan Mesh ([@cam72cam](https://github.com/cam72cam)) (OpenTofu Tech Lead)
- Igor Savchenko ([@DiscyDel](https://github.com/DicsyDel)) 
- Roger Simms ([@allofthesepeople](https://github.com/allofthesepeople))
- Wojciech Barczynski ([@wojciech12](https://github.com/wojciech12))
- Zach Goldberg ([@ZachGoldberg](https://github.com/ZachGoldberg))

### Agenda

We dedicated this meeting to agree on the initial plan for 1.9. Christian – OpenTofu Tech Lead – presented the plan to the TSC.

#### OpenTofu 1.9 Planning

##### Summary

Although a long list, the corresponding terraform 1.9 changes consist of a lot of small tasks (as well as quite a few items catching up to OpenTofu). This leaves us a fair amount of room to innovate in ways that will drive adoption. I'd like to specifically focus on provider for_each iteration (top issue in both tofu and terraform), as well as paying down technical debt related to testing. Additionally, I'd like to propose a stretch goal of adding the -exclude flag to tofu as it helps round off quite a few sharp UX edges.

###### Leftover from 1.7

- Tofu test changes, nearly ready to merge: https://github.com/opentofu/opentofu/issues/1185

###### Leftover from 1.8

- Bug in provider dev overrides, investigating at a low priority: https://github.com/opentofu/opentofu/issues/1715

###### Terraform 1.9

- Improved variable validation: small task, mostly testing, https://github.com/opentofu/opentofu/issues/1336
- Multiline console support: small task, already discussed by community, https://github.com/opentofu/opentofu/issues/1307
- Breaking change on providers in terraform test: unknown, https://github.com/hashicorp/terraform/issues/35160
- Bugfix sensitive templatefile: small task, https://github.com/hashicorp/terraform/issues/31119
- Improved version constraint calculations: small task, https://github.com/hashicorp/terraform/issues/33452
- Bugfix conflict between import and destroy: unknown, https://github.com/hashicorp/terraform/issues/35151
- Fix crash with `tofu providers mirror`: small task, https://github.com/hashicorp/terraform/issues/35318
- Fix conflict between create_before_destroy and -refresh=false, https://github.com/hashicorp/terraform/issues/35218

###### Proposed Goals:

- Implement provider iteration (for_each support): https://github.com/opentofu/opentofu/issues/300
    - Top requested feature from the community
    - Builds on top of static evaluation in 1.8
    - RFC has been reviewed and accepted: https://github.com/opentofu/opentofu/blob/main/rfc/20240513-static-evaluation-providers.md
    - Is a *big* migration incentive
- Improve test confidence and stability
    - Large quantity of testing exists in the codebase, of varying quality
    - We need to take time to understand it and plan improvements
    - Also need to define where and how we should be testing as a team going forward
    - Igor: what clean-up can we do to reduce complexity?
    - Roger: Potential for opt-in profiling
- Add `-exclude` flag for targeted plan/apply (stretch goal)
    - Currently in RFC process: https://github.com/opentofu/opentofu/pull/1717
    - Makes multi-stage applies much simpler
    - Allows selective excluding of portions of infrastructure without a massive -include list
    - Is a frequently requested feature.
    - Igor: How do we handle new features and experiments? Feature flag? Canary release?
    - Christian: To come back to this discussion in future meeting

###### Adjacent work (not locked to 1.9 milestone):

- Registry UI, good progress is being made, but it is a large undertaking
- Maintain official fork of HCL as HashiCorp ignores pull requests from our team.
    - There is a **strong indication** that we’re being ignored
    - Igor: Let’s upvote it to try to get it in
    - https://github.com/hashicorp/hcl/pull/676
- Registry should lock tags to a single commit to prevent supply chain attacks
- License check for mirrored providers (aws, gcp, etc...)
    - Not under CLA so lower risk
    - Janos: dual licensing new features
- Update [OpenTofu.org](http://opentofu.org/):
    - Update landing page to reflect state of project, proposals already in progress
    - Clean up sponsorships and create job postings page
    - Include official support contracts from TACOS and similar
        - Igor: Needs a real support offering, how do we maintain this list?
        - Roger: Potentially ranked by level of OpenTofu support
    - Create quick start guides, could be defined by core team and implemented by the community

#### Looking ahead to 1.10:

- Some interesting core changes in terraform that may be tricky to mirror (ephemeral values)
- Community Requests:
    - OCI Registry support https://github.com/opentofu/opentofu/issues/308
    - Conditional single instance resources: https://github.com/opentofu/opentofu/issues/1306
    - Backends as plugins *or* start to add new/updated backends
- Technical Debt:
    - Introduce internal concept of "immutable state" to allow refactoring and more efficient operation
    - Refactor and clean up command package
    - Supply chain (go.mod) review, with focus on removing hashicorp dependencies

#### Decision

High level approval of split between terraform mirroring features and future development.

Approval given to turn the 1.9 plan above into a public milestone and to start breaking down the issues involved.

## 2024-07-02

### Attendees

Core Team:

- Andrew Hayes ([@Andrew-Hayes](https://github.com/Andrew-Hayes))
- Christan Mesh ([@cam72cam](https://github.com/cam72cam))
- James Humphries ([@yantrio](https://github.com/Yantrio))
- Janos Bonic ([@janosdebugs](https://github.com/janosdebugs))

TSC:

- Igor Savchenko ([@DiscyDel](https://github.com/DicsyDel)) 
- Roger Simms ([@allofthesepeople](https://github.com/allofthesepeople))
- Roni Frantchi ([@roni-frantchi](https://github.com/roni-frantchi))
- Wojciech Barczynski ([@wojciech12](https://github.com/wojciech12))
- Zach Goldberg ([@ZachGoldberg](https://github.com/ZachGoldberg))

### Agenda

#### Summary of the last meeting

What is the current Vision of OpenTofu? Build the IaC tool of choice for the Community and Businesses that use and support it.

How do we follow our Vision?  We focus on **Adoption by winning hearts and minds**.

Who do we want to try to adopt OpenTofu?

- Large Organizations / Cloud Providers
    - Slow movement, but critical features can cause shifts (State Encryption)
    - Seeking stability / low risk
- Small Organizations / Startups
    - More agile / likely to give OpenTofu a try for smaller QoL improvements
- Individual Developers
    - New developers are looking for a strong community to participate in
    - Existing developers are trying to keep their existing patterns / infra working with as little friction as possible

How do we entice them to Adopt OpenTofu?

- Maintain compatibility with Terraform 1.5.5 as many organizations have frozen on this or lower version.
- Track developments in Terraform beyond 1.5.5 and prioritize new features when they make sense for OpenTofu.
- Continue to build new and exciting features that make OpenTofu stand out.
- Gather information from TACOs directly and combine with community issue voting to help shape roadmap.
- Testimonials / Logos for the website?

What is blocking them from Adopting OpenTofu?

- FUD: Fear, Uncertainty, Doubt
    - Lack of knowledge on provider compatibility
    - Lack of clear information on goals / roadmap
    - Unsure about the long term viability of OpenTofu
    - Lack of dedicated support contracts (TACO opportunity?)
- Cost of switching tooling / infrastructure

Do we want to prioritize different classes of potential OpenTofu adopters?  Do we want to focus on Top Down or Bottom Up adoption?

##### Discussion

- Website to address FUD / focus on tofu not manifesto
- Janos has been prototyping website changes / messaging
- Section for guides?  Do we want to host or link to external guides?
- Gold level sponsors / prioritize contributing sponsors.  Look at blender as a good example.
- Sponsored developers may be advocates for issues, as long as they are impartial.
- Keep bugging TACOS / Founders for Testimonials and Logos for the website.
- Janos: Developers are looking for features - Janos
- Roni: Enterprise is looking for stability / confidence

#### Proposed workflow between TSC, Tech Lead, and Core Team

[High Level Planning board on GitHub](https://github.com/orgs/opentofu/projects/9/views/1)

This board will serve as a discussion hub / common view into what tasks / goals are priorities. Between releases the Tech Lead will manage updates to this board, meeting with the Core Team and TSC independently to keep this board up to date.  

The [Top Voted Issues (GitHub)](https://github.com/opentofu/opentofu/issues/1496) should be taken into consideration when updating this board, alongside knowledge from TACOS about customer requests / roadblocks.

[Release Milestones on GitHub](https://github.com/opentofu/opentofu/milestone/7)

When the Core Team is wrapping up a release (alpha1 tentatively), the Tech Lead will meet with the Core Team and TSC independently to build out the next release milestone.  This discussion around building this milestone should be based on the top items on the High Level Planning board.

How public should this process be?  Items in high level planning might be useful to talk about in blog posts and articles, but could potentially misrepresent the flexibility of planning tasks at that level.  Milestones on GitHub are inherently public, though they are treated as a goal and not a commitment.  Frequently, issues that don’t make it into a given release are added as a higher priority into the next release milestone.

##### Ownership of the OpenTofu VSCode Extension

We have two repositories forked by a community member for the VSCode extension ([extension](https://github.com/gamunu/vscode-opentofu), [language server](https://github.com/gamunu/opentofu-ls)). Plus, this person owns the [OpenTofu extension in the Visual Studio Marketplace](https://marketplace.visualstudio.com/items?itemName=gamunu.opentofu).

He did some work on the mentioned repositories, but not recently. Plus, the extension has major bugs (e.g., [it throws errors when the Terraform extension is installed](https://github.com/gamunu/vscode-opentofu/issues/88)) and does not support our new features like State Encryption. The maintainer is not working actively on this extension.
We reached out to him by email to ask about collaboration. He mentioned he kept changes to a minimum since he’s unsure if he can maintain it without community backing. For easier collaboration, he suggested:

1. To add us as co-maintainers.
2. These projects should be moved under the OpenTofu organization.
    1. What permissions (if any) would the original forker have?

Do we want to accept any of the above suggestions? It is relevant to mention that the original Terraform repositories are owned by HashiCorp.

##### Discussion

- Roni: We can’t own the entire ecosystem, it’s a delicate balance on developer experience vs commitments.  Core team are influencers to help getting third party tooling going.
- Zach: My 2c is since our overall objective is opentofu adoption, fixing papercuts like IDE plugins should be in scope. Christian agrees.
- James: Full legal review of pulling in the code.  Prioritization?  Limited typescript knowledge in the core team.
- Roger: Golang considers lsp to be a high priority / in line with feature releases and documentation.  Expectation of developers.
- Ronny: How much control do we want/need over this project?
- Janos: Not sure if we have bandwidth for this.
- Roni: Ideally community would take this on, how needed is this today?
- Christian: We are introducing new features for 1.8 and 1.9 that would benefit from this.
- James: Let’s track it on the high level board and continue discussing what impact it would have on the capacity for the next releases.

##### Decision

- Let’s track it on the high level board and continue discussing what impact it would have on the capacity for the next releases.

## 2024-06-12

### Attendees

Core Team:

- Arel Rabinowitz ([@RLRabinowitz](https://github.com/RLRabinowitz))
- Christan Mesh ([@cam72cam](https://github.com/cam72cam))
- Jakub Martin ([@cube2222](https://github.com/cube2222))
- James Humphries ([@yantrio](https://github.com/Yantrio))
- Janos Bonic ([@janosdebugs](https://github.com/janosdebugs))

TSC:

- Igor Savchenko ([@DiscyDel](https://github.com/DicsyDel)) 
- Roger Simms ([@allofthesepeople](https://github.com/allofthesepeople))
- Roni Frantchi ([@roni-frantchi](https://github.com/roni-frantchi))
- Wojciech Barczynski ([@wojciech12](https://github.com/wojciech12))
- Zach Goldberg ([@ZachGoldberg](https://github.com/ZachGoldberg))

### Agenda

#### Vision of OpenTofu and it’s impact on the Organization

- Current status: as things are more stable, we can look for new directions
- What's our task moving forward:

  1. Maintaining status-quo
  2. Keep pace with Terraform
  3. Innovate IaC / bring new features

##### Discussion:

- Igor: outside influences may change this, but as a framework: innovation is good, but it can’t be to the detriment of people switching from Terraform/adopting OpenTofu.

- Janos: We would like to be able to talk longer-term features/functionality with the community.

- Igor: we should be talking about problems we want to solve.

- Roni: We started with OpenTofu being the only open-source alternative, the goal is winning hearts and minds. The roadmap ahead is the list of the most requested features.

- Arel: This is a good direction, but we would like to have a certain roadmap if possible. Terraform 1.10 has some very significant changes coming up based on a vision on how they want to do things.

- James: Do we drive decisions based on our own issue list, Terraform’s untouched 10 year old issues, Reddit... what do we care about? Old Terraform issues may lead us down the wrong path, for example.

- Kuba: We don’t necessarily need to map out everything ahead for a year, 6 months may do the trick.

- Igor: The main priority right now should be adoption. One-year goals are reasonable. We need to figure out why people don’t want to switch.

- Roger: We should try to get the info we can (e.g., from the TACOS).

- Wojciech: The TSC should take the Product Manager role (what are customers complaining about, etc)

- Zach: 

  1. Real World Experience w/Large enterprises — they are generally in 2 buckets - 
    1. Motivated by open source and eager for ammunition to make the case to spend $$ on moving to Tofu
    2. Cautious, in a “wait and see” position - not yet convinced tofu will be here in 5 years, don’t want to make a big bet on a direction that might not pan out for their org
    
       1. Often these customers are uneducated and have **incorrect opinions or bad facts** about how OpenTofu works or what migration means

   2. Goal: Be the state-of-the-art defacto IaC solution:

      1. Migration from TF has to be supported, first-class, easy for enterprise
      2. Tofu needs to provide motivation (i.e. stability, innovation, community) in addition to an on-ramp

   3. A strongly opinionated/narrow vision, e.g. “we want to build for XYZ” is possibly limiting our audience for now

- Igor: The vision for the next year is to focus on adoption. We’ll refine for a week, if nothing else comes up, we’ll adopt this.

- Roni: we may not need to define the technicalities of implementing the vision right now, that’s a separate discussion.

- Janos (async): We may have a perception problem, OpenTofu is publicly marketed as an “OpenSource Project” while Terraform is marketed as a “Product”. It may be possible to shift that by providing a platform for commercial support providers to list their offering.

#### Responsibilities between TSC / Tech Lead / Core team / Founders

1. Christian: our understanding is that Kuba until now would bring issues to the TSC’s attention.

2. Igor: Ideally, there should be checks and balances. The main role of the TSC is to make sure that the project is being developed in the interests of the community and making sure that OpenTofu is impartial. It is not the goal of the TSC to vote on individual issues. The TSC can vote on RFCs. Technically, we need to have a charter which we currently don’t have.

3. Christian: The new RFC is based on pull requests to make discussions easier. Question: do we want to go through the large amounts of work of writing an RFC before we ask the TSC, or should we ask the TSC with an enhancement that would need an RFC?

4. Wojciech: We want to keep the core team’s ability to bring ideas to the TSC. It would also good if Christian could join the TSC meetings since the written communication is not always fruitful.

5. Roni: The TSC would appreciate Christian being available for the meetings. Until now the RFC process was a place to ask a question “do we want this problem solved?” and there was a conflict with “how do we want the problem solved?”. We need this process defined better, especially with the problem to be solved in mind.

6. Christian: the current RFC process is very detailed (async: core team needs to communicate triage process)

7. Roger: The TSC steers the priorities of the core team.

8. Igor: The TSC capacity is very limited, we may not be able to go through all agenda items on time. Ideally, the core dev team is 80% independent. The core team can gather proposals and the TSC will do sanity checks. The TSC role is also to resolve conflicts.

9. James: I would like both the core team and the TSC to empower Christian to make these decisions because it will make the process easier. Christian joining the TSC meetings will make the process faster. Second issue: there is a difference between the TSC and the founders and Christian being in the middle should resolve surprise public communication happening.

10. Kuba: More concretely, Christian should propose a roadmap for feedback. Day-to-day Christian should make the decisions with an option to defer to the TSC. The option to escalate should also be open to core team members if they disagree with a decision. The biggest problem in this area so far was TSC capacity and the latency, which slowed down decision-making.

11. Christian: I can bring a rough roadmap that the TSC can then prioritize.

12. Roni: We trust Christian to make decisions in the day-to-day and when to escalate. This will allow the TSC to move a bit slower.

##### Decision

- Christian (OpenTofu Tech Lead) will join the TSC meetings.

#### Making the OpenTofu process public / outside communication

##### Discussion

- All outside communication should go through the Core Team and TSC
- Christian: We should move a lot of discussions to the public Slack
- Janos: As long as the communication is open, this shouldn’t be a problem in the future.

##### Decision

- General sentiment to move core-team discussions and related into public areas such as Slack/Github
- All outside communication should go through the Core Team and TSC

## 2024-06-04

### Attendees

- Wojciech Barczynski ([@wojciech12](https://github.com/wojciech12))
- Igor Savchenko ([@DiscyDel](https://github.com/DicsyDel)) 
- Zach Goldberg ([@ZachGoldberg](https://github.com/ZachGoldberg))
- Roger Simms ([@allofthesepeople](https://github.com/allofthesepeople))

### Agenda

#### IBM acquisition of Hashicorp

Shall we publish anything as OpenTofu regarding the acquisition?

##### Discussion/Decision

- Have a joined meeting with the Core team.
- Use it as opportunity to improve the communication between TSC and the Core team.

#### RFC should be MPL-licensed

##### Decision

All in favor (WB, IS, ZG, and RS)

#### Backend-as-Plugins

Continuation of the discussion from 2024-05-21.

##### Discussion

- Igor: we need a RFC before making a decision;
- Igor: The core team should decide which RFCs should be selected before the TSC.
- All: we see why it is important to make it easier to support for more backends out-of-the-box

#### 3rd party review service

@Wojciech Barczynski shared what 3rd party review services we could use in OpenTofu.

##### External funding and grants

@Igor Savchenko will explore sponsorship programs for Open Source, we could apply for.

#### Shall we join the openinventionnetwork?

@Roger Simms will reach out to them directly.

## 2024-05-21

### Attendees

- Wojciech Barczynski ([@wojciech12](https://github.com/wojciech12))
- Igor Savchenko ([@DiscyDel](https://github.com/DicsyDel)) 
- Zach Goldberg ([@ZachGoldberg](https://github.com/ZachGoldberg))
- Roni Frantchi ([@roni-frantchi](https://github.com/roni-frantchi))
- Roger Simms ([@allofthesepeople](https://github.com/allofthesepeople)) 

### Agenda

#### Christian Mesh, new Tech Lead of OpenTofu project

##### Discussion

- What is the definition of this role? Responsibilities?
- Who should define the roles/responsibilities?
- The team is very focused on 1.8, we do not want to context switch them to bureaucratic tasks.
- We might want to write down the TL's responsibilities after 1.8.x.

##### Decision

All in favor (WB, IS, ZG, RF, and RS)

#### Backend-as-Plugins

Continuation of the discussion from 2024-05-07.

##### Discussion

- A written-down product/project vision would help us to make decisions on topics such as e.g. backends as plugins.
- Do we need a RFC in order to vote on something as a steering committee?

  - Want to avoid appearance of bias as TACOs putting thumb on the scale for the roadmap
  - Is there a chicken-egg — companies want the idea to be accepted before investing in RFC, and the steering committee similarly wants a real RFC before agreeing.

- Backends as Plugins

  - @Roni Frantchi - Not convinced this is the right solution or something really worth prioritizing
  
  - @Igor Savchenko - it really needs an RFC for us to know what we’re voting on, it’s too abstract right now

  - @Roni Frantchi - if the steering committee says “OK lets see an RFC” is that actually interpreted as a green light for the community - once the RFC is up we’d move on it?

##### Decision

We cannot vote without a RFC. 

## 2024-05-07

## Agenda

### RFC: Init-time Constant Evaluation Proposal

https://github.com/opentofu/opentofu/issues/1042
This feature is a composable language addition that idiomatically solves many common top user requests (see linked issues):

- Module sources using variables/locals https://github.com/opentofu/opentofu/issues/286https://github.com/opentofu/opentofu/issues/1017
- Providers for_each support https://github.com/opentofu/opentofu/issues/300
- Module provider mappings from variables/for_each https://github.com/opentofu/opentofu/issues/300
- Backend configurations using variables/locals https://github.com/opentofu/opentofu/issues/388
- Lifecycle attributes must be known https://github.com/opentofu/opentofu/issues/304https://github.com/opentofu/opentofu/issues/1329
- Variable defaults/validation must be known https://github.com/opentofu/opentofu/issues/1336https://github.com/opentofu/opentofu/issues/1514
- Many more https://github.com/opentofu/opentofu/issues/1258

This proposal is an avenue to solve a very significant portion of the most frequently requested and top voted issues in OpenTofu and Terraform (see RFC for full related issue list). I am not proposing solving all of the above issues in a single PR/feature, but instead proposing a mechanism to support the addition of the above features in a simple and idiomatic way.

#### User Experience:

Many components of OpenTofu are dynamic and can refer to other objects/configuration. Users frequently attempt to use locals and variables in the above listed items, but are stymied by the fact that these items are quite limited in what they can reference.

This leads to significant additional complexity, with copy/paste between configurations being the default.  Some tools like Terragrunt offer ways to improve some of the common cases such as module sources, but many vanilla environments are severely lacking.  Another example is the -backend-config parameter and override files providing an incredibly hacky workaround to backend configuration limitations.

#### Technical Description:

OpenTofu's execution is split into two major components: setup to build the internal graph representation and evaluation of the graph.

What I propose is an init time constant evaluation stage be added between loading the configuration and building the graph.  This stage will be able to identify effectively constant values (variables/locals) which can be used during evaluation of the above.

Module Source Example:

```
variable "library_version" {
    type = string
}
module "my-mod" {
    source = "github.com/org/common-library?tag=${var.library_version}"
}

```

Provider Region Example

```
locals {
    region = {"primary": "us-east-1", "secondary": "us-east-2", "fallback": "us-west-1"}
}
provider "aws" {
    for_each = locals.region
    alias = each.key
    region = each.value
}
module "aws_resource" {
    source = "./per-region-module/"
    for_each = local.region
    providers = {
        aws = aws[each.key]
    }
}

```

#### Current State:

I have a [proof of concept](https://github.com/opentofu/opentofu/pull/1107) that supports variables in module sources, but more importantly implements an init time constant evaluation stage which supports that feature. The only remaining technical hurdle is how this interfaces with building the module tree when for_each is evaluated in the constant context. Given our teams recent work on the graph structure, I do not believe this will be a particularly difficult hurdle to overcome.

#### Summary:

I believe that by adding this init time evaluation we can idiomatically solve some of the largest pain points our users experience every day. In the future, we may re-design and re-engineer much of OpenTofu to remove the need for this evaluation stage.  However, this solves an incredible amount of our most important issues today.  It gives us the breathing room to decide on the technical future of the project without the pressure of users who are frustrated today.

#### Discussion:

- @Igor Savchenko thinks we should move forward - but we should have consider performance testing on large configurations and how those are impacted by the change. Also imported to note - this ***will*** break Terraform support.
- @Wojciech Barczyński reinforces that
- @Roger Simms 👍
- @Roni Frantchi asks - considering this is extending language features, and linters/language servers may raise errors or warnings if one uses this configuration - does that mean we will support that on .otf files only?

#### Decision:

- RFC accepted
- Ask core team to be mindful and rigorously test performance
- Strong recommendation to core team for consideration: if possible, the TSC’s preference is to roll out support for the above in iterations to limit the blast radius:
    1. Module source resolution
    2. Dynamic provider config
    3. …

### Alternate file extension .OTF for OpenTofu Specific Features

https://github.com/opentofu/opentofu/issues/1328

- Module and project authors can use .tf vs .otf to opt into different features but maintain support for both tools
- Easier for code completion / dev tools to support OpenTofu specific features
- Allows projects/modules to be written for OpenTofu and not Terraform if the author wishes (.otf only)
- Shows support / adoption of OpenTofu!
- Very little development effort required in OpenTofu

#### Discussion:

- TSC’s not entirely sure what’s the ask here - it’s been given green light for 1.8

### Next version of OpenTofu - what should it be? 1.8? 2.0?

- Implications on provider/module constraints?
- Feature comparison?

#### Discussion:

- @Roger Simms mentioned we wanted to get community feedback last time.
- @Wojciech Barczyński thinks we should carry on with the current versioning scheme while having our own release cycle as we gather more community feedback. He too fears jumping a major version can make people think OpenTofu introduces breaking changes that may deter them from joining.
- @Igor Savchenko currently, version compatibility and their mismatch in iterations confuses our users. We need something to distinguish between our own versions and HTF, while providing some kind of compatibility matrix? What is our versioning strategy?
- @Roni Frantchi is not concerned about 2.0 or 1.8 - either way, what concerns him is the provider/module constraints - for instance, right now providers may be adding constraints around new provider features support coming in HTF 1.8, which will “fail” compatibility as it checks OpenTofu’s version, despite beings supported in OTF 1.7

#### Decision:

- Come up with versioning strategy that is publicly available -
    - What is our release cadence?
    - Do we follow semver?
    - Consider in said strategy that there may be some perception that the release cadence of OTF is lower than that of HTF (despite OTF having more content in each) - should we have more frequent releases?
- Continue with current versioning schema for now (next version is 1.8)
- Open up an issue for the community to discuss whether we should change that versioning
- Open up an issue to track and come up with suggestion how the current or future versioning scheme will support provider constraints as support for new provider/module features are added on different version of OTF vs HTF

### Backends as Plugins

- We said we’re postponing it by 3 months, 3 months have passed.
- An oracle employee also reached out again, as they would like to add support for Oracle Object Storage as a Backend (and we blocked it by backends as plugins).

#### Discussion:

- @Igor Savchenko thinks it is not a priority still. As a TACO vendor, sounds great sure but - looking at the top ranking issue alternatives and what would benefit the community it does not seem remotely there.
- @Roger Simms similar
- @Wojciech Barczyński similar but we should reconsider again for next version.
- @Roni Frantchi similar

#### Decision:

Reconsider in 6 months

### The TSC never posted on GitHub summary since February

- We should discuss who owns this
    - AI: @Roni Frantchi will backtrack and post the summaries of all meetings dating back to Feb
    - From now on, whomever takes notes during the TSC meeting will also open the PR posting the public notes

### Change docs license in the charter to Mozilla as asked by Linux Foundation

#### Decision:

Yes.

## 2024-04-03

### Attendees:
    - Roger Simms;
    - Roni Frantchi;
    - Igor Savchenko;
    - Marcin Wyszynski
### Absent:
    - Jim Brikman

### Agenda

- The `tofu` keyword addition as an alternative to `terraform`.
    - The steering committee voted last week to block breaking changes until we have .otf files in place. However, since this is an opt-in feature, and it’s a community contribution by a member who **already created a PR for it,** it would be very discouraging to now tell them that “actually, this is gonna be frozen for a month”. Thus, I’d like to include it right away.
    - Options:
        - Reject for now, wait for RFC acceptance to the language divergence problem;
        - Accept just this one as an exception:
            - Igor: we acknowledge that this will break compatibility with some tools, but state encryption does it already, so we’re not making things worse, and we promise to have a long-term solution in 1.8 where this is a top priority. Good for adoption.
            - Roni: exception, we will not be accepting more until we have the solution to the divergence problem, including changes going into 1.8;
            - Roger: same;
            - Marcin: same;
        - We go back on our original decision, so it’s open season for divergence;
    - *Decision:* accept as an exception, unanimous;

- Discuss versioning, specifically continue to follow 1.6, 1.7, 1.8 versioning can cause confusion in differences between tofu and terraform. tofu 1.6 = terraform 1.6, but then tofu 1.7 is not the same as terraform 1.7 (both will have it’s own features and lack some). Longer we continue this pattern, more confusion it will cause on what are the differences and what are the expectations.
    - Concern (Roni): Terraform version constraints for modules;
    - We cannot forever follow Terraform versioning (Igor);
    - Could be weaponized by module authors hostile towards OpenTofu;
    - We cannot bet the future of the project on psychology of individuals - we don’t want to be anyone’s hostages;
    - Options:
        - Proposal 1: continue to use 1.X for terraform parity features and start. using 2.X for features specific to Opentofu (too late for State encryption). TBD how long we would like to support 1.X branch.
        - Proposal 2: Continue current versioning pattern;
        - Proposal 3: solicit input from the dev team and other stakeholders;
        - Proposal 4: go on the offense, provide module authors to explicitly state they don’t want their work used with OpenTofu;
        - Proposal 5: ignore Terraform versioning block in modules, if someone wants to put OpenTofu constraints, they need to put OpenTofu constraints explicitly;
    - *Decision:* take a week to openly discuss with different stakeholders, resume discussion during the next meeting;


## 2024-03-06

   1. Terraform variable/block Alias
   - For https://github.com/opentofu/opentofu/issues/1156 we realized we had not formalized what we will eventually alias `terraform` to as we move forward.  This covers the `terraform {}` block as well as the `terraform.workspace` and `terraform.env` config variables
      - During the community discussion on the Pull Request, the discussion revolved around using `tofu` as the alias or `meta` as the alias.
      - `tofu` fits with the current “theme” of the block and alias.
      - `meta` is more tool agnostic and may be a better long term / community option.
      - We’ve already accepted the addition of a tofu-specific namespace for this, and we’re just deciding which alias fits best
      - Note: This has an open PR by an external contributor and is blocking them: https://github.com/opentofu/opentofu/pull/1305
   - Other patterns to consider where the word “terraform” shows up:
      - `terraform.tfvars`: https://developer.hashicorp.com/terraform/language/values/variables
      - `.terraformrc`: https://developer.hashicorp.com/terraform/cli/config/config-file
      - There may be others…
      - The ones above we’d probably alias to `tofu` at some point
   - Decision:
      - Generally -  `tofu`
      - But then - this brings up an older discussion/decision made that the RFC to submit such “breaking” change in the language would require certain tooling (such, language server support for IDEs etc)
      - ***Before moving forward*** - we probably need an RFC for OpenTofu Language Server Protocol
      - Before introducing any language features or syntactic changes that diverge from Terraform language we should introduce some sort of file or other system to help language services distinguish between the two
         - Supporting tooling may break
         - E.g., tflint, IDE, etc
         - Creates a crappy experience for users
      - We should be extra careful with changes to the api of modules - having different file extension may affect the way they reference one another
         - E.g., A `.tf` file with `module "foo"` and a `.otf` file with `module "foo"`. What happens if something needs to reference `module.foo`? Which module gets used? How do language servers / IDEs check this stuff? How do module authors ensure compatibility.
         - So it’s mostly about the cross-references that touch module outputs.
      
      <aside>
      💡 We use the term “breaking change” here, but we don’t really mean the change is breaking. These are all opt-in features. But what we mean is that the experience may be poor: e.g., tflint/tfsec/etc will fail, IDEs won’t do syntax highlighting, etc.
      
      </aside>
      
      - Proposal 1: before moving on with any of the breaking changes RFC - we should choose and start to implement a solution that would allow those to coexist in IDEs/module authors and all the concerns listed above ?
         - Against: (everyone)
      - ✅ **[DECISION]** Proposal 1a: allow 1.7 to go out with state encryption as a breaking change, but block future breaking changes until we land this RFC in 1.8.
         - In favor: Roni, Jim, Igor, Roger, Marcin—**but with the caveat:**
               - If this is going to take more than ~1 quarter to implement, we should rethink it.
               - So we need to know: if we get someone working on this ASAP, is it likely that we can get this RFC approved, implemented, and included in 1.8?
               - If yes, let’s do it.
               - If no, let’s reconsider.
      - Proposal 2: maintain Tofu 1.6 LTS for a long period of time and always have it there as a backward compatible way to transition from Terraform. So you switch to that first, and then if happy in the Tofu world, then we have upgrade guides from Tofu 1.6 to newer versions, and those newer versions may have breaking changes.
         - In favor: Igor
      - Proposal 3: we ask for an initial placeholder RFC, but we don’t block releases with breaking changes on the RFC being completed/implemented. Instead, when we launch breaking changes, we have something that says “this is a breaking change, if you want to use it in a way compatible with Terraform, see this RFC, which will let you do that in a  future version.” And eventually, that RFC does land, and we have what we need without blocking.
         - In favor: Roger, Marcin, Igor


## 2024-02-27

1. Ecosystem
   - The core team feels strongly that we need to give users guidance on what safe migration paths from Terraform are and what the potential pitfalls exist. The lack of safe migration paths hinders adoption because users are left to their own devices to work around issues.
   Specifically, we would like to split the current migration guide into a few migration guides from a specific Terraform version x.y.z to a specific OpenTofu version a.b.c, outlining these few supported and tested migration paths that we are willing to fix bugs for if found. This would enable us to describe exactly what parts of the code users need to modify and what features may not be supported between those two specific versions.
   - **Decision:**
      - accept: Unanimous
      - reject:

2. Handling new language constructs
   - With state encryption, and the issue below, we’re introducing new constructs to the OpenTofu language. That also goes for functions we’ve added. In practice, for module authors this might lead to complexity or artificial limitations in order to support both Terraform and OpenTofu. Janos suggests that we introduce support for the .otf extension, and if there’s both [xyz.tf](http://xyz.tf) and xyz.otf in a directory (same name), we ignore xyz.tf. Thus introducing a simple way for people to support both in a single configuration.
   - Issue: https://github.com/opentofu/opentofu/issues/1275
   - *Decision:* TSC would like to see a **holistic RFC(s) it can vote on**, gather community feedback, consider capturing use cases of divergence from HTF and OTF side, and how these will be handled by IDE etc.
    

3. Deprecating module variables
   - We’ve accepted an issue to introduce the mechanism of deprecating module variables. After extensive discussions we’d like the steering committees decision on which approach to go with.
      - Issue: https://github.com/opentofu/opentofu/issues/1005
      - Approach 1
         - We add the deprecation as part of the variable description. Thus, one could embed `@deprecated: message` or `@deprecated{message}` (TBD) as part of their variable description, and tofu would raise a warning with the message.
         - Disadvantages: Magical and implicit. Introduces a new, slightly hacky, mechanism.
         - Advantages: Modules can use it while still supporting Terraform.
      - Approach 2
         - We add the deprecation as an explicit `deprecated` string field in the variable block.
         - Disadvantages: Modules using it will not work with Terraform, it will break on parsing the variable block.
         - Advantages: First-class, looks nicer and cleaner. Module authors can signal their support for OpenTofu by using this feature, and making their module not work with Terraform. Alternatively, module authors can use the .otf extension (if decided for, see above) to provide alternative code for OpenTofu.
      - Note: TSC doesn’t remember voting on accepting this issue;
      - **Decisions:**
         - Reject approach 1
         - Consider how approach 2 fits in with the OTF/HTF discrepancies RFCs

4. Functions in providers
   - This point is mostly a formality I believe, but: **Do we agree that functions in providers is something that we want to do** (regardless of when).
   - Note: Terraform is adding this in 1.8, **and the provider sdk including it is already stabilized and released.**
   - Note #2: If this is not too much work, we might actually get this into 1.7, and release support for this at the same time as Terraform. But that’s to be seen. To be clear, we’d only do this if Terraform 1.8 comes out first and we’re sure that the user-facing API is stabilized.
   - Note #3: proper issues / rfc’s for this will of course be created prior to implementation; this vote is just to get everybody on the same page around whether we’re doing this feature at all
   - **Decision:**
      - We do agree that functions in providers is something that we want to do (regardless of when).
      - Keep it out of OTF 1.7 - add to 1.8 roadmap

5. Registry UI
   - Registry UI, we haven’t seen anything appear, and people are asking for it. Please reevaluate.
   - **Decision:**
      - Please prioritize RFC of our own formal registry

## 2024-02-21

### Attendees

- Igor Savchenko ([@DiscyDel](https://github.com/DicsyDel))
- Roger Simms ([@allofthesepeople](https://github.com/allofthesepeople))
- Roni Frantchi ([@roni-frantchi](https://github.com/roni-frantchi))
- Wojciech Barczyński([wojciech12](https://github.com/wojciech12))

### Agenda

1. Comparison Table
   - Community members are asking for comparisons between OpenTofu and Terraform. It’s a tradeoff - on one hand it pushes us into the mindset of being a shadow-project, on the other hand it would ease the risk of migration and help those who value high levels of compatibility. It’s worth noting that the community has been creating similar tables already, like [nedinthecloud](https://nedinthecloud.com/2024/01/22/comparing-opentofu-and-terraform/).  
   - ❌ Add a compatibility table to the website
      - Yes:
      - No: Unanimous
   - ✅ Emphasize on the website much more strongly that we are a “drop in” replacement (100% compatible with TF [1.xxx](http://1.xxx) to OpenTofu 1.xxx)
      - *Ask the dev team to think about how to get this done*
      - Yes: Roni, Roger, Igor, Jim
      - No: Wojciech
   - ✅ Add/improve page on “why OpenTofu” (rather than just a compatibility table)
      - *Ask the dev team to think about how to get this done*
      - Yes: Unanimous
      - No:
   - ❓Add a “check” command OpenTofu that lets you know if you’re good to migrate from Terraform to OpenTofu or using features that might conflict
      - *This was a split vote. So let’s ask the dev team to think about what’s possible here and to come back with a more concrete proposal and see what they think.*
      - Yes: Roger, Igor, Jim
      - No: Roni, Wojciech
   - ❌ Add a “check” command OpenTofu that lets you know if you’re good to migrate from OpenTofu back to Terraform or using features that might conflict
      - Yes: Jim
      - No: Roger, Igor, Roni, Wojciech
   - Research what other projects have done in terms of migration tools
      - *No vote necessary*
      - Anyone on steering committee can do this and come back with more info
   - ❌ Add a “compatibility mode” that blocks usage of OpenTofu features not in Terraform
      - Yes:
      - No: Unanimous

## 2024-02-17

### Attendees

- Igor Savchenko ([@DiscyDel](https://github.com/DicsyDel))
- Marcin Wyszynski ([@marcinwyszynski](https://github.com/marcinwyszynski))
- Roger Simms ([@allofthesepeople](https://github.com/allofthesepeople))
- Roni Frantchi ([@roni-frantchi](https://github.com/roni-frantchi))
- Wojciech Barczyński([wojciech12](https://github.com/wojciech12))

### Absent

- Omry Hay ([@omry-hay](https://github.com/omry-hay))
- Yevgeniy Brikman ([@brikis98](https://github.com/brikis98))

### Agenda

1. Decide on producing more specific migration paths from Terraform to OpenT
   1. **Context** e.g. from Terraform version x.y.z to OpenTofu a.b.c. This would enable us to better describe exactly what parts of the code users need to modify and what features may not be supported between those two specific versions. 
   2. **Options**
      1. accept
      2. reject
   3. **Decision**: accept, unanimous
   
2. Decide if issue [OpenTofu-specific code override](https://github.com/opentofu/opentofu/issues/1275) should be accepted.
   1. **Context** as state encryption is a feature OpenTofu is adding which is not available to Terrafrom and we don't want to break compatibly for users trying out Tofu. This issue provides an option to create a new file type which would be used by OpenTofu but ignored by Terraform.
   2. **Decision** TSC would like to see a holistic RFC(s) in order to gather community feedback, provide alternatives and TFC to make decision on. Should consider capturing use cases of divergence from HTF to OTF and how these might be handled by IDEs etc.
   
3. Deprecating module variables.
   1. **Context** issue accepted to introduce mechanism of deprecating module variables. Options for this are: 
      1. Approach 1: add deprecation as part of the variable description. e.g. add something like `@deprecated: message` or `@deprecated{message}`and Tofu would raise a warning with the message.
         - *Advantages*: Modules can use this while still supporting Terraform.
         - *Disadvantages*: "Magical" and implicit solution. Introduces a new, slightly hacky, mechanism.
      2. Approach 2: add the deprecation as an explicit `deprecated`string field in the variable block.
         - *Advantages*: First-class support, looking cleaner and nicer. Module authors can signal their support for OpenTofu by using this feature, and making their module not work with Terraform. Alternatively, module authors can use the .otf extension (if decided for, see above) to provide alternative code for OpenTofu.
         - *Disadvantages*: Modules using it will not work with Terraform, it will break on parsing the variable block.
      3. **Decisions**:
         1. Reject approach 1
         2. Consider how approach 2 fits with OTF/HTF. Possibly include as part of above requested RFC on handling discrepancies. 

4. Functions in providers
   1. **Context** mostly a formality but do we agree "functions in providers" is something we want to do (without timeline or priority). 
      - **Note 1**: Terraform 1.8 is adding this feature and the provider sdk, including this, is already stabilized and released.
      - *Note 2*: This is not a lot of effort and could possibly make the Tofu 1.7 release, if Terraform 1.8 is released before Tofu 1.7 (to ensure API is stable).
      - *Note 3*: Full RFC will follow, this is mostly an ask from the core team to ensure everyone is in agreement with adding the feature at all.
   2. **Decisions** 
      - Agree feature should be added to the Tofu roadmap
      - Add to the Tofu 1.8 release, keep it out of 1.7 release, even if Terraform 1.8 is released wit the feature.

5. Registry UI
   1. **Context** previous decision to wait on this but community are now asking for it. 
   2. **Decision** Please prioritise RFC of OpenTofu's own formal registry. 

## 2024-01-30

### Attendees

- Igor Savchenko ([@DiscyDel](https://github.com/DicsyDel))
- Marcin Wyszynski ([@marcinwyszynski](https://github.com/marcinwyszynski))
- Roger Simms ([@allofthesepeople](https://github.com/allofthesepeople))
- Roni Frantchi ([@roni-frantchi](https://github.com/roni-frantchi))

### Absent

- Omry Hay ([@omry-hay](https://github.com/omry-hay))
- Yevgeniy Brikman ([@brikis98](https://github.com/brikis98))

### Agenda

1. Decide if [Backends as Plugins](https://github.com/opentofu/opentofu/issues/382) are accepted as a roadmap item
   1. **Context**  
      This is about accepting it as a long-term roadmap item - ie committing ourselves to doing it at some point, not necessarily for the specific implementation described in the RFC. Reasoning: We have many community members and companies wanting to get state backends in. We’d like to be able to answer “We’re not doing this, as we’ll be doing backends as plugins to handle it.” For this, we need this accepted by the TSC as a medium/long-term roadmap item.
   1. **Options**
      1. accept;
      1. reject (not go this direction);
      1. postpone the decision;
   1. **Decision**: postpone for 3 months
      Reasoning: this is not a key feature for our user base, and given that Terraform is seeing an increase in velocity, we need to churn out things that make a difference for average users.
1. Decide if we accept the [Allow specifying input variables as unknown in tofu plan](https://github.com/opentofu/opentofu/issues/812) proposal as a feature
   1. **Context**  
      - OpenTofu supports unknown values very well (e.g. outputs of not-yet-applied resources). This works well, but is not supported across statefiles. OpenTofu users commonly orchestrate multiple statefiles as a single "component" leading to multi-statefile plans, and unknown values that stem from a different statefile than your own.
      - Current solutions either use placeholder values (Terragrunt I believe), which are error prone as users sometimes accidentally apply the placeholder values, or they just use the previous variable value for the planning phase, which hides the actual blast radius a change in a statefile will have.
      - The goal here is to introduce the ability to mark an input variable of a tofu config as unknown during plan-time. This way all this tooling can properly signal what is actually the case - the variable is not currently known, due to changes in dependency statefiles. Leading to no error-proneness, and no hidden blast-radius.
      - This also involves making sure in the apply phase that all unknown inputs from the plan have been fully specified.
      - The issue contains a PoC in a comment, the expected changes required are limited, and mostly plumbing, as it's only further exposing existing functionality.
      - The goal here is to show momentum as a project, and at the same time provide better building blocks for external tooling.
   1. **Scope of acceptance**
      Accept it as an enhancement we'd like to introduce. The core team will iterate on the technical details, rfc and pick the rollout strategy / release we'd like to have it in.
   1. **Decision**
      Not ready to make a decision, please continue the investigation.
   1. **TSC notes**
      - research the possibility of implementing it as part of remote operations for the “cloud” backend. Also, make sure that it plays well with the vision of Terragrunt;
      - this potentially breaks an implicit contract that the plan indicates all changes made to the environment. With external inputs, the plan stops being 100% deterministic, which can break assumptions around processes like policy-as-code depending on the plan file. We should make this discussion part of the RFC;

## 2024-01-22 (async)

### Attendees

- n/a, we discussed directly in Notion;

### Agenda

1. How many historic releases we support
   1. **Context**  
      HashiCorp’s approach is to introduce patches for the most recent major (which means in their lingua changes to X and Y in X.Y.Z) release, as well as up to two prior ones. Which means that there are three supported releases at any given point in time.
   1. **Discussion**
      We discussed 3 options:
      1. **One release**. Only do patches for the most recent major release. So we are only supporting one release at any given point in time.
      1. **Two releases**. Only do patches for the most recent major release and the one before it. So we are only supporting one release at any given point in time.
      1. **Three releases**. Stick with HashiCorp’s approach: patches for the most recent major release, as well as up to two prior ones. So we support up to three releases at any given point in time.
   1. **Vote**: unanimous for option 3.
1. Certifications
   1. **Context**  
      Prominent community member asks us to provide some sort of certifications they can use to prove that we take security seriously.
   1. **Discussion**
      We discussed the following non-exclusive options:
      1. **SOC2 / ISO 27001.** Try to achieve these official certifications. Not clear how to do this for an open source organization though.
      1. **Code audit.** Perform an external code / security audit on the codebase.
      1. **Security scanning tools.** Install a variety of security scanning tools on the codebase: e.g., Snyk, DependaBot, Go Report Card, etc.
      1. **Security disclosure process.** Ensure we have a clear, well-defined, written process for (a) community members to disclose vulnerabilities to us, (b) us to escalate those and resolve them quickly, and (c) us to notify the rest of the community and roll out the patches.
   1. **Vote**: unanimous vote for security scanning tools and security disclosure process. Vote by Yevgeniy Brikman for code audit.
   1. **Follow-up**: Spacelift's Head of Security investigated certification and code audit, we will have him present his findings to the TSC at one of the following meetings.


## 2023-12-11

### Attendees:

- Igor Savchenko ([@DiscyDel](https://github.com/DicsyDel))
- Marcin Wyszynski ([@marcinwyszynski](https://github.com/marcinwyszynski))
- Roger Simms ([@allofthesepeople](https://github.com/allofthesepeople))
- Roni Frantchi ([@roni-frantchi](https://github.com/roni-frantchi))
- Yevgeniy Brikman ([@brikis98](https://github.com/brikis98))

### Absent
- Omry Hay ([@omry-hay](https://github.com/omry-hay))

### Agenda

1. Changing default registry namespace to opentofu
   1. **Context**  
      The namespace default change to OpenTofu had some unintended consequences and edge cases with versioning of providers and the interaction of having both hashicorp/xyz and xyz plain in a single config. See [Issue #988](https://github.com/opentofu/opentofu/issues/988) for more details.
   1. **Discussion**
      - Marcin thought a deprecation warning is acceptable
      - @Roni thinks there’s very little upside to making this change, and a deprecation warning of any sort could scare people away for no good reason, which with counter with the original goal of brand reputation and separation.
      - Igor suggested to soften the warning - just say something about it in docs, and not as deprecation warning label in the docs, something even softer
   1. **Vote**: Should we (a) Change the namespace (b) Add a deprecation notice when using unqualified (c) Do nothing of that sort for now, and mention something in our docs in the softest way possible
   2. **Decision**: Unanimous (c)

1. Launch date of GA
   1. **Context**  
      R&D team may be ready with a GA version as soon as December 20th. Marketing advised we should wait till after the holidays (Jan 10th) for a bigger splash.
   1. **Discussion**
      - Yevgeniy asks about engineering availability during the holiday to support early launch. Response was we have an engineering site that can support that, and Marcin mentioned other sites don’t mind jumping in some OpenTofu during the holidays as well
      - Igor and Yevgeniy felt like that while marketing/journalist’s availability as well as on personnel may be lower, engineers may actually appreciate getting their hands on something earlier and start evaluating the project during the holiday
        A mitigation was brought up to separate the marketing effort from the launch date: have OpenTofu roll out as soon as whenever ready (soft launch), and have marketing do its thing whenever they think it is most impactful (probably Jan 10th)
   1. **Vote:** Should we (a) Release GA on Dec 20th if ready as soft launch and launch marketing in Jan 10th (b) Release on Dec 20th (c) Release on Jan 10th [note on all options, we expect an RC at least a week prior to GA]
   1. **Decision**: Unanimous (c)

#### RFC/Issues reviewed

1. [Issue: Allow specifying input variables as unknown in tofu plan](https://github.com/opentofu/opentofu/issues/812)
   1. **Context**: This issue describes multiple workspaces and dependencies and how suggest a way to unlock a way to make it easier to work in such cases, also mentions Terragrunt run-all and how that would help in such cases there.
   1. **Discussion**
      - Igor and Marcin felt like the TSC should be discussion either issues that describe a wider problem and vote on asking for RFCs, or vote on RFCs
      - There was a broad discussion on workspace dependencies, what role Terragrunt plays, and what parts of it should and could be ported into OpenTofu
      - Yevgeniy felt like there are many reasons to accept 1st party support of having the entire feature in OpenTofu’s core; there are also reasons on the flipside to keep a smaller core and make sure the solution can be layered.
      - There was a broad discussion on us needing to define criteria for this specific issue, how it needs to be redefined as an issue describing a problem, the TSC should then first define criteria for RFCs, and only then ask for submission of those.
      - We then ran out of time - we will resume this topic and try to come with said criteria on the next meeting


## 2023-12-05

### Attendees:
- Igor Savchenko ([@DiscyDel](https://github.com/DicsyDel))
- Marcin Wyszynski ([@marcinwyszynski](https://github.com/marcinwyszynski))
- Roger Simms ([@allofthesepeople](https://github.com/allofthesepeople))
- Roni Frantchi ([@roni-frantchi](https://github.com/roni-frantchi))

### Absent
- Yevgeniy Brikman ([@brikis98](https://github.com/brikis98))
- Omry Hay ([@omry-hay](https://github.com/omry-hay))

### Agenda

#### Decide if we are changing the default registry namespace to opentofu
Decision: Quick unanimous yes

#### RFC/Issues reviewed

1. [Issue: UI for registry](https://github.com/opentofu/opentofu/issues/964)
   - **Discussion**
      - Roni thinks it should be a priority to have some UI there, especially as with the selected brew like design there’s convenient way to know which packages are supported by OpenTofu, and as we try to build the OpenTofu brand we cannot be sending people to have a look at HTF for modules/providers and docs (and be disappointed if some are missing and not submitted yet).
      - Igor thinks it’s a defocus for the core team and would rather not prioritize it. Disclosure: He is working on a library.tf project meant to serve as Terraform/OpenTofu registry. He thinks the community will benefit from one non-associated registry that would hold both Terraform and OpenTofu as well as other IaC.
      - Roger thought a UI/docs via the CLI would be a better way to go at it
      - Marcin had no strong opinion but also thought it is not a priority
   - **Vote**: Should we prioritize any registry ui/doc solution this Q?
      - Roni: Yes
      - Everyone else: No
   - **Decision**: Not a prio right now, but as most votes actually suggested some solutions, TSC would like to see some suggested RFCs for the above options to be submitted, and then we can re-examine

2. [RFC: Client-side state encryption](https://github.com/opentofu/opentofu/issues/874)
   - **Discussion**  
     Everyone agrees it should be accepted to as a highly requested feature, differentiating feature. @Marcin Wyszynski felt this is not the long term solution he would have wanted to see (don’t store secrets in state), but more upsides to having that intermediate solution now than not)
   - **Vote**: Should this be accepted as a higher priority this Q?
      - Everyone: Yes
   - **Decision:** Accept RFC an pull up

3. [Issue: Support OpenTofu in the VS Code Language Server](https://github.com/opentofu/opentofu/issues/970)
   - **Discussion**
      - Marcin thinks at this point it’s an unnecessary duplication and the language server would work just the same for OTF/HTF
      - Roni says that even if it doesn’t, it’s just a nice to have
      - Roger and Igor no strong opinion but think it is not a priority as well
   - **Vote**: Should this issue be accepted as a higher priority this Q?
      - Everyone: No
   - **Decision**: We may accept the RFC, but not have core team follow up on it

4. [PR: Implement gunzipbase64](https://github.com/opentofu/opentofu/pull/799)
   - **Discussion**
     Everyone agrees long term we would want OpenTofu to have custom function extensions/reuse as there’s gonna be a lot of these, but seeing it is an inverse of an existing function while its counterparts also have those, we should pursue.
   - **Vote**: Should this PR be accepted?
     Everyone: Yes
   - **Decision**: Accept PR

5. [PR: Fix filesystem state backend to be fast](https://github.com/opentofu/opentofu/pull/579)
   - **Vote**: Should this PR be accepted?
   - Everyone: Yes
   - Decision: Accept PR

With that, our time’s up. We’ll convene again next week to discuss more items.

## 2023-11-02
### Attendees: 
- Igor Savchenko ([@DiscyDel](https://github.com/DicsyDel))
- Marcin Wyszynski ([@marcinwyszynski](https://github.com/marcinwyszynski))
- Roger Simms ([@allofthesepeople](https://github.com/allofthesepeople))
- Roni Frantchi ([@roni-frantchi](https://github.com/roni-frantchi))

### Absent
- Yevgeniy Brikman ([@brikis98](https://github.com/brikis98))
- Omry Hay ([@omry-hay](https://github.com/omry-hay))

### Agenda

#### Selecting an RFC for a registry solution for resolving providers/modules
1. There was a **unanimous** consensus the RFC for [Homebrew-like artifact resolution registry component](https://github.com/opentofu/opentofu/issues/741) should be picked.
1. Main drivers for the decision (each in the meeting brought at least one of the following):  
   1. To be able to keep our word of being a drop-in replacement and innovate and win over hearts and minds we wish to have our core team focus on our CLI solution and avoid maintaining a highly available mission critical SaaS
   1. We wish to tie maximize our availability by standing on the shoulders of giants (GitHub/AWS)
   1. The transparency of the solution as a git repository, which is at the core of our competitive offering
   1. Decoupling between the solution of resolving artifacts and that of serving documentation, and artifact signing, would allow each component to be built according to their own non-functional requirements such as availability, and be replaced/evolve on its own.
1. Signing keys - we will launching with an equivalent level of security to the legacy registry, but the decoupled approach leaves the door open for future enhancements.
1. It was decided to not have the core team pursue a user facing registry design (i.e documentation part) just yet
1. Next steps are: (action item [@RLRabinowitz](https://github.com/RLRabinowitz) and [@cube2222](https://github.com/cube2222))
   1. Announcing selected RFC to the community ASAP
   1. Getting deeper into implementation details such as:
      1. Scraping (or not) of existing modules/providers + keys
      1. Detailed design on key submission
      1. Detailed design on version bumps
      1. Sharing the detailed design document
      1. Define scope and approach/breakdown of tasks for core team to pursue

#### Recurring technical steering committee meetings
  1. Since we believe we may have some backlog of agenda items still, we will start with a weekly meeting, currently looking at Thursday 7:30PM CET (exact _time_ pending, action item [@allofthesepeople](https://github.com/allofthesepeople))
  1. Agenda suggestions for meeting will be posted no less than 24h in advance, if no items are posted we will cancel the meeting

#### Personnel 
1. Following a founders meeting decision to hire core team members under various pledging companies their payroll and donate time, rather than under direct foundation payroll -
   1. Spacelift already hired two **dedicated** maintainers
   1. Spacelift built a profile and hiring pipeline dedicated for the Tofu effort which will be shared with companies interested in hiring Tofu dedicated personnel 
