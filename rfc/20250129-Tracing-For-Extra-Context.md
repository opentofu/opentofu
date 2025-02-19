# OpenTelemetry To Provide End Users Extra Context

### Introduction
As OpenTofu grows in adoption and complexity over time, end-users of tofu often find themselves asking “Why is my plan taking so long”. Understanding the inner workings of OpenTofu, particularly when it comes to performance bottlenecks can be an extremely overwhelming task. There are many factors involved that can cause tofu to appear to be slower than expected and the purpose of this RFC is to provide end-users with a tool which they can use to begin pinpointing the cause of such issues much easier.

This RFC proposes enhancing OpenTofu’s support for OpenTelemetry to provide end-users with a clear, high-level view of where time is spent during different phases of an OpenTofu run. By integrating tracing capabilities, users will gain visibility into the processes behind their runs without needing to wade through extensive debug logs or intricate traces. This should empower users to answer critical questions such as:
- How much time is spent refreshing each resource?
- Why does it take so long before applying changes?
- Which steps are the most time-consuming during the initialization phase?

By offering this window into OpenTofu’s internals, we aim to enable users to make more informed decisions, optimize their workflows, identify regressions and ultimately achieve faster, more predictable outcomes with `tofu`.

It is worth noting that OpenTelemetry is already added to the OpenTofu codebase in a very very limited manner, and this RFC is to discuss how we should extend that.

### Past Discussions

- https://github.com/opentofu/opentofu/pull/2028/files - Introduce OpenTelemetry to the OpenTofu codebase [RFC]
- https://github.com/opentofu/opentofu/issues/1519 - OpenTelemetry Support
- https://github.com/opentofu/opentofu/pull/1517 - A draft PR that added OpenTelemetry support to the planning process in OpenTofu as a proof of concept.

- https://terragrunt.gruntwork.io/docs/features/debugging/#opentelemetry-integration - Terragrunt has a similar implementation of OpenTelemetry that we could use as a reference. I also think that it would be amazing to enable the tracing and get traces for both Terragrunt and OpenTofu collected all at once, so you have full visibility end to end as to what is happening.

## Approach
To provide users with actionable insights into the performance of their runs, I propose adding traces to the major entrypoints into the system. Instead of overwhelming the end users with granular details from lower-level operations, the tracing here will focus on key stages of the workflow. These stages should align with the user-facing concepts of `tofu` and add additional context to the already existing logs, allowing people to easily understand where time is spent without requiring deep technical knowledge of the inner-workings of opentofu.

As it is almost impossible to estimate every flow through the application, I propose that we take an iterative approach to adding spans. Targetting what we know is used often first and then expanding on that over time.

### Tracing Flow for Plan + Apply
Below is the proposed flow of traces for a typical `plan` + `apply` operation, with key entrypoints highlighted for the end users.

- **Initialization**
    - **Parsing Configuration**: Trace the time spent parsing the configuration files to build the initial representation of infrastructure.
    - **Checking/Validating Providers**: Ensure that providers are downloaded already or not.
    - **Downloading Providers**: Trace the time taken to both download and validate the checksum of the providers that are downloaded.
- **Schema Fetching and Validation**
    - **Fetching Provider Schemas**: Trace the time spent fetching and validating schemas from providers, especially for dynamic configurations, or for validating provider-defined function calls.
- **Graph Construction**
    - **Total Time**: Trace the time taken to construct the graph
- **Refreshing Resources**
    - **Time Per Item**: Trace the time taken to talk to the provider to refresh each resource states or fetch each data source
- **Change Determination**
    - **Time Per Item**: Time taken to determine what changes need to happen and how for each item in the graph (resources, data sources etc.)
- **Apply Phase**
    - **Applying Changes**: Provide high-level traces for each entity’s apply item, indicating time taken and success/failure per item.

### Other Common Traceable Events
Alongside the actual flow of `tofu` and its execution, I propose that we also trace common events in the system such as communication with backends, time taken to handle state encryption, time taken to read/write files to/from disk.

### Key Benefits of This Approach

- **Actionable**: Users can immediately identify where time is being spent (e.g., slow resource refreshes or downloads).
- **High-Level Insights**: The tracing flow aligns with user-visible operations, avoiding confusion from lower-level details.
- **Complementary**: Tracing works alongside logs, adding timing and context without replacing existing diagnostic tools.

### OpenTelemetry Semantic Conventions
To adhere to existing standards and tooling out there, we should adopt some of the commonly used conventions for our telemetry.
#### **General Conventions**
I propose that we should adopt the general conventions defined by OpenTelemetry [here](https://github.com/open-telemetry/semantic-conventions/blob/5b24784e032ada99312d5a17d6970da434f5ce88/docs/README.md) to describe spans and resources. These conventions ensure that traces are self-explanatory and align with tools like Jaeger, and Grafana.

The list below is not exhaustive and is intended to show that there is a lot of ways that we can introduce great tracing to integrate with other tooling and provide a lot of information.

**Generic Attributes**:
- **`process.command_line`**: Command-line arguments used in the run.
    - Example: `"tofu plan -var-file=vars.tfvars"`
- **`file.path`**: Path of HCL files being parsed.
    - Example: `"/my-infra/main.tf"`
- **`service.name`**: The logical service name.
    - Example: `opentofu`
- **`service.version`**: The version of OpenTofu.
    - Example: `1.10.0`
- **`process.runtime.name`**, **`process.runtime.version`**, **`process.runtime.description`**: Details about the runtime (e.g., Go version).

**Semantic Conventions**
- **`trace.span_kind`**: Specify the role of the span in the operation.
    - `SPAN_KIND_SERVER`: Entry points like `tofu plan` or `tofu apply`.
    - `SPAN_KIND_CLIENT`: Calls to providers or APIs (e.g., fetching provider schemas, refreshing states).
- **`trace.parent_id`**: Links child spans to their parent spans for hierarchy.

## OpenTofu as a cog in the machine

In the context of OpenTofu's integration with OpenTelemetry, it's essential to consider how trace context is propagated **from** external systems like CI/CD pipelines (e.g., Spacelift, Jenkins) or tools like Terragrunt **into** OpenTofu. This ensures that traces remain connected across the entire workflow, providing end-to-end observability.

**Environment Variable Trace Context Propagation:**
The OpenTelemetry Enhancement Proposal [OTEP #258](https://github.com/open-telemetry/oteps/pull/258) introduces a standardized method for context propagation using environment variables. This approach is particularly beneficial for CI/CD systems and other tools that execute OpenTofu as a subprocess, where traditional context propagation methods (like HTTP headers) aren't applicable.

**Implementation in OpenTofu:**
1. **Reading Trace Context from Environment Variables:**
    - When OpenTofu starts, it should check for specific environment variables that carry trace context, as defined by OTEP #258.
    - The primary environment variable for trace context is `TRACEPARENT`, which contains the trace context in a standardized format.
2. **Injecting Trace Context into OpenTofu:**
    - If the `TRACEPARENT` environment variable is present, OpenTofu should extract the trace context from it.
    - OpenTofu can then continue the trace, creating spans that are linked to the originating trace from the CI/CD system or external tool.
3. **Propagating Trace Context to Provider Plugins [ Future Extension ]:**
    - As OpenTofu interacts with provider plugins, it should propagate the trace context to these plugins.
    - This ensures that the entire operation, from the CI/CD system through OpenTofu and down to the provider plugins, is part of a single, cohesive trace.

## Implementation Details and Desired End-User Configuration

For our initial telemetry implementation, OpenTofu targets the widely supported OTLP exporter. The existing code (telemetry.go) already sets up OTLP exporters, so no additional exporter work is required unless future demand calls for alternative options.

> [!NOTE]  
> This feature will be purely opt-in, and users will need to set environment variables to enable tracing. This approach ensures that tracing doesn't impact performance for users who don't require it.

### Example: Running Jaeger with OTLP Support

Most popular tracing tools, such as Jaeger, support OTLP.

**Running Jaeger with OTLP Support:**

To quickly spin up a Jaeger instance with OTLP enabled using Docker, you can use the following command:
```shell
docker run \
    --rm \
    --name jaeger \
    -e COLLECTOR_OTLP_ENABLED=true \
    -p 16686:16686 \
    -p 4317:4317 \
    -p 4318:4318 \
    jaegertracing/all-in-one:1.54.0
```

This command starts a Jaeger all-in-one container with OTLP collection enabled. The OTLP endpoint will be exposed on port 4317 (and 4318 for HTTP/JSON if needed).

**Configuring OpenTofu**

To direct OpenTofu to send traces to your Jaeger instance, you can set the following environment variables:

```shell
export OTEL_TRACES_EXPORTER=otlp
export OTEL_SERVICE_NAME=opentofu
export OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4317
export OTEL_EXPORTER_OTLP_INSECURE=true
```
- `OTEL_TRACES_EXPORTER=otlp`: Instructs OpenTofu to use the OTLP exporter.
- `OTEL_SERVICE_NAME=opentofu`: Sets the service name to “opentofu” for trace identification.
- `OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4317`: Points the exporter to your Jaeger instance.
- `OTEL_EXPORTER_OTLP_INSECURE=true`: Disables TLS since the default Jaeger setup uses an insecure connection.

These settings ensure that trace data is properly exported to the Jaeger instance, offering a quick and effective way to visualize OpenTofu’s telemetry data.

## Benefits:
- **End-to-End Observability:** By adopting environment variable-based context propagation, OpenTofu can participate in distributed traces initiated by external systems, providing a complete view of the infrastructure provisioning process.
- **Standardization:** Aligning with OTEP #258 ensures compatibility with other tools and systems that follow the same standard, promoting interoperability.
- **Ease of Implementation:** Using environment variables for context propagation is straightforward and doesn't require significant changes to existing workflows, making it an efficient solution for integrating tracing across diverse systems.

## Possible Future Expansions

- **Tracing each and every call to providers**: This could be helpful for provider authors, or people who wish to understand more about what is happening. This would be easy to add but could easily add too much clutter. We should evaluate during development how informative these traces are.
- **Expanding Trace Context to the Provider**: For our gRPC calls to the providers, we could send the `TRACEPARENT` through to the provider. Then, if a provider author wishes to provide OTEL tracing, they can do and have the full context of what is ongoing. This would be helpful, however it is not needed right now due to no existing providers supporting this.

## Open Questions
- What should `tofu` do if a mal-formed `TRACEPARENT` is passed in?
- What is the performance impact of adding tracing to `tofu`?
- How do we find a happy medium of tracing enough to be useful but not too much to be overwhelming to maintain?