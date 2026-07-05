package telemetry

import (
	"log"
	"net/http"
	"os"

	"github.com/gofiber/fiber/v3"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

var (
	globalMeter          metric.Meter
	requestDuration      metric.Float64Histogram
	requestCounter       metric.Int64Counter
	errorCounter         metric.Int64Counter
	requestsInFlight     metric.Int64UpDownCounter
	responseSize         metric.Int64Histogram
	metricsInitialized   bool
)

func MeterSetup() error {
	metricsPort := os.Getenv("METRICS_PORT")

	globalMeter = otel.Meter("ewallet-api")
	metricsInitialized = true

	var err error
	requestDuration, err = globalMeter.Float64Histogram(
		"http_request_duration_seconds",
		metric.WithDescription("Duration of HTTP requests in seconds"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return err
	}

	requestCounter, err = globalMeter.Int64Counter(
		"http_requests_total",
		metric.WithDescription("Total number of HTTP requests"),
	)
	if err != nil {
		return err
	}

	errorCounter, err = globalMeter.Int64Counter(
		"http_errors_total",
		metric.WithDescription("Total number of HTTP errors"),
	)
	if err != nil {
		return err
	}

	requestsInFlight, err = globalMeter.Int64UpDownCounter(
		"http_requests_in_flight",
		metric.WithDescription("Current number of HTTP requests being served"),
	)
	if err != nil {
		return err
	}

	responseSize, err = globalMeter.Int64Histogram(
		"http_response_size_bytes",
		metric.WithDescription("Size of HTTP response bodies"),
		metric.WithUnit("bytes"),
	)
	if err != nil {
		return err
	}

	go func() {
		mux := http.NewServeMux()
		mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
		log.Printf("metrics server listening on :%s", metricsPort)
		if err := http.ListenAndServe(":"+metricsPort, mux); err != nil {
			log.Printf("metrics server error: %v", err)
		}
	}()

	return nil
}

func MetricsMiddleware(c fiber.Ctx) error {
	if !metricsInitialized {
		return c.Next()
	}

	ctx := c.Context()
	method := c.Method()

	path := c.Path()
	if c.Route() != nil {
		path = c.Route().Path
	}
	route := normalizeRoute(path)

	attrs := []attribute.KeyValue{
		attribute.String("http.method", method),
		attribute.String("http.route", route),
	}

	requestsInFlight.Add(ctx, 1, metric.WithAttributes(attrs...))
	defer requestsInFlight.Add(ctx, -1, metric.WithAttributes(attrs...))

	err := c.Next()

	statusCode := c.Response().StatusCode()
	statusGroup := statusGroup(statusCode)

	allAttrs := append(attrs,
		attribute.Int("http.status_code", statusCode),
		attribute.String("http.status_group", statusGroup),
		attribute.String("service.name", "ewallet-api"),
	)

	requestCounter.Add(ctx, 1, metric.WithAttributes(allAttrs...))

	if statusCode >= 400 {
		errorCounter.Add(ctx, 1, metric.WithAttributes(allAttrs...))
	}

	return err
}

func normalizeRoute(path string) string {
	normalized := ""
	for i := 0; i < len(path); i++ {
		if path[i] == ':' {
			normalized += ":id"
			for i < len(path) && path[i] != '/' {
				i++
			}
			if i < len(path) {
				normalized += "/"
			}
			continue
		}
		normalized += string(path[i])
	}
	return normalized
}

func statusGroup(code int) string {
	switch {
	case code < 200:
		return "1xx"
	case code < 300:
		return "2xx"
	case code < 400:
		return "3xx"
	case code < 500:
		return "4xx"
	default:
		return "5xx"
	}
}
