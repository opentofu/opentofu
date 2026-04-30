# Collecting Data on Usage of Various OpenTofu Features

> [!NOTE]  
> **Nothing in this proposal involves OpenTofu silently sending data about usage
> to anywhere.**
>
> This proposal is focused on establishing a way for OpenTofu to optionally
> produce usage data that could _separately_ be sent somewhere else for
> analysis, but OpenTofu itself will not send that data anywhere except local
> disk, after which it'd be up to surrounding automation outside of our control
> to send it somewhere else if desired.

The OpenTofu project has so far relied primarily on direct feedback from
end-users to shape future work, and that is effective but mostly limits our
available signal to the following:

- OpenTofu did not behave as documented.
- OpenTofu's current features were not sufficient to meet someone's needs.

When deciding how to respond to this sort of feedback, we often find ourselves
needing to make tradeoffs about what impact choices we are making might have
on folks who we're not hearing from _at all_, because OpenTofu is already
working for them. We rely on hunches about how folks are using OpenTofu based
on anecdotes shared with us informally and on what sorts of questions we see
folks asking in online forums where broad discussion about OpenTofu usage
happens.

That has always been challenging due to the information being incomplete and
unstructured, but we now face a new challenge: the availability of LLM-based
search tools has caused a sharp dropoff of people discussing their usage of
software like OpenTofu with their peers on the open internet, and so even the
weak signal we previously had is now severely diminished.

This proposal aims to strike a compromise that would give the OpenTofu project
access to partial, broad, non-identifying data about usage of various OpenTofu
features while staying true to our principles of never directly collecting
personal data without consent and respecting the sensitive nature of data and
metadata about production infrastructure.

## Overall Model

This proposal imagines there being three main stakeholders:

- **End-user:** Chooses which OpenTofu features to use in which combinations,
  representing those decisions as source code in the OpenTofu language and
  other related languages.
- **Orchestrator:** Runs OpenTofu on behalf of an end-user and optionally
  collects feature usage information in a data store they control.

    The decision of whether to collect usage data and where to archive it
    is an agreement between the end-user and their chosen orchestrator.

    This proposal is written under the assumption that the so-called "TACOS"
    would be the main example of this role, working with their customers
    (as "end-user" stakeholders) to determine whether it's acceptable to collect
    usage data as a side-effect of OpenTofu plan/apply rounds running on their
    infrastructure and also whether it's acceptable to share that usage data
    publicly, vs. retaining it only for that vendor's internal use.
- **The OpenTofu Project**: Accepts data submitted by specific orchestrators via
  a pre-arranged process, and incorporates that data into a publically-visible
  dataset.

    The OpenTofu project would provide, as part of OpenTofu CLI, a mechanism to
    capture usage data in a format suitable for loading into a columnar data
    store and save it into a local file. The OpenTofu project therefore
    determines the format and nature of the data, but only obtains the resulting
    data files indirectly from orchestrators when an orchestrator and end-user
    together deem that appropriate.

    Although the project's primary interest is in using this data to inform
    future technical design decisions, the non-identifying data would be made
    publically available for other uses as a general resource for the ecosystem.
    For example, those developing other software in the same general space as
    OpenTofu could potentially adapt this data to help decide how to change
    similar features in their own products.

We want to reinforce that no part of this proposal calls for OpenTofu CLI to
directly "phone home" and send data to anywhere except an explicitly-designated
file on local disk. Whether and how that data file reaches the OpenTofu project
is negotiated separately through agreement between the three stakeholders.

## An Orchestrator's Perspective

From the perspective of an orchestrator, the `tofu plan` command would begin
offering a new option `-usage-data-into=FILENAME` which, when specified, would
cause OpenTofu to gather usage information during plan phase execution and then
write it into the given file once the planning phase is complete.

The generated file would be in [Apache Parquet](https://parquet.apache.org/)
format, making it suitable for uploading directly into a data warehouse without
any intermediate transformation steps. The orchestrator should just capture that
file verbatim and save it somewhere for later use. The exact data captured in
these data files will vary over time as new features are added to OpenTofu, but
the OpenTofu Project will document the schema in a public place so that all
stakeholders can understand what sort of data is being captured and know how
to query these datafiles to confirm exactly what was recorded.

Orchestrators that wish to provide some or all of the data they gather to the
OpenTofu Project to be part of the public dataset would periodically submit
these Parquet data files to a pre-arranged HTTPS endpoint, with each file
accompanied by the following metadata:

- Which orchestrator the data was collected by. This is just in case a specific
  orchestrator finds that some of the data they submitted needs to be retracted
  for some reason, such as due to having been collected incorrectly, so that
  we can distinguish the affected data files.

  (The request must also contain credentials to authenticate that the request is
  truly being made on behalf of the identified orchestrator.)

- The date (UTC) when the data was captured. This coarse time information is
  just to allow us to observe broad changes in usage over time and potentially
  correlate observed changes with outside events.

This proposal does not yet nail down the exact details of this submission
protocol, but we intend for it to be relatively lightweight and easy to
implement with readily-available software. We will choose a submission format
that allows the client to efficiently upload many small files together in a
single request in a streaming fashion that doesn't require buffering entire
files into memory and allows treating each file as an opaque blob. The accepter
will perform basic validation that the submitted Parquet files all have the
expected top-level schema, but will skip accessing and validating the data
itself.

Our expected usage pattern is for an orchestrator to gather up these data files
in storage they control first, and then submit them in batches (e.g. daily,
weekly, or monthly) and probably also delay submitting data so that there's
time to redact or retract certain data after the fact if an orchestrator learns
that they've captured something that should not be submitted to the public
dataset. However, this scheduling policy is ultimately up to the orchestrator
to decide, based on whatever agreements they've made with the end-users they
are submitting data on behalf of.

The only hard constraint is that we would reject any data files whose
date is before the beginning of the previous calendar month so that we can
aggregate historical raw data into larger files for distribution of the public
dataset and for more efficient querying of that dataset. This cutoff means that
we can consider historical time periods to be frozen and avoid constantly
rebuilding those historical aggregate data files.

> [!NOTE]
> **Why gather data during the planning phase?**
>
> This proposal calls for usage data collection to happen as a side-effect of
> the planning phase. The two main alternatives to that would've been:
>
> - Add a completely separate command that can be run independently of planning
>   to gather data, or integrate it into another similar inspection command like
>   `tofu show`.
>
>    One advantage of this approach is that it would be possible to analyze
>    a configuration in contexts other than actually planning against it, such
>    as when a new module version is detected by the OpenTofu module registry.
>
>    Integrating it into the plan phase makes it relatively straightforward to
>    introduce into an existing plan/apply automation without introducing any
>    new steps: OpenTofu just does whatever it would've normally done but also
>    captures some information about it, in similar vein to our existing
>    OpenTelemetry tracing support (but for aggregate, non-identifying data
>    rather than detailed, configuration-specific data.)
>
> - Integrate it into some other command that an OpenTofu automation system
>   would also be running already, like `tofu init` or `tofu apply`.
>
>     We could potentially extend in that way later, but starting with just
>     planning seems like a "sweet spot" where we have a good amount of
>     context available -- we can take into account content of prior state
>     rather than just configuration, for example -- and where we can
>     potentially gather partial data even when an error occurs.
>
> In a later proposal we could discuss the specific idea of gathering similar
> data about module packages published to our public registry, but data gathered
> in that way would have a different "grain" than the per-round data this
> proposal is aiming to gather and so would likely need to be differentiated in
> the data warehouse anyway, so it would make sense to have a different
> mechanism for gathering that per-module-package-version data.
>
> Note that this option is proposed only for the `tofu plan` command
> _even though_ some other commands run a planning phase as part of their work.
> This is because this mechanism is intended for use primarily by automated
> pipelines like those implemented by TACOS where the plan and apply phases
> are always separated, and not for local interactive CLI use where there'd be
> nowhere for the resulting data to go. We could also revisit this decision
> later by introducing a more general way to enable usage data collection if
> we find a need for it, but this first iteration is focused only on this one
> usage pattern for simplicity's sake.

## Per-run Usage Data Format

Parquet files represent columnar data with a variety of different data types.
Conceptually we can think of these files as representing a series of rows, and
in our case each row will represent a single "feature" from the configuration
or from other context OpenTofu considers during the planning phase.

> [!NOTE]
> **Why Parquet?**
>
> Apache Parquet strikes a good compromise of allowing us to represent
> structured data of various kinds in a compact form that can be queried
> directly by various data warehouse systems such as Presto and hosted systems
> like Google BigQuery or Amazon Athena.
>
> As a compact binary format it does have the notable drawback of not being easy
> to inspect by humans without specialized tools. However, there _are_
> readily-available tools like
> [parquet-tools](https://pypi.org/project/parquet-tools/) for inspecting
> individual files should someone wish to directly inspect a data file to
> evaluate whether it contains data that is suitable to share.

We will typically be using the public dataset to compute broad aggregates
grouped by time period, and so our data format will prioritize first
partitioning by the date when each object was created to limit how many
individual blobs we need to scan, and then primarily filtering by the type
of feature we're interested in. For more fine-grain details about how a feature
is being used we will prioritize flexibility of evolving the datapoints we're
collecting over time over efficiency of scanning over records within a file.

With that in mind then, the raw data format produced by OpenTofu CLI has a
very simple, flexible schema:

| Column Name | Type | Description |
|--|--|--|
| `feature_type` | `String` | One of a predefined set of strings that defines what kind of feature each row is describing, and thus which object type is expected in the `details` column. |
| `details` | `Variant` | Various details about the feature, using a different object type for each distinct `feature_type`. |

Using a `Variant` field here prioritizes flexibility over storage efficiency,
so that we can freely extend the set of feature types and the object type
used for each feature type in future versions while keeping the static schema
consistent. Each `details` value will include a redundant copy of the
small-integer indices of the object type corresponding to the row's feature
type, but each distinct name string will be stored only once in each file.

The storage for `details` will be optimized further later in the backend data
processing pipeline once the many small files are aggregated together and so
[Variant Shredding](https://parquet.apache.org/docs/file-format/types/variantshredding/)
should be a productive optimization in those derived aggregate files, but the
initial raw files will typically be too small for such optimizations to be
particularly productive.

Each significant new feature we add to OpenTofu after accepting this proposal
should discuss what it would contribute to this data format: does it introduce
an entirely new feature? If so, what detail keys and value types do instances
of that feature have? Does it instead just add new details to an existing
feature?

The appendix [Retroactive Feature Type Definitions for Existing Features](#retroactive-feature-type-definitions-for-existing-features)
provides some feature type definitions we could potentially include in an
initial implementation of usage data generation to describe various language
features that predate this proposal.

Future RFCs and design documents that modify the usage data format are
responsible for considering and documenting exactly what data is to be tracked,
how exactly we're expecting to make use of that data (to justify the cost of
storing it and justify the schema design decisions for it), and how the chosen
storage format avoids including identifying information.

> [!NOTE]
> The `Variant` data type in Parquet is, at the time of writing, a somewhat
> "bleeding-edge" feature that is not yet well supported across the ecosystem.
>
> This proposal is currently betting that the ecosystem will catch up to
> supporting this by the time we've implemented the proposed system and gathered
> enough data for it to be useful. At the time of writing this note, at least
> some existing hosted "data lakehouse" systems have at least experimental
> support for that new feature, including Amazon S3 Tables.
>
> The Go module [`github.com/parquet-go/parquet-go`] already has support for
> generating Parquet files containing variant-typed columns, so we have at least
> one option for generating raw data files using the proposed schema.

## Public Dataset Format

The OpenTofu Project will publish the public dataset of usage information in
a series of static Parquet data files that each contain the full data collected
in a single calendar month. At any given time the latest available dataset will
be two months old, to allow for late-arriving submissions from aggregators. Once
a monthly dataset is published we will not modify it unless we become aware of a
serious problem with it.

These files will be provided only as static data snapshots. The OpenTofu Project
**will not** provide any infrastructure for third-parties to directly query that
data, so anyone who wishes to run raw queries against the data will need to load
it into a query engine of their choice run at their own expense. OpenTofu
maintainers will have access to a data lake containing these per-month data
files, funded by the sponsoring organizations in a manner arranged by the
Technical Steering Committee, and will share relevant aggregates from the data
as part of justifying proposals in RFCs and other design documents so that
readers can better understand the reasoning without having to re-run the same
queries directly against raw data.

Each whole-month data file shall contain a table with the following schema:

| Column Name | Type | Description |
|--|--|--|
| `run_id` | `UUID` | A meaningless identifier that is generated for each distinct raw data file that has been aggregated. |
| `run_date` | `Date` | The date that the file this record came from was generated on, according to the information provided by the orchestrator. |
| `feature_type` | `String` | One of a predefined set of strings that defines what kind of feature each row is describing, and thus which object type is expected in the `details` column. |
| `details` | `Variant` | Various details about the feature, using a different object type for each distinct `feature_type`. |

The `feature_type` and `details` columns exactly match those from the per-run
raw data format described in the previous section. The additional `run_id`
column allows recognizing which sets of rows originated from the same source
per-run file, to allow writing queries that ask questions like
"how many runs used (feature of interest) at least once?".

Note that because the exact object type used for each `feature_type` is expected
to vary over time in different versions of OpenTofu, these whole-month data
snapshots will have a mixture of different object types associated with each
`feature_type`, and so those querying the data must be prepared to handle those
differences. As far as possible we will evolve the details schema for each
feature type in a way that avoids breaking queries built against earlier
versions of the schema, but only to an extent that doesn't make the data
misleading by reporting something inaccurately or incorrectly. Those working
with the data must be prepared to adapt their queries for changes to the schema
over time as OpenTofu continues to evolve.

The `run_id` is generated pseudorandomly for each distinct file, so it doesn't
provide any direct information about which source file the records came from.
The `run_date` is identical for all records with the same `run_id`; this is
denormalized just for simplicity's sake, since the date is here primarily for
filtering or aggregation purposes.

> [!NOTE]
> The OpenTofu Project may choose to also offer some high-level web-based
> dashboards that summarize different aspects of the data over time using
> smaller derived datasets, if we discover certain questions that are an ongoing
> area of interest for the community.
>
> However, we're intentionally not committing to that here both because we won't
> know what is in that category until we get some experience working with the
> data and because we'll need to do a cost/benefit analysis to make sure we're
> not spending our limited resources on something that has only marginal
> benefit. Offering static monthly rollups for folks to load into their own
> query engines is a good place to start because the datasets behind any summary
> dashboards would presumably be materialized views derived from those static
> rollups anyway.

### Proxy IDs for Identifying Data

There are some situations where, although we are not interested in specific
identifying data values, we _are_ potentially interested in their uniqueness
within a specific run.

For example, we have no interest in capturing the specific source addresses used
for calling remote modules, but it _is_ potentially interesting to know if
many module calls in the configuration refer to the same source address or to
different subdirectory paths in the same module package, because that's
reflective of whether organizations prefer "monorepo-style" patterns over many
small repositories, and other similar decisions that affect the behavior and
performance of the module installer.

The same can be said for specific provider source addresses: we don't want to
know exactly which providers you are using (some of them may be in-house and
therefore unique to an organization) but knowing whether a pair of resources
in the configuration rely on the same provider or not -- particularly across
different modules in a configuration -- can give some insight into how features
like default provider configurations and inheritance of providers between
modules are likely to be used in practice.

To accommodate this, our feature data generation code will include a mechanism
where we can trade a potentially-identifying string for a
pseudorandomly-selected UUID called a _proxy ID_ that uniquely identifies
that string without actually disclosing it. Each distinct key in the details for
each distinct feature type has its own distinct pool of these identifiers so
that we won't disclose when two different properties coincidentally have the
same string value describing different concepts. For example: the UUID for a
module source address will never match the UUID for a provider source address in
the same run even if the source strings happened to coincidentally contain the
same sequence of characters.

This will allow queries such as counting the number of distinct module source
addresses used in a configuration vs. the total number of module calls in that
configuration, without actually disclosing what those source address strings
were.

## Implementation Inside OpenTofu

The problem of collecting usage data has a similar shape to the problem of
generating execution traces with OpenTelemetry: it's a cross-cutting concern
where many different parts of the system are collaborating to assemble a single
central data structure. The key differences compared to our OpenTelemetry
tracing implementation are:

- Usage data is a flat list of features rather than a tree of trace spans.
- If OpenTofu happens to evaluate the same feature multiple times during its
  execution for any reason then we should still report it only once for usage
  data, whereas for trace spans we _want_ to see if we're performing the same
  work multiple times whenever that work is significant enough to be worth
  mentioning in traces.
- We generate OpenTelemetry trace data primarily for direct human consumption
  in trace views while debugging and don't guarantee any consistency between
  OpenTofu versions, but feature usage data is intended for use primarily in
  aggregate across many reports generated by different versions of OpenTofu
  and so the data format we use must remain relatively consistent over time.

Therefore, as with telemetry traces the main way we will orchestrate usage data
collection throughout the planning phase is by putting a feature data reporting
object inside the `context.Context` passed into the planning phase whenever
usage data collection is enabled. Different subsystems of the language runtime
can then attempt to obtain that object and send feature usage reports to it
as appropriate, as part of whatever work those subsystems were already doing.

The API of that reporting object is the main place we see the differences from
the OpenTelemetry tracing approach:

```go
package usagedata

// Collector is the type of the object that is passed around in
// [context.Context] during any planning phase where usage data collection is
// enabled.
type Collector struct {
    // ...
}

// CollectorFromContext returns the [Collector] from the given context, or nil
// if there is no active usage data collector.
//
// Callers should typically use this as follows:
//
//     if coll := usagedata.CollectorFromContext(ctx); coll != nil {
//         feature := potentiallyExpensiveWorkToBuildFeature()
//         coll.Report(feature)
//     }
func CollectorFromContext(ctx context.Context) *Collector {
    // ...
}

// Report adds the given feature to the active feature usage report, potentially
// replacing a previous report of the same feature as uniquely identified by
// the combination of FeatureType and FeatureKey from the given object.
//
// It's therefore functionally okay (though potentially performance-wasteful)
// to re-report the same feature multiple times, which can potentially occur
// for features reported in shared codepaths that can get called repeatedly
// during the language runtime's configuration analysis and evaluation.
//
// Calling Report on a nil collector is a safe no-op, but callers that need
// to do non-trivial work to build the [Feature] value should perform that work
// only after checking that the collector they are using is non-nil.
func (c *Collector) Report(feature Feature) {
    // ...
}

// ProxyID either allocates a new proxy ID for the given value under the given
// feature type and details key, or returns the one allocated by a previous call
// with the same arguments.
//
// The value can be of any comparable type, and equality of that type must
// represent two values being equivalent for proxy ID allocation purposes.
func (c *Collector) ProxyID(featureType string, detailsKey string, value any) uuid.UUID {
    // ...
}

// Feature is implemented by any type that represents a type of feature that
// can be recorded in a usage data report.
//
// (This is shown here with all of its methods exported, but all implementations
// of this type are expected to be in the same package so these might actually
// be unexported in the final implementation to reinforce that. Final API
// design for this internal package to be determined during the implementation
// phase if this proposal is otherwise accepted.)
type Feature interface {
    // FeatureType returns the string that should be written into the
    // "feature_type" field of the resulting row in the usage data table.
    FeatureType() string

    // FeatureKey returns a comparable value that uniquely identifies this
    // particular feature within all features whose FeatureType returns the
    // same value.
    //
    // This is used only internally for coalescing duplicate reports about
    // the same feature, and never included in the resulting dataset. It's
    // likely to be some representation of the address of the feature
    // being described, which would not be appropriate to include in the
    // final dataset because it'd contain identifying information.
    FeatureKey() any

    // FeatureDetailsParquetVariant returns an encoded version of the
    // feature details in the Parquet [Variant Binary Encoding Format].
    // The result is ready to be written directly into the "details" field
    // of the resulting row in the usage data table.
    //
    // If a particular feature uses proxy IDs in place of identifying strings
    // then this function is responsible for requesting the proxy IDs from the
    // given collector using [Collector.ProxyID].
    //
    // [Variant Binary Encoding Format]: https://parquet.apache.org/docs/file-format/types/variantencoding/#variant-binary-encoding
    FeatureDetailsParquetVariant(*Collector) []byte
}
```

The intention of this API is that each distinct feature type would have a
corresponding struct type in this package that implements the `Feature`
interface. This means that callers elsewhere in the codebase can focus just on
building values of those types without having to directly interact with the
Parquet representation of the features; only `package usagedata` should actually
directly interact with a Parquet encoding library, and the set of possible
`feature_type` values is centrally controlled by the implementations of those
types in this package.

A [Collector] just gathers up all of the reported features in memory. Once
the planning phase is complete, the CLI layer (currently: the local backend's
operation implementations) is responsible for requesting that the final Parquet
representation of the entire data table be written to the output file selected
in the `-usage-data-into=FILENAME` command line option.

## Notable Limitations and Biases

### Customers of large platforms only

Because the proposed process relies on relationships between the OpenTofu
Project and each of the "orchestrator" stakeholders individually, the resulting
dataset will be inherently biased to only include:

- Those who choose to run OpenTofu as part of an OpenTofu-specific hosted
  platform such as a "TACOS", as opposed to running in their own custom internal
  pipelines.

    This might therefore indirectly bias towards only larger companies that have
    the resources to purchase OpenTofu-specific SaaS products, and miss anything
    that tends to be used only by smaller-scale participants that prefer to
    rely on general-purpose execution systems like GitHub Actions.

- Those who are willing to cooperate with their chosen orchestrator to collect
  and share the non-identifying data.

    This is likely to exclude anyone using OpenTofu in a sensitive setting
    where even non-identifying data is deemed too high-risk to share, or those
    who have concerns about "fingerprinting" based on particularly-unusual
    usage patterns.

This proposal is making the assumption that data with known biases we can take
into account is better than no data at all. Of course, it remains to be seen
whether we will hold ourselves to keeping those biases in mind when evaluating
the data.

## Retroactive Feature Type Definitions for Existing Features

Although we expect that RFCs and other design documents written after the
acceptance of this proposal would be responsible for defining their own
extensions to the usage data schema, there are of course various features
already in OpenTofu that we could potentially report on.

This section therefore defines some basic initial feature reporting schemas
for existing features, both as some examples of what we could implement in a
first implementation round of this proposal and to illustrate what it might
look like to propose additions to this schema for newer features in later RFCs.

This initial set focuses mainly on language features, but also includes some
information about the interactions between language features and the current
evaluation context.

Some general design guidelines:
- For integer values whose size is unimportant, use `Int64` to allow a wide
  range of values.
  
    Parquet stores a column's values using the smallest possible size for the
    largest value in it regardless of the schema-defined type. This is unlikely
    to be productive in the small per-run files we start with, but can become
    more productive in our larger monthly rollups for keys that are split out
    using variant shredding.
- Boolean values are represented using `Bool`, even though that means that in
  some query engines explicit casting would be required to e.g. count the
  number of rows with a certain property using an aggregate "sum" function.

    Once our data pipeline combines many individual per-run tables into a
    monthly rollup, boolean detail keys that appear frequently in the dataset
    can potentially become eligible for variant shredding, allowing them to be
    stored compactly as a linear bitmask.
- For detail keys that are present for multiple feature types and have a similar
  meaning across those feature types, use the same representation across the
  different feature types to make it more likely for those nested fields to
  be considered for variant shredding.

    For example, both input variables and output values can be declared as
    "ephemeral", with a similar meaning in each case, so we should use the
    same representation of that property across both of those feature types.
- To represent types or type constraints from the OpenTofu language, use a
  compact (single-line, no spaces) version of the normal type constraint syntax,
  with the argument of any `object` or `tuple` constructor replaced by just
  `...`.

    This compromise is intended to let us report on the broad top-level kind
    of type being used while still keeping relatively low cardinality and,
    in particular, avoiding disclosing specific attribute names from object
    types that are likely to be identifying.

    Some examples encodings of types or type constraints:
    - `string`
    - `object(...)`
    - `map(object(...))`
    - `list(object(...))`
    - `tuple(...)`
    - `any`
    - `map(any)`

    If a future version of the OpenTofu language includes support for
    user-defined types or type aliases, those _must not_ be recognizable in
    the type constraint representation. Later design of such a feature should
    specify whether those get serialized by inlining the definition of the
    user-defined type (making the user-defined type indistinguishable from
    its underlying type) or by using some special placeholder to explicitly
    mark that something was redacted.

### Input Variables

Example questions we could ask about usage of input variables:

- How often are input variables declared as complex types like
  `map(object(...))` vs primitive types like `string`?
- How often are ephemeral input variables being used in root modules, or in
  nested modules? Same for "sensitive".
- Do authors tend to write descriptions for their input variables?
- How often is input variable validation used?
- How many modules are using the feature for marking an input variable as being
  deprecated? How many module calls are still assigning values to input
  variables that are marked as deprecated?

Initial details schema for feature type `variable`, which is emitted for each
instance of each distinct `variable` block in the configuration:

| Key | Type | Description |
|---|---|---|
| `nesting` | `Int64` | The number of levels of nesting in the module path leading to the declaration, starting at 0 for variables declared in the root module. |
| `type` | `String` | Normalized and simplified string representation of the type constraint expression, or null if there is no `type` argument at all. |
| `ephemeral` | `Bool` | True only if the input variable is declared as being "ephemeral", otherwise false. |
| `sensitive` | `Bool` | True only if the input variable is declared as being "sensitive", otherwise false. |
| `described` | `Bool` | True only if the `description` argument is present, regardless of what string is assigned to it, otherwise false. |
| `deprecated` | `Bool` | True only if the `deprecated` argument is present, regardless of what string is assigned to it, otherwise false. |
| `postconditions` | `Int64` | The number of `validation` blocks present in the declaration. Zero if no such blocks are present. This is named "postconditions" because input variable validations behave essentially as postconditions for the input variable's definition, and so this can be stored and queried together with the similar fields describing resource instances. |

Initial details schema for feature time `variable_def`, which is emitted for
each input variable definition inside each instance of each distinct `module`
block in the configuration:

| Key | Type | Description |
|---|---|---|
| `nesting` | `Int64` | Matches the "nesting" field of the `variable` feature representing the declaration of the variable that this definition assigns to. (This is one greater than the nesting level of the call itself, so definitions for a `module` block in the root module would have this set to one. Root module input variables defined outside of the configuration by planning options have nesting level zero.) |
| `type` | `String` | Normalized and simplified string representation of the dynamic type of the assigned value. |
| `type_converted` | `Bool` | True if the dynamic type in `type` does not exactly match the type constraint of the associated variable but conversion was possible, or false if no conversion was needed. Null if type conversion failed, since in that case the given value was not successfully assigned at all. |
| `deprecated` | `Bool` | True if the variable that is being defined was declared as being deprecated, maching the `deprecated` field of the associated `variable` feature. |

Note that this structure does not allow direct correlation between a
`variable_def` and its associated `variable`. We consider the declaration and
definition to be two separate features that we report on separately, because
one tells us information about the caller of a module while the other tells
us information about that module itself, and those may be different parties
that prefer to use different parts of the language. It may still be possible
to _approximately_ correlate definitions with declarations by comparing the
type constraints and nesting levels, but supporting that is not a goal of
this schema.

("Each instance" of an input variable means that when a module call has `count`
or `for_each` specified these features would get re-reported for each instance
of the module call. This initial proposed schema does not include any way to
distinguish between multiple `module` blocks calling the same module address
vs. a single `module` block with multiple instances.)

### Output Values

Example questions we could ask about usage of output values?

- How often are output values assigned complex types like `map(object(...))` vs
  primitive types like `string`?
- How often are ephemeral output values being used in root modules, or in
  nested modules? Same for "sensitive".
- Do authors tend to write descriptions for their output values?
- How often are output value preconditions used?
- How many modules are using the feature for marking an output value as being
  deprecated?

Initial details schema for feature type `output`, which is emitted for each
instance of each distinct `output` block in the configuration:

| Key | Type | Description |
|---|---|---|
| `nesting` | `Int64` | The number of levels of nesting in the module path leading to the declaration, starting at 0 for variables declared in the root module. |
| `type` | `String` | Normalized and simplified string representation of the dynamic type of the assigned value. |
| `ephemeral` | `Bool` | True only if the output value is declared as being "ephemeral", otherwise false. |
| `sensitive` | `Bool` | True only if the output value is declared as being "sensitive", otherwise false. |
| `described` | `Bool` | True only if the `description` argument is present, regardless of what string is assigned to it, otherwise false. |
| `deprecated` | `Bool` | True only if the `deprecated` argument is present, regardless of what string is assigned to it, otherwise false. |
| `preconditions` | `Int64` | The number of `precondition` blocks present in the declaration. |

There is not currently any feature type defined for a _reference to_ an output
value, because current the OpenTofu language runtime can't detect that precisely
enough to produce complete data.

("Each instance" of an output value means that when a module call has `count`
or `for_each` specified these features would get re-reported for each instance
of the module call. This initial proposed schema does not include any way to
distinguish between multiple `module` blocks calling the same module address
vs. a single `module` block with multiple instances.)
