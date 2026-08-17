package exporter

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/exporters/jaeger/thrift"
	"go.opentelemetry.io/otel/sdk/resource"
	sdk "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/semconv/v1.21.0"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

// JaegerExporter wraps OpenTelemetry Jaeger exporter
type JaegerExporter struct {
	tracer   trace.Tracer
	provider *sdk.TracerProvider
	exporter sdk.SpanExporter
	logger   *zap.Logger
}

// NewJaegerExporter creates a new Jaeger exporter
func NewJaegerExporter(endpoint string, logger *zap.Logger) (*JaegerExporter, error) {
	if logger == nil {
		logger = zap.NewNop()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*1e9) // 30 seconds
	defer cancel()

	// Create Jaeger Thrift exporter
	exporter, err := thrift.New(
		thrift.WithAgentHost("jaeger-agent"),
		thrift.WithAgentPort(6831),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create Jaeger exporter: %w", err)
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

	logger.Infow("Jaeger exporter initialized", "endpoint", endpoint)

	return &JaegerExporter{
		tracer:    tracer,
		provider:  provider,
		exporter:  exporter,
		logger:    logger,
	}, nil
}

// Tracer returns the OpenTelemetry tracer
func (j *JaegerExporter) Tracer() trace.Tracer {
	return j.tracer
}

// Stop gracefully stops the exporter
func (j *JaegerExporter) Stop(ctx context.Context) error {
	if err := j.provider.Shutdown(ctx); err != nil {
		j.logger.Errorw("Error shutting down tracer provider", "error", err)
		return err
	}
	return nil
}
