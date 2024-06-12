# GPG Key Verification for the Registry

Issue: https://github.com/opentofu/opentofu/issues/832 

> [!NOTE]  
> This RFC was originally written by @Yantrio and was ported from the old RFC process. It should not be used as a reference for current RFC best practices.

Building upon the foundations laid out in the Homebrew-like Registry RFC (issue #741) and related to the Stable Registry Repository Folder Structure (issue #827), this document proposes a secure method to handle GPG keys within our Registry. The use of GPG keys is critical for the validation of provider artifacts, as it enables users to verify the integrity and the origin of the content they rely on.

## Proposed Solution

Provider authors will need to be able to upload their public GPG keys to a dedicated directory within the registry's GitHub repository. These keys will then be employed, or will have already been employed to sign the SHASUMS of the artifacts, providing a layer of trust and verification for clients consuming these resources.

To maintain the registry's integrity, it is essential that the process of uploading GPG keys is tightly controlled. We must enforce comprehensive validation steps to ascertain that individuals uploading GPG keys are authorized to do so and that the keys themselves are genuine. The goal of this validation process is to protect against a spectrum of security threats, including but not limited to impersonation and man-in-the-middle attacks.

### Automation vs Human Approval in GPG Key Validation

#### Decision Factors

In the development of the registry, an important consideration has been the balance between automation and human intervention in the verification of GPG key uploads. Our mission is to streamline the process, enhancing both efficiency and security. While an entirely automated system could significantly expedite operations, security is inherently strengthened when approached with a multitude of measures.

#### Proposed Combination Approach

The recommended approach is a hybrid system where automation serves as the first line of defense, assisting in the preliminary verification of GPG keys and the identities of their submitters. This system is designed to facilitate rapid processing of GPG key submissions while establishing a rigorous security protocol.

##### Automation

For creating an automated suite of checks, a suite of individual github actions can be applied whenever a GPG key is uploaded. Small, individual checks allow us to extend the suite of checks in the future, and also quickly see at a glance, as humans, which of these checks has failed. It also means that the checks can be written in a multitude of languages/approaches (Bash for simple, golang for complicated).

In developing a secure and efficient workflow for the Opentofu registry, the following automated verification steps should be implemented for GPG key submissions:

1. **Key Format Verification**:
   - Confirm that the key file follows the proper naming conventions.
   - Validate that the key is stored in the correct directory structure.
   - Ensure that the key being uploaded is the public part of the key pair, never the private key.

2. **GitHub Organization Membership Validation**:
   - Check that the individual uploading the key is an authenticated member of the GitHub organization associated with the providers.

3. **Contributor Activity Check**:
   - Verify that the submitter has made active and recent contributions to repositories within the organization, evidencing their ongoing involvement.

4. **Key Expiry Assessment**:
   - Ensure that the GPG key has not expired. An expired key will trigger a manual review process as it may indicate an outdated or compromised key.

5. **Key Usage Compatibility**:
   - Verify that the key is RSA-based, as required for compatibility with Opentofu.
   - Confirm that the key has the necessary flags for signing, indicating it is intended for the correct purposes.

6. **Identity Verification**:
   - Validate that the key has a legitimate identity associated with it, including a verifiable real email address.

7. **Artifact Signing Check**:
   - Confirm that the GPG key has been used to sign all available releases in the relevant GitHub provider repository.
   - If multiple keys are present, ensure that every release is accounted for by the provided keys.

8. **Certificate Revocation Status** (Stretch Goal):
   - Evaluate if the key has an accessible certificate revocation mechanism in place.
   - Check the revocation status to ensure the key has not been compromised and is still trusted.

This comprehensive automated verification workflow is designed to pre-screen submissions thoroughly before human review. While the goal is to automate rigorously, the system acknowledges the complexity of security and the need for a final human evaluation to ensure the utmost integrity of the Opentofu registry.

##### Human Oversight

After the automated checks are completed, the role of human oversight becomes crucial. The process mandates that:

1. An authorized individual will review the output of the automated checks.
2. This reviewer will verify the authenticity of the key submission and the credibility of the submitter.
3. Any flags or irregularities identified by the automation will be scrutinized.

This level of human intervention serves as a robust safeguard against the exploitation of potential vulnerabilities in the automated system.

#### Trade-offs

The primary trade-off of this hybrid approach is the introduction of additional latency in the GPG key approval process due to the requirement of human oversight. This extra step necessitates an allocation of human resources and could extend the time taken for keys to be integrated into the registry. However we hope that the improvement of the Automation as an aid to the human verification reduces this trade-off in the long-term.

#### Conclusion

Despite the trade-off, the hybrid model is essential to maintain the integrity of the Opentofu registry. By combining automated pre-checks with human validation, we create a dynamic defense system capable of adapting to new threats and anomalies. I advocate for this model as it establishes a robust verification mechanism while retaining the flexibility to evolve with the changing landscape of cybersecurity threats.


### Open Questions

Could we ever switch thisti a fully automated process?  Perhaps as we become more confident in the automation?

## Potential Alternatives

