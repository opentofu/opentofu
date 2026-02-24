# OpenTofu Bug Report Process

This document describes this project's typical process for posting, triaging,
and responding to bug reports.

The main purpose of this documentation is to help contributors know what they
might expect when they open an bug report. However, this is intentionally a
primarily-human-driven process where maintainers aim to be pragmatic and adjust
as needed when something doesn't fit the process well. This is not a set of
rules that we expect to follow unquestioningly in all cases.

🐞 **To share a bug report,
[open a GitHub issue](https://github.com/opentofu/opentofu/issues/new?assignees=&labels=bug%2Cpending-decision&projects=&template=bug_report.yml).**
We don't expect everyone who opens a bug report to read the rest of this
document, but the following text includes some tips that might help us to act
on your bug report more quickly.

## What is a "bug" anyway?

For the sake of this document, "bug" mainly describes a situation where OpenTofu
is behaving in a different way than it was designed to behave, and thus the
typical way to resolve a bug is to change the implementation to match the
originally-intended design, rather than to change the design.

However, there is often some ambiguity about what exactly the design intention
was, and that's particularly true for features that OpenTofu inherited from its
predecessor where the current maintainers were not directly involved in the
design process. In such cases we will need to make a judgement call about
whether something was intended behavior or not.

We distinguish "bug" from "enhancement" mainly because each requires a different
kind of response. Whereas addressing a bug is primarily about changing the
implementation to match a pre-existing design intention, an enhancement request
asks us to consider changing the design to support a new use-case or new
workflow. Enhancement requests typically have the extra step of deciding exactly
what design change to make, which requires following a different process.

## The Fast Path

The "fast path" is intended to allow us to respond quickly to clear and
unambiguous bug reports without getting so bogged down in discussion and
consensus-building.

The fast path can apply to any bug where all of the following are true:

- The original report contains enough information for a maintainer to reproduce
  the described behavior on their own computer, and a maintainer has
  successfully done so.
- The observed behavior clearly contradicts a statement made in the OpenTofu
  documentation, or in other documentation-like sources like the `-help` output
  of a command or a previously-accepted RFC whose proposal hasn't been
  superseded by a later RFC.
- There is a relatively-obvious, low-risk way to change the implementation to
  match what the documentation described, without affecting the behavior of any
  other features or introducing new capabilities that would be subject to
  OpenTofu's compatibility promises.

For any bug report that is eligible for the fast path, any individual OpenTofu
maintainer can post a comment describing exactly what steps they took to
reproduce the described behavior, what documentation (or similar) they are
citing to justify it not matching the design intention, and the specific change
they propose to fix it. They can then immediately mark the bug report as
"accepted" without waiting for the usual triage and consensus process, and
optionally prepare a pull request implementing their proposed fix.

This is therefore a asynchronous process that allows a single maintainer to
act completely autonomously aside from the code review step. The code review
step still provides an opportunity for other maintainers to ask for the issue to
return to the longer path for deeper discussion when appropriate, but we prefer
to bias toward fast acceptance whenever things seem uncontroversial.

As a bug _reporter_, you can help a bug to be handled quickly by doing your best
to provide the information that could justify fast path eligibility as described
above in your original bug report text, in which case a maintainer need only
confirm that they were able to follow your reproduction steps and that any
proposed fix is self-contained and low-risk enough.

## The Longer Path

For any bug report that is _not_ eligible for the fast path, maintainers and
other participants need to collaborate in comments on the issue to try to answer
at least the following questions:

- Can anyone other than the original reporter reliably reproduce the described
  behavior? If not, can we explain why? (e.g. perhaps it requires running on
  a specific network where OpenTofu would be interacting with some unusual
  network equipment.)
- Does the described behavior seem to match the design intention for the
  relevant features? If so, should we transform the report into an enhancement
  request to discuss changing OpenTofu's design, or should we close the issue?
- Is a localized fix for the bug possible, or does something else in the broader
  system need to change too?
- Is there any way to work around the problem in the meantime, using the current
  OpenTofu implementation unmodified?

Hopefully, discussion on the issue will introduce enough new information to make
the issue belatedly eligible for the fast path, in which case we can proceed as
in the previous section.

For all other cases, the maintainers will discuss the situation in periodic
synchronous meetings and try to find consensus for what action to take. The
maintainers might either find consensus immediately, or they might identify some
additional questions to ask to gather more information in order to help make a
decision. In either case, a representative of the maintainer team will post
a comment on the issue describing the result of that discussion.

In hopefully-unusual cases we may not be able to make a decision at all for some
reason, such as if a bug is not reliably reproducible and we run out of ideas
for what might be causing it. In that case, we will eventually close the issue
with a comment to that effect but remain willing to reopen it if someone is able
to provide new information that might unblock it, in which case the discussion
and consensus process continues with that new information.

## Once a Bug Report is "Accepted"

Whether it happens asynchronously by fast-path or it requires more extensive
discussion, the intended goal is for a bug report issue to eventually be marked
as "accepted", using a GitHub issue label.

Once a bug report reaches that state, the maintainer that marked it as such
will leave a comment describing what was accepted -- that is, what specific
solutions the maintainers have decided to accept -- and pin that comment to
make it more visible.

For accepted issues we welcome code contributions from the community as
described in [Contributing to OpenTofu](CONTRIBUTING.md), unless the acceptance
comment stated otherwise. Thanks!

## When a Bug Report Becomes an Enhancement Request

Sometimes we will find that the behavior described in a bug report matches what
was documented, in which case we will typically try to reinterpret the bug
report as an enhancement request to meet a new use-case that OpenTofu was not
previously designed to support well.

The full process for enhancement requests its beyond the scope of this document,
but the main thing to know is that when maintainers switch into this mode the
type of discussion is likely to switch to understanding more about what the
requester was trying to achieve and considering various different possible
solutions to meet that goal so that the maintainers can make design tradeoffs.

Reclassifying a bug report issue into an enhancement request issue does not
imply a change in the priority of addressing it, but instead just recognizes
that a different type of work is needed: product research and technical design,
rather than simply code debugging and repair.
