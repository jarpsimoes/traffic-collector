package exporter

import (
	"context"

	"go.opentelemetry.io/otel/trace"
)

// Exporter is the interface for trace exporters
type Exporter interface {
	Tracer() trace.Tracer
	Stop(ctx context.Context) error
}
