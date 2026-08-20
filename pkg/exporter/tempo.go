package exporter

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"

	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/sdk/resource"
	sdk "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

// TempoExporter wraps OpenTelemetry OTLP exporter for Grafana Tempo
type TempoExporter struct {
	tracer   trace.Tracer
	provider *sdk.TracerProvider
	exporter sdk.SpanExporter
	logger   *zap.SugaredLogger
}

// NewTempoExporter creates a new Tempo exporter
func NewTempoExporter(endpoint string, insecure bool, username, password string, logger *zap.SugaredLogger) (*TempoExporter, error) {
	if logger == nil {
		logger = zap.NewNop().Sugar()
	}
	if (username == "") != (password == "") {
		return nil, fmt.Errorf("tempo basic auth requires both username and password")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*1e9) // 30 seconds
	defer cancel()

	endpoint = normalizeTempoEndpoint(endpoint)
	options := []otlptracegrpc.Option{
		otlptracegrpc.WithEndpoint(endpoint),
	}
	if insecure {
		options = append(options, otlptracegrpc.WithInsecure())
	}
	if username != "" {
		credentials := base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
		options = append(options, otlptracegrpc.WithHeaders(map[string]string{
			"authorization": "Basic " + credentials,
		}))
	}

	// Create OTLP gRPC exporter
	exporter, err := otlptracegrpc.New(ctx, options...)
	if err != nil {
		return nil, fmt.Errorf("failed to create OTLP exporter: %w", err)
	}

	// Create resource
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String("traffic-collector"),
			semconv.ServiceVersionKey.String("1.0.0"),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	// Create tracer provider
	provider := sdk.NewTracerProvider(
		sdk.WithBatcher(exporter),
		sdk.WithResource(res),
	)

	tracer := provider.Tracer("traffic-collector")

	logger.Infow("Tempo exporter initialized", "endpoint", endpoint)

	return &TempoExporter{
		tracer:   tracer,
		provider: provider,
		exporter: exporter,
		logger:   logger,
	}, nil
}

// Tracer returns the OpenTelemetry tracer
func (t *TempoExporter) Tracer() trace.Tracer {
	return t.tracer
}

// Stop gracefully stops the exporter
func (t *TempoExporter) Stop(ctx context.Context) error {
	if err := t.provider.Shutdown(ctx); err != nil {
		t.logger.Errorw("Error shutting down tracer provider", "error", err)
		return err
	}
	return nil
}

func normalizeTempoEndpoint(endpoint string) string {
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Host == "" {
		return endpoint
	}

	if parsed.Port() != "" {
		return parsed.Host
	}

	switch strings.ToLower(parsed.Scheme) {
	case "https":
		return parsed.Hostname() + ":443"
	case "http":
		return parsed.Hostname() + ":80"
	default:
		return parsed.Host
	}
}
