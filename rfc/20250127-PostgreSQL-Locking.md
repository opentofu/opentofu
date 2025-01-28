# PostgreSQL Backend: Schema Based Locking (Workspace Creation)

Issue: https://github.com/opentofu/opentofu/issues/2218 <!-- Ideally, this issue will have the "needs-rfc" label added by the Core Team during triage -->

PostgreSQL (pg) backend allows users to set a schema to be used for state storage. It makes it possible
to reuse the same database with different schemas per OpenTofu "setup" (e.g. different projects, dev/stage/prod
environments, etc.). Those configuration setups are isolated and must be applied without ties to each other,
even though they are operating in the same database.

Currently, workspace creation locking is database scoped with a shared static ID, which disallows creation of
workspaces (including the `default` one) in parallel if the database is reused across multiple OpenTofu setups.

## Proposed Solution

Historically, `pg` backend used transactions to handle locking, which comes with its own problems due to the
need of transactions rollback. This approach doesn't align with the locking design of OpenTofu, so session
based locking via `pg_advisory_lock` fits better (which is the second version of `pg` locking implementation).

Also, `pg_advisory_lock` is database scoped, so `pg` backend needs to handle collisions between different state
IDs, even if they come from different OpenTofu setups (i.e. different schemas inside the same database). This is
handled via single sequence in a shared schema (`public`).

However, workspace creation locking uses a static ID (`-1`), which makes any workspace creation blocking, even if
those workspaces are isolated (i.e. from different schemas). This is the problem, which needs to be fixed. Proposed
solution is to change static `-1` ID to schema-based ID to isolate workspace creation locking to a specific schema.

### User Documentation

We don't need to change anything from the user perspective. The only difference is that parallel `tofu apply` calls
for different setups must not fail even if their backends are using the same database (but different schemas).

### Technical Approach

Technically, we need to use schema based value instead of static `-1` ID in calls to `pg_advisory_lock`. We should 
use negative values since positive ones are reserved for state IDs in `pg_advisory_lock`. Proposed solution is to
hash the name of the schema (which is unique per database) and negate the integer value to be used in `pg_advisory_lock`.

This is still prone to collisions and is not going to solve 100% of similar issues, however, this is the easiest available
option. Additional attention needs to be paid to overflows, since `pg_advisory_lock` uses 64-bit signed integers as IDs
and we want to generate a hash, which fits into that range.

Since `pg_advisory_lock` is session scoped we don't need to worry about the migration or breaking changes. Problem may arise
if users run parallel `tofu apply` for the same backend with different versions of OpenTofu (i.e. one run acquires `-1` lock
and the other one acquires the hash of the same schema name). In this scenario, we end up with unsafe database writes.

### Open Questions

* Do we need to put an additional safe guard for unsafe writes described previously? Is the release notes warning enough?

### Future Considerations

We may want to reduce potential hash collisions in the future.

## Potential Alternatives

Alternatively, we can introduce a new table to generate a sequentual ID tied to a specific schema name. This allows us to
not rely on hashes and produce unique IDs for each schema. However, it could introduce more problems with database state.
