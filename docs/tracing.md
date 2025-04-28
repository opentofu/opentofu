# OpenTofu Tracing Guide

This document describes how to use and implement tracing in OpenTofu Core using OpenTelemetry.

> [!NOTE]  
> For background on the design decisions and motivation behind OpenTofu's tracing implementation, see the [OpenTelemetry Tracing RFC](https://github.com/opentofu/opentofu/blob/main/rfc/20250129-Tracing-For-Extra-Context.md).

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
> **For Contributors**: When adding tracing to OpenTofu, remember that the primary audience is **end users** who need to understand performance, not developers. Add spans sparingly to avoid polluting traces with too much detail.

### Basic Span Creation

```go
import (
    "github.com/opentofu/opentofu/internal/tracing"
    "github.com/opentofu/opentofu/internal/tracing/traceattrs"
    otelAttr "go.opentelemetry.io/otel/attribute"  // Note the alias
)

func SomeFunction(ctx context.Context) error {
    // Create a new span
    ctx, span := tracing.Tracer().Start(ctx, "Human readable operation name")
    defer span.End()
    
    // Add attributes to provide context
    span.SetAttributes(otelAttr.String("opentofu.some.attribute", "value"))
    
    // Using predefined attributes from traceattrs package
    span.SetAttributes(otelAttr.String(traceattrs.ProviderAddress, "hashicorp/aws"))
    
    // Your function logic here...
    
    // If an error occurs
    if err != nil {
        tracing.SetSpanError(span, err)
        return err
    }
    
    return nil
}
```

> [!TIP]  
> We should use the `otelAttr` alias for OpenTelemetry's attribute package to clearly distinguish it from OpenTofu's trace attribute constants in the `traceattrs` package.
> This convention makes the code more readable and prevents import conflicts.

### Tracing Conventions

#### Span Naming

- Use human-readable, action-oriented names that describe operations from a user perspective
- Prefer names like "Provider installation" over internal function names like "InstallProvider"
- Use consistent terminology from the OpenTofu CLI and documentation
- Span names should represent UX-level concepts, not internal code structure

#### Attributes

- Prefer standard [OpenTelemetry semantic conventions](https://opentelemetry.io/docs/specs/semconv/) where applicable
- Follow the [OpenTelemetry attribute naming convention](https://opentelemetry.io/docs/specs/semconv/general/naming/)
- Cross-cutting attributes are defined in `internal/tracing/traceattrs`

```go
// Good attribute names
"opentofu.provider.address"      // For provider addresses
"opentofu.module.source"         // For module sources
"opentofu.operation.target_count" // For operation-specific counts
```


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