# Optimized Refresh and Detection of New Objects

By default, OpenTofu's "plan" phase currently includes a step of requesting the
latest settings for every object tracked in the prior state, to check whether
something has changed in the remote system outside of OpenTofu's workflow.

This extra step is useful because it ensures that OpenTofu is planning against
the true current state of the remote objects rather than a stale snapshot of
what was current at the end of the previous plan/apply round, but the current
approach has some drawbacks too:

- The current provider protocol uses a separate call to refresh each object,
  which means that configurations that include many objects are often slow
  to refresh and may cause API rate limits to be exceeded.

    This tends to cause those with larger configurations to use `-refresh=false`
    to completely disable refreshing, or to use the `-target=...` option
    to work with only small fragments of configuration at a time, both of
    which can cause OpenTofu to be left with an inconsistent view of the
    remote objects, potentially causing problems later.

- Refreshing individual objects doesn't allow OpenTofu to detect entirely new
  objects that might've been created outside of OpenTofu, and so we have a
  separate "import" workflow to deal with those and that is useful only if
  the operator already knows that the additional objects have been created.

This document proposes some changes to OpenTofu's default behavior that should
hopefully strike a better compromise where for most operators the refresh
behavior will be a benefit rather than a burden. It also proposes retaining
something more like the current default behavior as an opt-in, so it will
still be available for those who wish to prioritize having a completely-updated
state, and those folks can still benefit from some of the performance
improvements even when the force full refreshing.

Related issues:

- [Option to skip refreshing resource instances whose configuration hasn't changed since the last apply](https://github.com/opentofu/opentofu/issues/1703)
- [Make Terraliths a Thing of the Past – Enable Scalable Root Modules in OpenTofu](https://github.com/opentofu/opentofu/issues/2860)
- [More granular state storage, locking, and planning](https://github.com/opentofu/opentofu/issues/2662)
- [Auto Import Resources If Possible](github.com/opentofu/opentofu/issues/2321)
- [Ability to have OpenTofu automatically import an existing object if it exists, or create it otherwise](https://github.com/opentofu/opentofu/issues/1760)

## Proposed Solution

There are three main parts to this proposal that could potentially be
implemented separately but that are proposed together because the complement
each other to produce a better overall system:

1. Allow providers to optionally optimize their refresh calls by performing
   many at once in a single request to the remote system, whereever the remote
   API has support for that.

    This requires a provider protocol extension.

2. Introduce a new configuration language feature for describing search queries
   that might discover new remote objects that would be considered to be in
   the management scope of the current configuration.

    This would extend the meaning of "refresh" to also include discovery of
    objects that OpenTofu didn't create, after which the operator can decide
    whether to adopt them into the desired state (by generating configuration)
    or to delete them as unwanted drift.

    The full functionality of this part of the proposal requires a provider
    protocol extension, but partial support is possible with existing provider
    protocol features.

3. Change OpenTofu's default behavior so that it will only refresh objects whose
   resource instance configurations have changed since the most recent
   plan/apply round or which are dependencies of resource instances whose
   configurations have changed.

    This new compromise gives OpenTofu access to up-to-date information about
    the objects involved in an intentional configuration change while allowing
    unrelated objects to remain stale until a future run.

    The current behavior of always refreshing everything would remain available
    as a new planning option useful for e.g. periodic "drift detection" runs,
    and OpenTofu would also continue to refresh everything in the `-refresh-only`
    planning mode where detecting differences is the primary purpose.

    This does not require a provider protocol change, and so would provide
    immediate benefit regardless of which providers are being used.

The following subsections describe each of the items above in more detail.

### Bulk refresh

Today's OpenTofu calls the provider protocol's "read managed resource" operation
separately for each resource instance from the prior state, as part of the
process of planning each resource instance.



### Automatic discovery of new objects

### Refreshing only objects whose configuration has changed

## Open Questions and Alternatives

### Continue refreshing everything by default?

This document has proposed that we change the _default_ behavior of OpenTofu
so that it will refresh only the subset of objects whose configurations have
changed (or that have actions planned for any other reason).

We could also potentially choose to retain the current default and require
those who want the new behavior to explicitly opt in to it.

The proposal to change the default is founded in the assumption that more
operators would want the new behavior than would want to keep the old behavior,
that this behavior change is not significant enough to be considered "breaking",
and that those who need the previous behavior would be able to add the new
planning option relatively easily.

This echoes a tradeoff made for a similar change made long ago to OpenTofu's
predecessor:

Originally the "apply" command performed the plan and apply phase together
immediately without any interactive confirmation prompt, and so anyone who
wanted to review their plan before applying it needed to use the saved plan
workflow, which is pretty inconvenient for those who are running the program
interactively from a shell prompt.

Someone proposed adding a new option to enable an interactive mode for "apply"
which would show the plan and then prompt for confirmation before proceeding.
Subsequent discussion found that actually the interactive approval prompt was
the more commonly-needed mode, and so the interactive prompt was implemented
as the new default behavior and the `-auto-approve` option added for the
minority who wanted the previous behavior.

That appears to have been a good decision in the long run, even though it was
admittedly inconvenient for those who needed to adjust their existing usage
patterns or wrapper scripts at the time. Similarly, I think that partial
refreshing is the better default behavior for most operators, and that the
current full-refresh behavior is more suited to special situations like when
implementing a "drift detection" system which runs periodically with the
explicit goal of finding changes made outside of OpenTofu.
