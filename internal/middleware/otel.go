package middleware

import (
	"net/http"

	"github.com/faisalaffan/ewallet-system/pkg/telemetry"
	"github.com/gofiber/fiber/v3"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

func OtelMiddleware(c fiber.Ctx) error {
	propagator := propagation.TraceContext{}

	headerMap := http.Header{}
	c.Request().Header.VisitAll(func(key, value []byte) {
		headerMap.Set(string(key), string(value))
	})
	ctx := propagator.Extract(c.Context(), propagation.HeaderCarrier(headerMap))

	method := c.Method()
	path := c.Route().Path

	ctx, span := telemetry.GlobalTracer.Start(ctx, method+" "+path,
		trace.WithSpanKind(trace.SpanKindServer),
		trace.WithAttributes(
			attribute.String("http.method", method),
			attribute.String("http.route", path),
			attribute.String("http.url", c.OriginalURL()),
			attribute.String("client.ip", c.IP()),
			attribute.String("user_agent", c.Get("User-Agent")),
		),
	)
	defer span.End()

	c.SetContext(ctx)

	err := c.Next()

	statusCode := c.Response().StatusCode()
	span.SetAttributes(attribute.Int("http.status_code", statusCode))

	if statusCode >= 400 {
		span.SetStatus(codes.Error, "request failed")
	}

	return err
}
