package telemetry

import (
	"log/slog"
	"os"
)

func LoggerSetup(serviceName string) {
	level := slog.LevelInfo
	if os.Getenv("OTEL_LOG_LEVEL") == "debug" {
		level = slog.LevelDebug
	}

	handler := slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		Level: level,
		ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey {
				a.Key = "timestamp"
			}
			if a.Key == slog.MessageKey {
				a.Key = "message"
			}
			return a
		},
	})

	logger := slog.New(handler).With(
		"service", serviceName,
		"env", os.Getenv("OTEL_DEPLOYMENT_ENVIRONMENT"),
	)

	slog.SetDefault(logger)
	slog.Info("logger initialized", "level", level.String())
}
