# Technical Steering Committee (TSC) Summary

## 2023-12-05

### Attendees: 
- Igor Savchenko ([@DiscyDel](https://github.com/DicsyDel))
- Marcin Wyszynski ([@marcinwyszynski](https://github.com/marcinwyszynski))
- Roger Simms ([@allofthesepeople](https://github.com/allofthesepeople))
- Roni Frantchi ([@roni-frantchi](https://github.com/roni-frantchi))

### Absent
- Yevgeniy Brikman ([@brikis98](https://github.com/brikis98))
- Omry Hary ([@omry-hay](https://github.com/omry-hay))

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
- Omry Hary ([@omry-hay](https://github.com/omry-hay))

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
