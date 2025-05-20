# Technical Charter (the “Charter”) for OpenTofu a Series of LF Projects, LLC

| Revision |  Date  |
|---|---|
| Initial Adoption | September 15, 2023 |
| Amendment | May 20, 2025 |

This Charter sets forth the responsibilities and procedures for technical contribution to, and oversight of, the OpenTofu open source project, which has been established as OpenTofu, a Series of LF Projects, LLC (the “Project”).  LF Projects, LLC (“LF Projects”) is a Delaware series limited liability company. All contributors and participants (including committers, maintainers, and other technical positions) in the Project (collectively, “Contributors”) must comply with the terms of this Charter.

1. **Mission and Scope of the Project**

   a. The mission of the Project is to develop and preserve open source infrastructure as code.

   b. The scope of the Project includes collaborative development under the Project License (as defined herein) supporting the mission, including documentation, testing, integration and the creation of tools, libraries, and other artifacts that aid the development, deployment, operation, or adoption of the open source project.  The Project intends to be impartial and community-driven.

2. **Technical Steering Committee**

   a. The Technical Steering Committee (the “TSC”) will be responsible for all technical oversight of the open source Project.

      The TSC voting members are initially those Committers of the Project that are listed as “TSC Members” within the “CONTRIBUTING” file within the Project’s code repository. TSC members selected to serve on the TSC serve until their resignation or replacement by the TSC. The TSC may choose an alternative approach for determining the voting members of the TSC, and any such alternative approach will be documented in the CONTRIBUTING file.  Any meetings of the Technical Steering Committee are intended to be open to the public, and can be conducted electronically, via teleconference, or in person.

   b. TSC projects generally will involve Contributors and Committers. The TSC may adopt or modify roles so long as the roles are documented in the CONTRIBUTING file. Unless otherwise documented: 

      * i. Contributors include anyone in the technical community that contributes code, documentation, or other technical artifacts to the Project;

      * ii. Committers are Contributors who have earned the ability to modify (“commit”) source code, documentation or other technical artifacts in a project’s repository; and  

      * iii. A Contributor may become a Committer by approval of the TSC. A Committer may be removed by a majority approval of the TSC.

   c. Participation in the Project through becoming a Contributor and Committer is open to anyone so long as they abide by the terms of this Charter.

   d. The TSC may (1) establish work flow procedures for the submission, approval, and closure/archiving of projects, (2) set requirements for the promotion of Contributors to Committer status, as applicable, and (3) amend, adjust, refine and/or eliminate the roles of Contributors, and Committers, and create new roles, and publicly document any TSC roles, as it sees fit.

   e. The TSC may elect a TSC Chair, who will preside over meetings of the TSC and will serve until their resignation or replacement by the TSC.  

   f. Responsibilities: The TSC will be responsible for all aspects of oversight relating to the Project, which may include:

      * i. Coordinating the technical direction of the Project;

      * ii. Interpreting the provisions of the Charter;

      * iii. Addressing legal matters concerning the Project (including matters concerning Section 7 of this Charter) in consultation with the Series Manager;

      * iv. Approving sub-project or system proposals (including, but not limited to, incubation, deprecation, and changes to a sub-project’s scope);

      * v. Organizing sub-projects and removing sub-projects;

      * vi. Creating sub-committees or working groups to focus on cross-project technical issues and requirements;

      * vii. Appointing representatives to work with other open source or open standards communities or to other organizations working with the Project;

      * viii. Establishing community norms and rules for contribution, workflows, release candidates, and security issue reporting policies, which will be documented in the CONTRIBUTING file and similar files in the repositories of the Project;

      * ix. Enabling discussions, seeking consensus, and where necessary, voting on technical matters relating to the code base that affect multiple projects; and

      * x. Coordinating any marketing, events, or communications regarding the Project.

3. **TSC Voting**

   a. The TSC meets regularly to discuss matters of importance and take votes if necessary. All voting TSC members shall have adequate notice of meetings.

   b. The TSC makes decisions primarily by consensus. Where no consensus can be reached, the TSC takes a vote.

      * i. Each TSC member is entitled to one vote, provided, however, that in the event that more than the TSC Cap (as defined below) number of TSC members are employed by the same company or by the same group of related companies, then those TSC members will have their combined votes limited to the TSC Cap.  The "TSC Cap" means, in cases where the TSC has at least 5 members, two, and in cases where the TSC has four or fewer members, one.

      * ii. Voting TSC members have a duty to make a good-faith effort to regularly participate in votes and notify other voting TSC members of their availability.

      * iii. Voting may take place during meetings or through asynchronous methods approved by the TSC.
In-Meeting Voting: A vote may proceed when at least 50% of all voting TSC members are present (physically or virtually). Unless this charter specifies a higher threshold, a motion is considered approved if it receives a majority of the votes cast during the meeting.
Asynchronous Voting: For votes conducted outside of meetings (e.g., via email or designated online platforms), a motion is considered approved only if it receives affirmative votes from at least 50% of all voting TSC members.

      * iv. Supermajority voting is conducted by a meeting with all voting TSC members participating or by asynchronous means. Voting TSC members must have adequate notice of the time and subject of a supermajority vote and may request the postponement of the vote by 14 calendar days if necessary. Supermajority votes require a two-thirds majority of all voting TSC members to pass.

      * v. In the event a vote cannot be resolved by the TSC, any voting member of the TSC may refer the matter to the Series Manager for assistance in reaching a resolution.

   c. Supermajority voting is required for:

      * i. Amending this Charter (subject to approval by LF Projects) as outlined in Section 8.

      * ii. Legal matters concerning the Project.

      * iii. Approving the use of an alternative license subject to the restrictions outlined in 7.c.

   d. Asynchronous voting may occur on any suitable medium where all voting TSC members have at least 14 calendar days of verifiable notice to cast their vote.

   e. The TSC will document its decisions in a public file in the main Project repository no later than 14 days after the vote was taken. For decisions not including confidential information, the TSC will include detailed notes on the discussion taken prior to the vote.

4. **Compliance with Policies**

   a. This Charter is subject to the Series Agreement for the Project and the Operating Agreement of LF Projects. Contributors will comply with the policies of LF Projects as may be adopted and amended by LF Projects, including, without limitation the policies listed at [https://lfprojects.org/policies](https://lfprojects.org/policies).

   b. The TSC may adopt a code of conduct (“CoC”) for the Project, which is subject to approval by the Series Manager.  In the event that a Project-specific CoC has not been approved, the LF Projects Code of Conduct listed at [https://lfprojects.org/policies](https://lfprojects.org/policies) will apply for all Contributors in the Project.

   c. When amending or adopting any policy applicable to the Project, LF Projects will publish such policy, as to be amended or adopted, on its web site at least 30 days prior to such policy taking effect; provided, however, that in the case of any amendment of the Trademark Policy or Terms of Use of LF Projects, any such amendment is effective upon publication on LF Projects' web site.

   d. All Contributors must allow open participation from any individual or organization meeting the requirements for contributing under this Charter and any policies adopted for all Contributors by the TSC, regardless of competitive interests. Put another way, the Project community must not seek to exclude any participant based on any criteria, requirement, or reason other than those that are reasonable and applied on a non-discriminatory basis to all Contributors in the Project community.

   e. The Project will operate in a transparent, open, collaborative, and ethical manner at all times. The output of all Project discussions, proposals, timelines, decisions, and status should be made open and easily visible to all. Any potential violations of this requirement should be reported immediately to the Series Manager.

5. **Community Assets**

   a. LF Projects will hold title to all trade or service marks used by the Project (“Project Trademarks”), whether based on common law or registered rights.  Project Trademarks will be transferred and assigned to LF Projects to hold on behalf of the Project. Any use of any Project Trademarks by Contributors in the Project will be in accordance with the license from LF Projects and inure to the benefit of LF Projects.  

   b. The Project will, as permitted and in accordance with such license from LF Projects, develop and own all Project GitHub and social media accounts, and domain name registrations created by the Project community.

   c. Under no circumstances will LF Projects be expected or required to undertake any action on behalf of the Project that is inconsistent with the tax-exempt status or purpose, as applicable, of the Joint Development Foundation or LF Projects, LLC.

6. **General Rules and Operations.**

   a. The Project will:

      * i. engage in the work of the Project in a professional manner consistent with maintaining a cohesive community, while also maintaining the goodwill and esteem of LF Projects, Joint Development Foundation and other partner organizations in the open source community; and

      * ii. respect the rights of all trademark owners, including any branding and trademark usage guidelines.

7. **Intellectual Property Policy**

   a. Contributors acknowledge that the copyright in all new contributions will be retained by the copyright holder as independent works of authorship and that no contributor or copyright holder will be required to assign copyrights to the Project.

   b. Except as described in Section 7.c., all contributions to the Project are subject to the following:

      * i. All new inbound code contributions to the Project must be made using Mozilla Public License Version 2.0 (the “Project License”).

      * ii. All new inbound code contributions must also be accompanied by a Developer Certificate of Origin ([http://developercertificate.org](http://developercertificate.org)) sign-off in the source code system that is submitted through a TSC-approved contribution process which will bind the authorized contributor and, if not self-employed, their employer to the applicable license;

      * iii. All outbound code will be made available under the Project License.

      * iv. Documentation will be received and made available by the Project under the Creative Commons Attribution 4.0 International License (available at [http://creativecommons.org/licenses/by/4.0/](http://creativecommons.org/licenses/by/4.0/)).

      * v. The Project may seek to integrate and contribute back to other open source projects (“Upstream Projects”). In such cases, the Project will conform to all license requirements of the Upstream Projects, including dependencies, leveraged by the Project.  Upstream Project code contributions not stored within the Project’s main code repository will comply with the contribution process and license terms for the applicable Upstream Project.

   c. The TSC may approve the use of an alternative license or licenses for inbound or outbound contributions on an exception basis. To request an exception, please describe the contribution, the alternative open source license(s), and the justification for using an alternative open source license for the Project. License exceptions must be approved by a two-thirds vote of the entire TSC.

   d. Contributed files should contain license information, such as SPDX short form identifiers, indicating the open source license or licenses pertaining to the file.

8. **Amendments**

   a. This charter may be amended by a two-thirds vote of the entire TSC and is subject to approval by LF Projects.


