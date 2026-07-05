package telemetry

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

var (
	GlobalTracer trace.Tracer = otel.Tracer("ewallet-api")
)

func TracerSetup(ctx context.Context, serviceName string) (func(context.Context) error, error) {
	exporterEndpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")

	insecure := os.Getenv("OTEL_EXPORTER_OTLP_INSECURE") == "true"

	endpoint := strings.TrimPrefix(exporterEndpoint, "http://")
	endpoint = strings.TrimPrefix(endpoint, "https://")

	opts := []otlptracehttp.Option{
		otlptracehttp.WithEndpoint(endpoint),
	}
	if insecure {
		opts = append(opts, otlptracehttp.WithInsecure())
	}

	exp, err := otlptracehttp.New(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create OTLP trace exporter: %w", err)
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			attribute.String("service.name", serviceName),
			attribute.String("service.version", os.Getenv("SYSTEM_VERSION")),
			attribute.String("deployment.environment", os.Getenv("OTEL_DEPLOYMENT_ENVIRONMENT")),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	sampler := sdktrace.AlwaysSample()
	if os.Getenv("OTEL_TRACES_SAMPLER") == "always_off" {
		sampler = sdktrace.NeverSample()
	}

	bsp := sdktrace.NewBatchSpanProcessor(exp,
		sdktrace.WithBatchTimeout(5*time.Second),
		sdktrace.WithMaxQueueSize(2048),
		sdktrace.WithMaxExportBatchSize(512),
	)

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sampler),
		sdktrace.WithResource(res),
		sdktrace.WithSpanProcessor(bsp),
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})

	GlobalTracer = tp.Tracer(serviceName)

	return func(ctx context.Context) error {
		return tp.Shutdown(ctx)
	}, nil
}

func GetMethodName(skip int) string {
	pc, _, _, ok := runtime.Caller(skip)
	if !ok {
		return "unknown"
	}
	fn := runtime.FuncForPC(pc)
	if fn == nil {
		return "unknown"
	}
	name := fn.Name()
	if idx := strings.LastIndex(name, "."); idx != -1 {
		return name[idx+1:]
	}
	return name
}

func SetupTracerHandler(c fiber.Ctx) (fiber.Ctx, trace.Span) {
	methodName := GetMethodName(2)
	ctx := c.Context()

	route := ""
	if c.Route() != nil {
		route = c.Route().Path
	}

	opts := []trace.SpanStartOption{
		trace.WithSpanKind(trace.SpanKindServer),
		trace.WithAttributes(
			attribute.String("handler.method", methodName),
			attribute.String("http.method", c.Method()),
			attribute.String("http.url", c.OriginalURL()),
			attribute.String("http.route", route),
			attribute.String("client.ip", c.IP()),
			attribute.String("user_agent", c.Get("User-Agent")),
		),
	}

	ctx, span := GlobalTracer.Start(ctx, "Handler."+methodName, opts...)

	traceID := span.SpanContext().TraceID().String()
	spanID := span.SpanContext().SpanID().String()
	c.Locals("trace_id", traceID)
	c.Locals("span_id", spanID)
	c.Set("X-Trace-ID", traceID)

	c.SetContext(ctx)
	return c, span
}

func EndTracerHandler(c fiber.Ctx, span trace.Span, err error) {
	status := c.Response().StatusCode()
	span.SetAttributes(attribute.Int("http.status_code", status))

	if err != nil || status >= 400 {
		span.SetStatus(codes.Error, "request failed")
		if err != nil {
			span.RecordError(err)
		}
	}
	span.End()
}

func SetupTracerService(ctx context.Context) (context.Context, trace.Span) {
	methodName := GetMethodName(2)

	_, file, line, ok := runtime.Caller(1)
	callerInfo := ""
	if ok {
		if idx := strings.LastIndex(file, "/"); idx != -1 {
			file = file[idx+1:]
		}
		callerInfo = fmt.Sprintf("%s:%d", file, line)
	}

	opts := []trace.SpanStartOption{
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(
			attribute.String("code.function", methodName),
			attribute.String("code.file", callerInfo),
			attribute.String("trace_id", uuid.New().String()),
		),
	}

	return GlobalTracer.Start(ctx, "Service."+methodName, opts...)
}

func EndTracerService(span trace.Span, err error) {
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		span.RecordError(err)
	}
	span.End()
}
