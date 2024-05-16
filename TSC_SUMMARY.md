# Technical Steering Committee (TSC) Summary  

The Technical Steering Committee is a group comprised of people from companies and projects backing OpenTofu. Its purpose is to have the final decision on any technical matters concerning the OpenTofu project, providing a model of governance that benefits the community as a whole, as opposed to being guided by any single company. The current members of the steering committee are:

- Igor Savchenko ([@DiscyDel](https://github.com/DicsyDel)) representing Scalr Inc.
- Roger Simms ([@allofthesepeople](https://github.com/allofthesepeople)) representing Harness Inc.
- Roni Frantchi ([@roni-frantchi](https://github.com/roni-frantchi)) representing env0
- Wojciech Barczynski ([@wojciech12](https://github.com/wojciech12)) representing Spacelift Inc.
- Yevgeniy Brikman ([@brikis98](https://github.com/brikis98)) representing Gruntwork, Inc.

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
- Open up an issue for the community to discuss wether we should change that versioning
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
