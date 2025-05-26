# OpenTofu Governance

This document outlines the governance structure for the OpenTofu project, particularly focusing on the Technical Steering Committee (TSC) which has overall responsibility for the technical direction and oversight of the project as defined in the [CHARTER](CHARTER.md).

## Technical Steering Committee (TSC)

### TSC Membership Addition and Termination
#### Adding New TSC Members
New TSC members may be added to the TSC by the following process:
1. A candidate must be nominated by an existing TSC member
2. The candidate must have demonstrated significant contributions to the project, such as:
   - Code contributions
   - Documentation contributions
   - Community engagement and support
   - Technical leadership
3. The addition of a new TSC member requires a supermajority vote (two-thirds) of the existing TSC members
4. Upon approval, the new member is added to the TSC list and granted the appropriate access rights to project resources
#### TSC Member Termination or Resignation
TSC membership may be terminated or resigned through the following processes:
1. **Voluntary Resignation**: A TSC member may resign at any time by notifying the TSC in writing. No vote is required to accept a resignation.
2. **Removal for Inactivity**:
   - If a TSC member is inactive (no substantial participation in TSC meetings, votes, or project activities) for a period of 3 consecutive months, they may be considered for removal
   - Removal for inactivity requires a majority vote of the active TSC members
   - The member in question must be notified of the potential removal at least 14 days before the vote
3. **Removal for Cause**:
   - A TSC member may be removed for violations of the Code of Conduct or for actions deemed detrimental to the project
   - Removal for cause requires a supermajority vote (two-thirds) of the TSC, excluding the member in question
   - The member in question must be given an opportunity to address the concerns before the vote
4. **Loss of Affiliation**:
   - If a TSC member changes organizations and this change would violate the voting power restrictions, the TSC may need to rebalance membership
   - In such cases, the TSC will determine by consensus or vote which member(s) should step down
### Responsibilities
The TSC is responsible for all technical oversight of the OpenTofu project, including, but not limited to:
1. Coordinating the technical direction of the Project
2. Interpreting the provisions of the Technical Charter
3. Addressing legal matters concerning the Project in consultation with the Series Manager
4. Approving sub-project or system proposals
5. Organizing sub-projects and removing sub-projects
6. Creating sub-committees or working groups to focus on cross-project technical issues and requirements
7. Appointing representatives to work with other open source or open standards communities
8. Establishing community norms, workflows, and policies for the project
9. Enabling discussions, seeking consensus, and where necessary, voting on technical matters
10. Coordinating any marketing, events, or communications regarding the Project
### Meetings
The TSC meets every two weeks to discuss matters of importance and take votes if necessary. All voting TSC members shall have adequate notice of meetings. A proposed agenda for each meeting should be published at least 3 days in advance.
TSC meetings are open to the public and will be conducted electronically, via teleconference, or in person.
### Documentation of Decisions
The TSC will document its decisions in a public TSC folder in the main Project repository no later than 3 days after the vote was taken. For decisions not including confidential information, the TSC will include detailed notes on the discussion taken prior to the vote.
## Related Projects
The OpenTofu project may include or be related to other projects that are part of the broader OpenTofu ecosystem. This section outlines how these relationships are governed.
### OpenTofu Projects
Projects under the OpenTofu GitHub organization are considered part of the OpenTofu project family.
### Project Governance and Delegation
1. The TSC has ultimate authority over all projects in the OpenTofu organization.
2. The TSC may delegate decision-making authority to maintainers of specific projects:
   - Delegation of authority requires an explicit vote by the TSC
   - Delegated authority is considered a major decision requiring documentation
   - Projects with delegated authority must maintain a clear MAINTAINERS.md file
3. While maintainers of specific projects may have decision-making authority, the TSC retains the right to overrule decisions as necessary, though it should show restraint in doing so.
### Related External Projects
OpenTofu may have formal or informal relationships with external projects outside the OpenTofu organization:
1. **Formal Relationships**: Established through TSC vote and may include:
   - Technical collaborations
   - Shared governance structures
   - Co-maintained specifications or standards
2. **Informal Relationships**: Community-driven integrations and extensions that don't require formal governance oversight.
### Contributing to Related Projects
Contributions to related projects should follow:
1. The governance model of the specific project
2. The OpenTofu code of conduct
3. Any additional guidelines established by the TSC for cross-project collaboration
## Amendments
This governance document may be amended by a two-thirds vote of the entire TSC and is subject to approval by LF Projects.
