package tracing

import (
	"runtime"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
)

func Tracer() trace.Tracer {
	if !isTracingEnabled {
		return otel.Tracer("")
	}

	pc, _, _, ok := runtime.Caller(1)
	if !ok || runtime.FuncForPC(pc) == nil {
		return otel.Tracer("")
	}

	// We use the import path of the caller function as the tracer name.
	return otel.GetTracerProvider().Tracer(extractImportPath(runtime.FuncForPC(pc).Name()))
}

// extractImportPath extracts the import path from a full function name.
// the function names returned by runtime.FuncForPC(pc).Name() can be in the following formats
//
//	main.(*MyType).MyMethod
//	github.com/you/pkg.(*SomeType).Method-fm
//	github.com/you/pkg.functionName
func extractImportPath(fullName string) string {
	lastSlash := strings.LastIndex(fullName, "/")
	if lastSlash == -1 {
		// When there is no slash, then use everything before the first dot
		if dot := strings.Index(fullName, "."); dot != -1 {
			return fullName[:dot]
		}
		return "unknown"
	}

	dotAfterSlash := strings.Index(fullName[lastSlash:], ".")
	if dotAfterSlash == -1 {
		return "unknown"
	}

	return fullName[:lastSlash+dotAfterSlash]
}
