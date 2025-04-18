# Security Advisory Policy for Upstream Dependencies

> [!NOTE]
>
> This particular policy is focused on responses to advisories in our _upstream dependencies_, rather than in the OpenTofu codebase itself. Our policy and process for accepting and responding to directly-reported security issues is not in scope for this document.
>
> **Please disclose OpenTofu-specific security concerns responsibly per the Security Reporting Process documented in the root of the repository.**

OpenTofu is often used in environments that make use of software that attempts to detect whether our releases are indirectly affected by security advisories in upstream dependencies.

Unfortunately, there is a considerable variation in the accuracy of different tools in this category, and some have quite a high false-positive rate. Organizations that employ those tools tend to notify us when they see a problem -- which we are grateful for! -- but since upgrading dependencies in our patch releases always involves some risk, and issuing new patch releases has an opportunity cost against other development, we need to find a balance where we can focus on actual vulnerabilities and minimize time spent responding to false-positive reports.

So far we've tended to handle each report in isolation, essentially revisiting our policy on these from scratch each time to make the risk/reward tradeoff from base principles. Now that we've had the opportunity to learn from those experiences, it's time to formalize our process for dealing with indirect security advisories reported against our upstream dependencies so that we can respond efficiently and effectively when they occur and so those who use OpenTofu can know what to expect from us when such a situation occurs.

This policy covers three separate but connected steps in this process:

- Learning that there is a potential upstream advisory that we need to respond to.
- Evaluating the impact of the advisory and deciding how to respond to it.
- Sharing our conclusions with the community in a consistent way.

Because OpenTofu CLI/Core is a Go codebase using dependencies also written in Go, our process is influenced by [Vulnerability Management for Go](https://go.dev/blog/vuln), and will rely on the Go project's tools and database as our primary source of information on indirect security advisories.

## Learning that there is an upstream advisory

In order to react proactively to newly-reported advisories, the OpenTofu project will use daily scheduled runs of [`govulncheck`](https://pkg.go.dev/golang.org/x/vuln/cmd/govulncheck) against our `main` branch and against the maintenence branches for all of the minor release series that have not reached end-of-life under our security maintenence policy.

These scheduled runs will be against the _source code_ of each included release, rather than the binaries built from them, because that mode provides the most precise results from `govulncheck` that give the lowest possible false-positive rate. This tool indirectly relies on the [Go Vulnerability Database](https://go.dev/doc/security/vuln/database), which is run by the Go project and aggregates security advisories against publicly-available Go modules with package-level and symbol-level detail.

We will also continue to gratefully accept third-party reports via our issue tracker. If such reports concern advisories that `golvulncheck` does not also detect then we will use the available information to distinguish between two cases:

1. The problem is recorded in other vulnerability databases but not yet included in the Go Vulnerability Database.

    In this case, we will first consult [the open issues related to the vulnerability database](https://github.com/golang/vulndb/issues) to determine if a related report is pending review, and use the draft information there in conjunction with information available from other sources to determine whether OpenTofu is affected by the problem.

    If there is not yet even any _proposed_ entry for the Go Vulnerability Database then we will use information from other available databases to determine whether OpenTofu is affected.

2. The problem is already included in the Go Vulnerability Database, but wasn't reported by `govulncheck` specifically because the problem does not affect any API that OpenTofu interacts with.

    This situation represents a false-positive, in which case we will typically skip directly to the step of sharing our conclusions, explaining why OpenTofu is not affected by the problem, without making any changes to the codebase. In this case, lower-accuracy security scanners may still incorrectly flag OpenTofu as affected by the problem, in which case we encourage organizations to report that inaccuracy to their security scanner vendor so that they can improve their product's accuracy and lower its false-positive rate.

## Evaluating the impact and responding

The `govulncheck` tool produces a detailed report of each source code line in the OpenTofu codebase that interacts with upstream API that is affected by an advisory. We will review the indicated code and consider its relationship with the content of the security advisory.

The vulnerability database suggests a minimum version of the upstream dependency that contains the resolution of the reported problem, and so our default response will be to upgrade to that suggested version. However, some upstreams include security fixes only along with other changes in a release and in such cases we will need to carefully consider the potential impact of any other changes we'd be adopting along with that upgrade.

If we find that adopting the proposed fix version would regress other OpenTofu functionality then our first preference would be to work with the upstream to help them to produce a security-focused patch release that we can upgrade to safely. Otherwise, we will implement mitigations in the OpenTofu codebase itself, including potentially adopting parts of the upstream library directly into OpenTofu itself so that we can preserve the previous functionality we depended on until OpenTofu's next minor release.

We will backport the resolution to any valid security advisory to all of the minor release series that are affected by it and that have not yet reached end-of-life under our security support policy, and will then issue new patch releases in those affected series.

As a matter of policy we will _not_ adopt an upgrade of a third-party dependency in a patch release only to quiet a false-positive report from an imprecise security scanner. Organizations using those security scanners are encouraged to notify their vendor about the false positive so that they can improve their false-positive rate. However, we _will_ typically adopt new releases of upstream dependencies in our `main` branch for inclusion in the next minor release series unless it would cause some other regression that we are not yet ready to address, so that such false positives will not accumulate indefinitely.

## Sharing our conclusions

We will respond to any upstream advisory that was either detected by `govulncheck` or reported in good faith by a community member by publishing a security advisory in [our repository's Security Advisories section on GitHub](https://github.com/opentofu/opentofu/security/advisories).

Our advisory for each report will include a summary of whether and how the report relates to each of the minor release series that are not yet end-of-life. If our response included the issuing of new patch releases in any of those series, we will clearly indicate the minimum patch release in each series that includes the fix.

We will publish _low-severity_ advisories even for reports that we conclude to be false-positives, explaining our reasoning for that decision and indicating that no versions of OpenTofu are affected and no upgrades are required. Such advisories serve to acknowledge that we became aware of the potential for a problem and have actively investigated it, so that those whose own security scanners generate false positives about it can be confident that we are already aware of the (non-)problem and therefore they do not need to re-report it.
