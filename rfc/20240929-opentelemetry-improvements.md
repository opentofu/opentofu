# RFC Title

TODO:
Issue: https://github.com/OpenTofu/{ Repository }/issues/{ issue number } <!-- Ideally, this issue will have the "needs-rfc" label added by the Core Team during triage -->

Currently OpenTofu supports extremely minimal amounts of OpenTelemetry tracing. This RFC proposes a series of improvements to the OpenTelemetry support in OpenTofu, along with the possible use cases we could support and how we can use those cases to reduce the complexity of the OpenTelemetry implementation.

### Past Discussions

https://github.com/opentofu/opentofu/issues/1519 - OpenTelemetry Support
https://github.com/opentofu/opentofu/pull/1517 - A draft PR that added OpenTelemetry support to the planning process in OpenTofu as a proof of concept.

https://terragrunt.gruntwork.io/docs/features/debugging/#opentelemetry-integration - Terragrunt has a similar implementation of OpenTelemetry that we could use as a reference.  I also think that it would be amazing to enable the tracing and get traces for both Terragrunt and OpenTofu collected all at once so you have full visibility end to end as to what is happening.

## Proposed Solution

In this RFC I propose to add a series of improvements to the OpenTelemetry support in OpenTofu. However it is not as simple as just adding a few lines of code, sadly the codebase was not designed to be passing around `context.Context` objects for each call, and the majority of the work that will be required in this RFC will be to refactor the codebase to support propogating the `context.Context` object through the codebase.

Due to the complexity of the codebase, I propose that we identify areas of the codebase that we want to start with and refactor those areas to support OpenTelemetry. This will allow us to incrementally add OpenTelemetry support to the codebase, and will allow us to identify any issues that we may encounter as we add OpenTelemetry support to the codebase.

### Use Cases

#### "Why is my plan|apply|destroy taking so long?"
In an effort to improve the performance of the plan, apply,and similar processes, we can use OpenTelemetry to trace the time taken for each step of the process. This will allow us to identify areas of the codebase that are taking a long time to execute, and will allow us to identify areas of the codebase that we can improve. 

If we can provide end users with a mechanism to trace the time taken, then we us this as a mechanism to identify areas of the codebase that are taking a long time to execute. It is very difficult to identify areas that require improvements across the board without having full visibility into what is happening.

#### Identifying regressions of performance in the codebase
When implementing new features in OpenTofu, such as provider-defined functions, we can use OpenTelemetry to trace the time taken for each step of the process. If we can have these traces in place, we can determine if we've introduced a regression in performance in the codebase. 

This is extremely important as we continue to add new features to OpenTofu, as we want to ensure that we are not introducing regressions in performance in the codebase. In fact we want to ensure that we are improving the performance of the codebase as we add new features.

### User Documentation

To enable OpenTelemetry trace collection in OpenTofu, you will need to set the `OTEL_EXPORTER` environment variable to `otel` and the `OTEL_SERVICE_NAME` environment variable to the name of the service you are running, this could be `opentofu`. This will enable OpenTelemetry trace collection in OpenTofu.

If you also provide the `OTEL_EXPORTER_OTLP_ENDPOINT` environment variable, you can send the traces to an OpenTelemetry Collector. This will allow you to collect traces from multiple services and aggregate them in one place.

An example of this may be exporting to a jaeger instance that you have started using docker:

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

And then setting the environment variables to be used by OpenTofu:

```shell
export OTEL_EXPORTER=otel
export OTEL_SERVICE_NAME=opentofu
export OTEL_EXPORTER_OTLP_ENDPOINT=localhost:4317
tofu init
```

### Technical Approach

For each package in which we intend to produce traces from, we introduce a new package-wide variable that will hold the `Tracer` object. This will allow us to create a new `Tracer` object for each package that we want to produce traces from.

For example, you can see the existing `telemetry.go` in the `internal/command` package:

```go
...
package command

import (
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
)

var tracer trace.Tracer

func init() {
	tracer = otel.Tracer("github.com/opentofu/opentofu/internal/command")
}
```

This `tracer` object can then be used to create spans in the codebase. For example:
```go
	ctx, span := tracer.Start(ctx, "install providers")
	defer span.End()
```

As you can see, the tracer.Start call will require the `context.Context` object to be passed in. This is the main reason why we need to refactor the codebase to support passing around the `context.Context` object. Without this refactoring, we will not be able to produce decent traces from the codebase.

#### The fun part: Adding context.Context everywhere

The main part of this RFC will be to refactor the codebase to support passing around the `context.Context` object. This will be a large amount of work, and will require a lot of testing to ensure that we have not broken anything in the codebase.

In situations where a context is already passed into a method, it is usually to ensure that we're timing out a call after a certain amount of time. In these situations, I propose that we pass a second context to the method called `traceCtx` which is used for just tracing. Whilst this does introduce a little bit of complexity, it reduces the risk of the initial implementation by not requiring us to merge the `context.Context` object with the `traceCtx` object.

#### How to handle different use cases?
We may find that we want to provide 2 different levels of tracing, one for end-user consumption and one for debugging and internal use. In this case,
we may want to have a way to turn some certain tracing on/off.

I propose we have 2 different levels of tracing. This will mean that instead of exposing a `Tracer` object in the package, we instead expose something that is an extremely thin wrapper that understands if we are in "debug" tracing mode, or just "user" tracing mode.

For example:
```go
type TraceManager struct {
    tracer    trace.Tracer
    debugMode bool
}

func (tm *TraceManager) Start(ctx context.Context, name string) (context.Context, trace.Span) {
 return tm.tracer.Start(ctx, name)
}

func (tm *TraceManager) StartDebug(ctx context.Context, name string) (context.Context, trace.Span) {
 if tm.debugMode {
  return tm.tracer.Start(ctx, name)
 }
 return ctx, trace.NoopSpan{}
}
```

If we do it this way, the `TraceManager` object can be created with a `debugMode` flag that will allow us to turn on/off the debug tracing. This can be controlled with an env var or a cli flag. This will allow us to have 2 different levels of tracing in the codebase, without having to check if we are in debug mode in every single call to `Start`.

#### Initial areas to target with OpenTelemetry

- Initialization
  - Time taken to determine provider/module versions that require fetching
  - Time taken to fetch each provider/module versions
  - Time taken to handle checking GPG signatures and checksums
- Planning
  - Time taken to parse all the configuration files
  - Time taken to determine the changes that need to be made
  - Time taken for each call to the provider
  - Time taken for each function call

These 2 initial areas allow us to quickly help people determine why runs are being slow, and with a large overlap of logic between the 2 areas and the apply phase we can quickly add tracing to the apply phase later on.

### Open Questions

- Is there a large overhead in introducing tracing?

### Future Considerations

- How can we ensure that context is passed around correctly in the codebase?
- Can we use OpenTelemetry traces to automate regression testing around performance?

## Potential Alternatives
I am not aware of any potential alternatives
