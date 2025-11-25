# OpenTofu Tracing Guide

This document describes how to use and implement tracing in OpenTofu Core using OpenTelemetry.

There's background information on OpenTofu's tracing implementation in [the OpenTelemetry Tracing RFC](https://github.com/opentofu/opentofu/blob/main/rfc/20250129-Tracing-For-Extra-Context.md)

> [!WARNING]
> If you change which version of the `go.opentelemetry.io/otel/sdk` we have selected in our `go.mod`, you **must** make sure that `internal/tracing/traceattrs/semconv.go` imports the same subpackage of `go.opentelemetry.io/otel/semconv/*` that is used by the selected version of `go.opentelemetry.io/otel/sdk`.
>
> This is important because our tracing setup uses a blend of directly-constructed `semconv` attributes and attributes chosen indirectly through the `resource` package, and they must all be using the same version of the semantic conventions schema or there will be a "conflicting Schema URL" error at runtime.
>
> (Problems of this sort should be detected both by a unit test in `internal/tracing/traceattrs` and an end-to-end test that executes OpenTofu with tracing enabled.)

## Overview

OpenTofu provides distributed tracing capabilities via OpenTelemetry to help end users understand the execution flow and performance characteristics of OpenTofu operations. Tracing is particularly useful for:

- Debugging performance issues (e.g., "Why is my plan taking so long?")
- Understanding time spent in different operations
- Visualizing the execution flow across providers and modules
- Diagnosing issues in CI/CD pipelines

Tracing in OpenTofu is **strictly opt-in** and disabled by default. It's designed to have minimal overhead when disabled and to provide valuable insights when enabled.

> [!IMPORTANT]  
> OpenTofu's tracing functionality refers only to OpenTelemetry traces for local debugging and analysis.
> No telemetry or usage data is sent to external servers, and no data leaves your environment unless you explicitly configure an external collector.

## Enabling Tracing

To enable tracing in OpenTofu:

1. Set the environment variable `OTEL_TRACES_EXPORTER=otlp`
2. Configure the OpenTelemetry exporter using standard OpenTelemetry environment variables

Example configuration for a local Jaeger collector:

```bash
export OTEL_TRACES_EXPORTER=otlp
export OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4317
export OTEL_EXPORTER_OTLP_INSECURE=true
```

For a complete list of configuration options, refer to the [OpenTelemetry Documentation](https://opentelemetry.io/docs/specs/otel/protocol/exporter/).

## Quick Start with Jaeger

To quickly spin up a local Jaeger instance with OTLP support:

```bash
docker run -d --rm --name jaeger \
  -p 16686:16686 \
  -p 4317:4317 \
  -p 4318:4318 \
  -p 5778:5778 \
  -p 9411:9411 \
  jaegertracing/jaeger:2.5.0
```

Then configure OpenTofu as shown above and access the Jaeger UI at http://localhost:16686.

## Adding Tracing to OpenTofu Code

> [!NOTE]  
> **For Contributors**: When adding tracing to OpenTofu, remember that the primary audience is **end users** who need to understand performance, not OpenTofu developers. Add spans sparingly to avoid polluting traces with too much detail.

### Basic Span Creation

```go
import (
    "github.com/opentofu/opentofu/internal/tracing"
    "github.com/opentofu/opentofu/internal/tracing/traceattrs"
)

func SomeFunction(ctx context.Context) error {
    // Create a new span
    ctx, span := tracing.Tracer().Start(ctx, "Human readable operation name",
        tracing.SpanAttributes(
            traceattrs.String("opentofu.some_attribute", "value")
        ),
    )
    defer span.End()
    
    // Optionally add additional attributes after the span is created, if
    // they only need to appear in certain cases.
    span.SetAttributes(traceattrs.String("opentofu.some_other_attribute", "value"))
    
    // Use the more specific attribute-construction helpers from package
    // traceattrs where they are relevant, to ensure we follow consistent
    // semantic conventions for cross-cutting concerns.
    span.SetAttributes(traceattrs.OpenTofuProviderAddress("hashicorp/aws"))
    
    // Your function logic here...
    
    // If an error occurs
    if err != nil {
        tracing.SetSpanError(span, err)
        return err
    }
    
    return nil
}
```

OpenTelemetry has many different packages spread across a variety of different Go modules, and those different modules often need to be upgraded together to ensure consistent behavior and avoid errors at runtime.

Therefore we prefer to directly import `go.opentelemetry.io/otel/*` packages only from our packages under `internal/tracing`, and then reexport certain functions from our own packages so that we can manage all of the OpenTelemetry dependencies in a centralized place to minimize "dependency hell" problems when upgrading. Packages under `go.opentelemetry.io/contrib/instrumentation/*` are an exception because they tend to be more tightly-coupled to whatever they are instrumenting than to the other OpenTelemetry packages, and so it's better to import those from the same file that's importing whatever other package the instrumentation is being applied to.

> [!WARNING]
> Don't import `go.opentelemetry.io/otel/semconv/*` packages from anywhere except `internal/tracing/traceattrs/semconv.go`!
>
> If you want to use standard OpenTelemetry semantic conventions from other packages, use them indirectly through reexports in `package traceattrs` instead, so we can make sure there's only one file in OpenTofu deciding which version of semconv we are currently depending on.

### Tracing Conventions

#### Span Naming

- Use human-readable, action-oriented names that describe operations from a user perspective
- Prefer names like "Provider installation" over internal function names like "InstallProvider"
- Use consistent terminology from the OpenTofu CLI and documentation
- Span names should represent UX-level concepts, not internal code structure

#### Attributes

- Prefer standard [OpenTelemetry semantic conventions](https://opentelemetry.io/docs/specs/semconv/) where applicable, using helper functions from [`internal/tracing/traceattrs`](https://pkg.go.dev/github.com/opentofu/opentofu/internal/tracing/traceattrs).
- Use `OpenTofu`-prefixed functions in [`internal/tracing/traceattrs`](https://pkg.go.dev/github.com/opentofu/opentofu/internal/tracing/traceattrs) for OpenTofu-specific cross-cutting concerns.
- It's okay to use one-off inline strings for attribute names specific to a single span, but make sure to still follow the [OpenTelemetry attribute naming conventions](https://opentelemetry.io/docs/specs/semconv/general/naming/) and use the `opentofu.` prefix for anything that is not a standardized semantic convention.
- If a particular subsystem of OpenTofu has some repeated conventions for attribute names, consider creating unexported string constants or attribute construction helper functions in the same package to centralize those naming conventions.

#### Error Handling

Use the `tracing.SetSpanError` helper to consistently record errors:

```go
if err != nil {
    tracing.SetSpanError(span, err)
    return err
}
```

This helper supports various error types including standard errors, strings, and OpenTofu diagnostics.

### Instrumentation Guidelines

1. **Focus on Key Operations**: Instrument high-level operations that are meaningful to end users rather than every internal function.
2. **Include Valuable Context**: Add attributes that help identify resources, modules, or operations.
3. **Respect Performance**: Avoid expensive computations solely for tracing.
