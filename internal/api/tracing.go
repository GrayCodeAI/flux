package api

import (
	"net/http"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

var tracer = otel.Tracer("graycode-router/internal/api")

// statusRecorder wraps http.ResponseWriter to capture the status code.
// It also passes through Flusher and Hijacker interfaces when the underlying
// writer supports them (required for SSE streaming).
type statusRecorder struct {
	http.ResponseWriter
	statusCode int
	written    bool
}

func (sr *statusRecorder) WriteHeader(code int) {
	if !sr.written {
		sr.statusCode = code
		sr.written = true
	}
	sr.ResponseWriter.WriteHeader(code)
}

func (sr *statusRecorder) Write(b []byte) (int, error) {
	if !sr.written {
		sr.statusCode = http.StatusOK
		sr.written = true
	}
	return sr.ResponseWriter.Write(b)
}

// Flush passes through to the underlying ResponseWriter if it supports Flusher.
func (sr *statusRecorder) Flush() {
	if f, ok := sr.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// TracingMiddleware returns an HTTP middleware that creates an OTel span for
// each request, recording method, route, status code, and error state.
func TracingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		spanName := r.Method + " " + r.URL.Path
		ctx, span := tracer.Start(
			r.Context(),
			spanName,
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(
				semconv.HTTPRequestMethodKey.String(r.Method),
				semconv.URLPath(r.URL.Path),
				semconv.HTTPRoute(r.URL.Path),
				semconv.ServerAddress(r.Host),
				semconv.UserAgentOriginal(r.UserAgent()),
			),
		)
		defer span.End()

		// Propagate the span context through the request.
		r = r.WithContext(ctx)

		sr := &statusRecorder{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(sr, r)

		span.SetAttributes(
			attribute.Int("http.status_code", sr.statusCode),
			semconv.HTTPResponseStatusCode(sr.statusCode),
		)

		if sr.statusCode >= 400 {
			span.SetStatus(codes.Error, http.StatusText(sr.statusCode))
			span.SetAttributes(attribute.Bool("error", true))
		} else {
			span.SetStatus(codes.Ok, "")
		}
	})
}
