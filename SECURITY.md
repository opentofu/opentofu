# Security Reporting Process

Please report any security issue or OpenTofu crash report to
security@opentofu.org where the issue will be triaged appropriately.
Thank you in advance for helping to keep OpenTofu secure.

# Security Release Process

OpenTofu is a large growing community of volunteers, users, and vendors. The OpenTofu community has
adopted this security disclosure and response policy to ensure we responsibly handle critical
issues.

## Product Security Team (PST)

Security vulnerabilities should be handled quickly and sometimes privately. The primary goal of this
process is to reduce the total time users are vulnerable to publicly known exploits.

The Product Security Team (PST) is responsible for organizing the entire response including internal
communication and external disclosure but will need help from relevant developers to successfully
run this process.

The initial Product Security Team will consist of members of Steering Committee and Core Development Team in the private
[OpenTofu Security](https://groups.google.com/a/opentofu.org/g/security) list. In the future we may
decide to have a subset of maintainers work on security response given that this process is time
consuming.

## Disclosures

### Private Disclosure Processes

The OpenTofu community asks that all suspected vulnerabilities be privately and responsibly disclosed
via the [reporting policy](README.md#reporting-security-vulnerabilities).

### Public Disclosure Processes

If you know of a publicly disclosed security vulnerability please IMMEDIATELY email
[security@opentofu.org](security@opentofu.org) to inform the Product
Security Team (PST) about the vulnerability so they may start the patch, release, and communication
process.

If possible the PST will ask the person making the public report if the issue can be handled via a
private disclosure process (for example if the full exploit details have not yet been published). If
the reporter denies the request for private disclosure, the PST will move swiftly with the fix and
release process. In extreme cases GitHub can be asked to delete the issue but this generally isn't
necessary and is unlikely to make a public disclosure less damaging.

## Patch, Release, and Public Communication

For each vulnerability a member of the PST will volunteer to lead coordination with the "Fix Team"
and is responsible for sending disclosure emails to the rest of the community. This lead will be
referred to as the "Fix Lead."

The role of Fix Lead should rotate round-robin across the PST.

Note that given the current size of the OpenTofu community it is likely that the PST is the same as
the "Fix team." (I.e., all maintainers). The PST may decide to bring in additional contributors
for added expertise depending on the area of the code that contains the vulnerability.

All of the timelines below are suggestions and assume a private disclosure. The Fix Lead drives the
schedule using their best judgment based on severity and development time. If the Fix Lead is
dealing with a public disclosure all timelines become ASAP (assuming the vulnerability has a CVSS
score >= 4; see below). If the fix relies on another upstream project's disclosure timeline, that
will adjust the process as well. We will work with the upstream project to fit their timeline and
best protect our users.

### Fix Team Organization

These steps should be completed within the first 24 hours of disclosure.

- The Fix Lead will work quickly to identify relevant engineers from the affected projects and
  packages and CC those engineers into the disclosure thread. These selected developers are the Fix
  Team.
- The Fix Lead will get the Fix Team access to private security repos to develop the fix.

### Fix Development Process

These steps should be completed within the 1-7 days of Disclosure.

- The Fix Lead and the Fix Team will create a
  [CVSS](https://www.first.org/cvss/specification-document) using the [CVSS
  Calculator](https://www.first.org/cvss/calculator/3.0). The Fix Lead makes the final call on the
  calculated CVSS; it is better to move quickly than making the CVSS perfect.
- The Fix Team will notify the Fix Lead that work on the fix branch is complete once there are LGTMs
  on all commits in the private repo from one or more maintainers.

If the CVSS score is under 4.0 ([a low severity
score](https://www.first.org/cvss/specification-document#i5)) the Fix Team can decide to slow the
release process down in the face of holidays, developer bandwidth, etc. These decisions must be
discussed on the opentofu secruity mailing list.

A three week window will be provided to members of the private distributor list from candidate patch
availability until the security release date. It is expected that distributors will normally be able
to perform a release within this time window. If there are exceptional circumstances, the OpenTofu
security team will raise this window to four weeks. The release window will be reduced if the
security issue is public or embargo is broken.

### Fix and disclosure SLOs

* All reports to security@opentofu.org will be triaged and have an
  initial response within 3 business days.

* Privately disclosed issues will be fixed or publicly disclosed within 90 days
  by the OpenTofu security team. In exceptional circumstances we reserve the right
  to work with the discloser to coordinate on an extension, but this will be
  rarely used.

* Any issue discovered by the OpenTofu security team and raised in our private bug
  tracker will be converted to a public issue within 90 days. We will regularly
  audit these issues to ensure that no major vulnerability (from the perspective
  of the threat model) is accidentally leaked.
