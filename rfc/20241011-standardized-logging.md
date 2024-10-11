# Standardized logging in and around OpenTofu 

OpenTofu and tools around it currently use a wide array of logging interfaces, such as [hclog](https://github.com/hashicorp/go-hclog), [go log](https://pkg.go.dev/log), [go slog](https://pkg.go.dev/log/slog), etc. Some of these tools also make use of logging in a global context, such as hclog being global in OpenTofu, making wiring tools using different logging tools together challenging.

This is especially true when it comes to testing. When go tests run in parallel, their output will get jumbled unless `t.Log()` or `t.Logf()` are used, which limits how much we can run tests in parallel. Unfortunately, the authors of the mentioned logging utilities have not provided convenient adapters for test logging.

This RFC attempts to outline the requirements and possible implementations for standardized logging across all projects in the OpenTofu space. This is a first of a series of RFCs attempting to outline how testing is done in OpenTofu.

## Requirements

### Requirement 1: Injectability

In order to make wiring together various tools possible and easy, the logging utility should support being injected rather than just provide globally-scoped functions.

### Requirement 2: Logging interface

In order to wire the logging output to various destinations (e.g. `t.Logf`), the logging interface should provide the ability to route log messages to their destination without undue complexity or opinionation. Ideally, the logger should provide a simple interface that any party can implement, such as:

```go
type Logger interface {
	Tracef(...)
	Infof(...)
	...
}
```

This is especially important because this logger should also be used in libraries we expose to third parties, such as `TofuDL`, and they should be free to bring their own loggers by fulfilling the interface requirements.

### Requirement 4: Test log adapter

The logging utility should provide an out-of-the-box adapter for the `t.Logf` facility in order to facilitate writing log messages from parallelized tests.

### Requirement 5: Sprintf semantics

Currently, most if not all of our log messages have an Sprintf-style calling structure, passing parameters along, such as:

```go
log.Warnf("Failed to do x (%v)", err)
```

The logging library should provide easy access to logging in a similar style. The functions with this semantic should have the `f` suffix to help IDEs identify them as an Sprintf-style function.

### Requirement 6: Context passing

In order to facilitate performance testing using OpenTelemetry and other tools, the logging utility should support passing along a context with log messages, for example:

```go
log.Warnf(ctx, "Failed to do x (%v)", err)
```

The logging implementation may opt to pick different function names for context-based and contextless logging.

### Requirement 7: The Writer interface

OpenTofu has to deal with outputs from external programs a lot, as do various libraries. For example, OpenTofu launches providers as external binaries, libregistry launches an external git, etc. The output of these programs should be easy to pipe to logs, optionally with a line-by-line prefix. Therefore, the logger should implement or provide easy access to an `io.WriteCloser` implementation, which logs on a line-by-line basis from a byte input.

### Requirement 8: Scoping

OpenTofu and its libraries are large codebases and it can be challenging to identify where a certain log message came from. Therefore, it is desirable that the logging library provide some sort of scoping mechanism individual parts can make use of. For example:

```go
func NewGitHub(logger log.Logger) GitHub {
	return &github{
		logger: logger.WithScope("GitHub")
    }
}
```

### Requirement 9: Log levels

In OpenTofu, we have need for at least the following log levels due to historic reasons, equivalent to the `TF_LOG` variable:

- Trace
- Debug
- Info
- Warn
- Error

The logging library should support these log levels.

### Requirement 10: Performance 

Because OpenTofu processes, at times, events in the order of millions, logging should have a minimal performance impact and should perform as little work as possible to fulfill the goals.

### Requirement 11: Usable as a library

Whichever logging tool we chose must be usable as a library with minimal dependencies as this tool will be included in a wide array of libraries used, at times, by third parties. The library should not have an excessive dependency tree that makes maintaining an SBOM tedious. If we decide to implement our own overlay for third party libraries, it should live in its own repository to make it easy to consume.

### Requirement 12: License

The logging tool must be compatible with the MPL-2.0 as well as the Apache-2.0 license.

### Other considerations

#### Structured logging

Ever since Graylog came onto the scene, structured logging was hoped to fill the need for more information (stack traces, metrics, etc.) alongside the log messages. However, in the 15 years since Graylog emerged, no standard or best practices have emerged as to what additional data to log. Furthermore, compiling additional structured information and serializing it into a wire format always carries a performance penalty, which contradicts requirement 9.

Because there is currently no clear standard as to what extra data to log, structured logging is out of scope for this RFC. If a strong need for structured logging emerges later, an additional RFC can provide an extended interface for this purpose.

## Possible implementations

### Hclog

HashiCorp's [hclog](https://github.com/hashicorp/go-hclog) is the de-facto standard logger for OpenTofu because it is built into so many tools. Whichever tool we pick, we must account for the ability to route log messages to hclog.

Hclog fulfills almost all requirements, with the following notes:

1. Hclog's `Logger` interface does not have an `f` suffix for its logging functions.
2. Hclog's `Logger` interface has a large number of additional functions that make writing an adapter difficult.
3. Hclog does not have an adapter for `t.Logf`, making it difficult to set up for test logging.
4. Hclog does not provide a context passing opportunity. Instead, it provides a way to send the logger along in the context.

### Go log

Go's built-in log functionality in the `log` package provides a basic, Sprintf-style logger. However, it does not provide an interface, and it is very cumbersome to route log messages to `t.Logf`, which makes it less than ideal for our purposes. It also does not provide a context passing functionality, nor does it provide a trace log level.

### Go slog

Go's structured logger in the `log/slog` package provides as structured logging interface for Go. However, it does not provide an interface, does not provide an Sprintf-style calling convention as it is meant primarily for structured logging. Callers must call `fmt.Sprintf` themselves to create a readable parametrized error message. It also provides no easy way to route messages to `t.Logf`. It also does not provide a Trace log level, which has to be added.

### Custom overlay

As prototyped in [libregistry](https://github.com/opentofu/libregistry/tree/main/logger), we can write a custom overlay to provide all functionality outlined above and still incur only minimal maintenance cost by outsourcing the heavy lifting to the libraries above.

This can be achieved by defining a lightweight logger interface and providing adapters, such as `log.NewTestLogger()` for testing, `log.NewSlogLogger()` for slog, etc. However, this should not live in libregistry, instead it should be its own library without dependencies.
