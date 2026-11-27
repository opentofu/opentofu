# RFC: `run.*` references after `command = plan` run blocks in `tofu test`

Issue: https://github.com/opentofu/opentofu/issues/3754

## Problem

In `tofu test`, a run block with `command = plan` does not make its outputs
available to later run blocks. A later block that references
`run.<name>.<output>` — in its `variables` block or in an `assert` condition —
fails, even though the same reference works after a `command = apply` run.

Reproduced on OpenTofu v1.13.2 with only outputs, no providers:

```hcl
# main.tf
output "name" {
  value = "test-plan"
}

# main.tftest.hcl
run "setup" {
  command = plan
}

run "use_output" {
  command = plan
  variables {
    name = run.setup.name
  }
  module {
    source = "./other"
  }
  assert {
    condition     = output.received_name == "test-plan"
    error_message = "expected received_name to be 'test-plan'"
  }
}
```

Result today:

```
main.tftest.hcl... fail
  run "setup"... pass
  run "use_output"... fail

Error: Unsupported attribute
  on main.tftest.hcl line 7, in run "use_output":
   7:     name = run.setup.name
    ├────────────────
    │ run is object with no attributes
```

This is the behaviour documented as a known limitation in
[#3824](https://github.com/opentofu/opentofu/pull/3824), which added the
documentation note but did not change the behaviour.

A second surface with the same root cause was reported in #3754 on
2026-08-14: `run.<name>` inside an `assert` condition, where the reference
does not resolve and the expression can fall back to root-module scope,
aborting the rest of the file. The use cases that need this are relational
assertions between plan runs: two runs of the same inputs agree, two runs
of different inputs diverge, and a value round-trips through several runs
— claims a literal cannot replace without changing what is asserted.

## Background

The test runner is in `internal/command/test.go`.

- `TestFileRunner.ExecuteTestFile` keeps one `TestFileState` per module key
  in `runner.States`. A state's `Run` is the run that most recently updated
  the tracked state, and its `State` is used both to build the `run` object
  for later runs and as the cleanup configuration at the end of the file.
- `TestFileRunner.ExecuteTestRun` returns `(state, updatedState)`. For a
  `command = plan` run it returns `(state, false)` after evaluating the
  plan (`planCtx.TestContext(...).EvaluateAgainstPlan(run)`), because a
  plan produces no real state.
- `ExecuteTestFile` only writes `runner.States[key].State` and
  `runner.States[key].Run` when `updatedState` is true. A plan run therefore
  never populates the entry.
- `getEvalContextForTest` builds the `run` object only from states whose
  `Run` is non-nil, reading `State.Modules[""].OutputValues`. With no entry
  for a plan run, `run` is an empty object and `run.setup` is an
  unsupported attribute.

`TestFileRunner.Cleanup` already anticipates plan-only entries: a state
with `Run == nil` and an empty state is skipped as "nothing to cleanup".
That is the constraint any fix must preserve — planned outputs must never
be mistaken for real state that needs destroying.

## Proposed Solution

### User Documentation

After this change, `run.<name>.<output>` resolves after a `command = plan`
run exactly where it already resolves after a `command = apply` run:

- in a later run block's `variables` block,
- in a later run block's `assert` conditions,
- in provider configuration transforms that are evaluated with the test
  context, if the maintainers agree that surface is in scope (see Open
  Questions).

The value is the output value from the plan: a known planned value where
the plan knows it, and an unknown value where the plan cannot know it
yet (for example a provider-computed value). An unknown value propagates
as unknown into downstream variables and assertions, following the
existing plan-evaluation semantics, rather than erroring at reference
time.

A later `command = apply` run for the same module key supersedes any
planned outputs recorded for that key: once real state exists, `run.*`
reflects real state, not the earlier plan.

Plan-only entries are never destroyed and never appear in destroy
summaries. If a test file contains only plan runs, cleanup is a no-op,
as it is today.

The documentation limitation added in #3824 is updated to describe this
behaviour instead of the current limitation.

### Technical Approach

Separate the two things `TestFileState` currently couples:

1. **Real state for cleanup.** Unchanged: `runner.States[key].State` and
   `runner.States[key].Run` are only updated when a run actually updates
   state (today, `command = apply`). `Cleanup` continues to use exactly
   this map.
2. **Planned outputs for reference.** New, separate recording — for
   example a `PlannedOutputs` map on `TestFileRunner`, keyed the same way
   as `runner.States` (main state identifier, or module source for
   submodule runs), holding the output values from
   `plan.PlannedState` for each plan run, keyed by run name.

`getEvalContextForTest` then builds the `run` object from real state
where it exists, and from planned outputs where only a plan exists for
that run name. Where both exist for the same run name, real state wins.

Concretely, in `ExecuteTestRun`, the `command = plan` branch already has
`plan.PlannedState` available before it returns `(state, false)`. The
change records `plan.PlannedState`'s root-module output values for this
run into the planned-outputs map before returning, without touching
`runner.States`. `ExecuteTestFile`'s `updatedState` gate is left alone,
so the existing state-serialization workaround
(`simulateStateSerialization`, which exists to drop destroyed
non-root-module outputs from real state) is not applied to planned
values.

Chained plan runs work transitively under this approach: each plan
run's evaluation context already includes previously recorded planned
outputs, so `run C` can reference `run B` (plan), which referenced
`run A` (plan).

### Open Questions

1. **Unknown planned values.** Is passing through `cty` unknown values
   the right semantics, or should referencing an output that is unknown
   after planning be an error at reference time? The proposal is
   pass-through, consistent with plan evaluation, but this needs
   maintainer agreement, including how assertions on unknown values
   behave.
2. **Sensitive outputs.** Planned output values carry the same
   sensitivity marking as applied outputs. Within a single test file,
   is exposing a sensitive planned output to a downstream run's
   variables/assertions acceptable, on the same basis as applied
   outputs? The proposal assumes yes, since it never leaves the test
   file's evaluation context.
3. **Provider configuration scope.** Should planned outputs also be
   visible to `buildEvalContextForProviderConfigTransform`, or only to
   `variables` and `assert`? The proposal includes it for consistency,
   but it widens the surface.
4. **Supersession order.** If a plan run and an apply run share a module
   key but have different run names, should the `run` object contain
   both (each under its own run name), with only same-name apply
   superseding same-name plan? The proposal is per-run-name, not
   per-module-key, supersession.
5. **Expected failures.** A plan run with expected failures already
   validates before returning. Should its planned outputs still be
   recorded for downstream runs, or only on a passing plan? The
   proposal is to record only on a passing plan.
6. **State serialization parity.** Real state goes through
   `simulateStateSerialization` between runs. Do planned outputs need
   an equivalent step (for example, dropping outputs of modules that a
   later plan destroys), or is that a real-state-only concern? The
   proposal says real-state-only, but this should be checked against
   a multi-module plan chain during implementation.

### Future Considerations

- The same separation (reference data vs. cleanup state) would make it
  possible to support `run.*` after other non-applying commands if any
  are added to the test framework later, without re-coupling cleanup.
- If the test framework later supports parallel run blocks, the
  planned-outputs map would need the same ordering discipline as
  `runner.States` today (evaluation order, no automatic dependency
  sorting). That is out of scope here.

## Potential Alternatives

1. **Return `(plannedState, true)` from plan runs and teach `Cleanup`
   to skip plan-only states.** Simpler at the call site, but it
   re-couples reference data and cleanup state in exactly the map the
   bug report identifies as the problem, and risks a plan-only entry
   being treated as destroyable state in every other consumer of
   `runner.States` (interrupt summaries, destroy summaries). Rejected
   in favour of separation.
2. **Fix only the `assert` surface.** The `variables` surface and the
   `assert` surface share the same `run` object construction, so fixing
   one without the other would leave an inconsistent language: the
   same reference would work in one place and fail in another.
   Rejected.
3. **Keep the current behaviour and document it.** This is the status
   quo since #3824. It leaves the relational plan-vs-plan assertions
   in #3754 impossible to write, which is the user-facing gap this
   RFC addresses. Rejected as a resolution, though the documentation
   remains correct until an implementation lands.
