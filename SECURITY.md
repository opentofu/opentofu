# Security Reporting Process

Please report any security issue via
[Private Vulnerability Reporting](https://github.com/opentofu/opentofu/security/advisories/new) where the issue will be triaged appropriately.
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

The initial Product Security Team will consist of members of Steering Committee and Core Development Team. In the future we may
decide to have a subset of maintainers work on security response given that this process is time
consuming.

## Disclosures

### Private Disclosure Processes

The OpenTofu community asks that all suspected vulnerabilities be privately and responsibly disclosed
via the [reporting policy](README.md#reporting-security-vulnerabilities).

### Public Disclosure Processes

If you know of a publicly disclosed security vulnerability please IMMEDIATELY submit a report via
[Private Vulnerability Reporting](https://github.com/opentofu/opentofu/security/advisories/new) to inform the Product
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

The Fix Lead drives the schedule using their best judgment based on severity and development time. If the Fix Lead is
dealing with a public disclosure all timelines become ASAP (assuming the vulnerability has a CVSS
score >= 4). If the fix relies on another upstream project's disclosure timeline, that
will adjust the process as well. We will work with the upstream project to fit their timeline and
best protect our users.

## Security advisories in upstream dependencies

When a security problem is found in an library we depend on, we usually learn about it only once it
has already been publicly announced.

We use a scheduled job that periodically runs [`govulncheck`](https://go.dev/doc/tutorial/govulncheck)
to detect whether currently-supported OpenTofu releases might be affected by any advisories in the
[Go Vulnerability Database](https://pkg.go.dev/vuln/), and so we typically learn of these advisories
soon after they are published in
[issues labelled "govulncheck"](https://github.com/opentofu/opentofu/issues?q=is%3Aissue%20label%3Agovulncheck).

OpenTofu maintainers review each advisory to determine whether it actually impacts OpenTofu or whether
it is a false positive.

For any advisory that is relevant to users of OpenTofu, we produce new patch releases for any
currently-supported series that the available fixes can be applied to, and publish
[an OpenTofu-specific security advisory](https://github.com/opentofu/opentofu/security/advisories)
describing how the problem might impact users of OpenTofu.

For false positive advisories our policy is to document our conclusions in comments on the relevant
GitHub issue, and upgrade to a newer version of the dependency only on our main branch for inclusion
in the next minor release series. We do not typically backport these changes to earlier release series.

For more information on our process for upstream security advisories, refer to
[rfc/20250314-security-patch-policy.md](Security Advisory Policy for Upstream Dependencies).
