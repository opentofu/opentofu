# Technical Steering Committee (TSC) Summary  

The Technical Steering Committee is a group comprised of people from companies and projects backing OpenTofu. Its purpose is to have the final decision on any technical matters concerning the OpenTofu project, providing a model of governance that benefits the community as a whole, as opposed to being guided by any single company. The current members of the steering committee are:

- Igor Savchenko ([@DiscyDel](https://github.com/DicsyDel)) representing Scalr Inc.
- Roger Simms ([@allofthesepeople](https://github.com/allofthesepeople)) representing Harness Inc.
- Roni Frantchi ([@roni-frantchi](https://github.com/roni-frantchi)) representing env0
- Wojciech Barczynski ([@wojciech12](https://github.com/wojciech12)) representing Spacelift Inc.
- Yevgeniy Brikman ([@brikis98](https://github.com/brikis98)) representing Gruntwork, Inc.


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
