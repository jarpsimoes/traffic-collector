package collector

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/jarpsimoes/traffic-collector/pkg/ebpf"
	"github.com/jarpsimoes/traffic-collector/pkg/exporter"
	"github.com/jarpsimoes/traffic-collector/pkg/k8s"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

// Config holds the collector configuration
type Config struct {
	NodeName       string
	TempoEndpoint  string
	JaegerEndpoint string
	BatchSize      int
	BatchTimeout   time.Duration
	Logger         *zap.Logger
}

// NewConfig creates a new Config with defaults from environment
func NewConfig() *Config {
	cfg := &Config{
		NodeName:       os.Getenv("NODE_NAME"),
		TempoEndpoint:  getEnvOrDefault("TEMPO_ENDPOINT", "http://tempo:4317"),
		JaegerEndpoint: getEnvOrDefault("JAEGER_ENDPOINT", "http://jaeger-collector:14268"),
		BatchSize:      100,
		BatchTimeout:   10 * time.Second,
	}

	if cfg.NodeName == "" {
		cfg.NodeName = os.Hostname()
	}

	return cfg
}

func getEnvOrDefault(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

// Collector represents the main traffic collection component
type Collector struct {
	cfg       *Config
	logger    *zap.Logger
	ebpf      *ebpf.Program
	exporters []exporter.Exporter
	k8sMeta   *k8s.Metadata

	mu           sync.RWMutex
	running      bool
	health       bool
	ready        bool
	tracesCount  uint64
	errorsCount  uint64
	uptime       time.Time
}

// New creates a new Collector instance
func New(cfg *Config) (*Collector, error) {
	if cfg.Logger == nil {
		cfg.Logger = zap.NewNop()
	}

	coll := &Collector{
		cfg:    cfg,
		logger: cfg.Logger,
		uptime: time.Now(),
	}

	// Initialize eBPF program
	var err error
	coll.ebpf, err = ebpf.NewProgram(cfg.Logger)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize eBPF program: %w", err)
	}

	// Initialize exporters
	if err := coll.initExporters(); err != nil {
		return nil, fmt.Errorf("failed to initialize exporters: %w", err)
	}

	// Initialize Kubernetes metadata enrichment
	coll.k8sMeta, err = k8s.NewMetadata(cfg.Logger)
	if err != nil {
		coll.logger.Warnw("Failed to initialize Kubernetes metadata", "error", err)
		// Don't fail if k8s metadata is unavailable
		coll.k8sMeta = nil
	}

	coll.logger.Infow("Collector initialized",
		"node", cfg.NodeName,
		"tempo_endpoint", cfg.TempoEndpoint,
		"jaeger_endpoint", cfg.JaegerEndpoint,
	)

	return coll, nil
}

func (c *Collector) initExporters() error {
	// Initialize Tempo exporter (OTLP)
	tempoExp, err := exporter.NewTempoExporter(c.cfg.TempoEndpoint, c.logger)
	if err != nil {
		c.logger.Warnw("Failed to initialize Tempo exporter", "error", err, "endpoint", c.cfg.TempoEndpoint)
	} else {
		c.exporters = append(c.exporters, tempoExp)
		c.logger.Infow("Tempo exporter initialized", "endpoint", c.cfg.TempoEndpoint)
	}

	// Initialize Jaeger exporter
	jaegerExp, err := exporter.NewJaegerExporter(c.cfg.JaegerEndpoint, c.logger)
	if err != nil {
		c.logger.Warnw("Failed to initialize Jaeger exporter", "error", err, "endpoint", c.cfg.JaegerEndpoint)
	} else {
		c.exporters = append(c.exporters, jaegerExp)
		c.logger.Infow("Jaeger exporter initialized", "endpoint", c.cfg.JaegerEndpoint)
	}

	if len(c.exporters) == 0 {
		return fmt.Errorf("no exporters initialized")
	}

	return nil
}

// Start begins collecting network traffic
func (c *Collector) Start(ctx context.Context) error {
	c.mu.Lock()
	if c.running {
		c.mu.Unlock()
		return fmt.Errorf("collector already running")
	}
	c.running = true
	c.health = true
	c.ready = true
	c.mu.Unlock()

	c.logger.Infow("Starting traffic collection")

	// Start event processing loop
	go c.eventLoop(ctx)

	return nil
}

func (c *Collector) eventLoop(ctx context.Context) {
	defer func() {
		c.mu.Lock()
		c.running = false
		c.mu.Unlock()
	}()

	if err := c.ebpf.Start(ctx); err != nil {
		c.logger.Errorw("eBPF program error", "error", err)
		c.mu.Lock()
		c.health = false
		c.mu.Unlock()
		return
	}

	// Process events from eBPF program
	for event := range c.ebpf.Events() {
		if err := c.processEvent(ctx, event); err != nil {
			c.logger.Debugw("Error processing event", "error", err)
			c.mu.Lock()
			c.errorsCount++
			c.mu.Unlock()
		}
	}
}

func (c *Collector) processEvent(ctx context.Context, evt *ebpf.TrafficEvent) error {
	// Create a span for this network event
	tracer := c.exporters[0].Tracer() // Use first exporter's tracer
	if tracer == nil {
		return fmt.Errorf("no tracer available")
	}

	span := tracer.Start(ctx, "network.traffic",
		trace.WithAttributes(
			// Network attributes
			// Source and destination would be added from evt
		))
	defer span.End()

	// Enrich with pod metadata if available
	if c.k8sMeta != nil {
		if podInfo := c.k8sMeta.GetPodInfo(evt.PID); podInfo != nil {
			// Add pod metadata to span
			_ = podInfo // Use pod info for enrichment
		}
	}

	c.mu.Lock()
	c.tracesCount++
	c.mu.Unlock()

	return nil
}

// Stop gracefully stops the collector
func (c *Collector) Stop(ctx context.Context) error {
	c.mu.Lock()
	if !c.running {
		c.mu.Unlock()
		return nil
	}
	c.running = false
	c.mu.Unlock()

	c.logger.Infow("Stopping traffic collector")

	// Stop eBPF program
	if err := c.ebpf.Stop(ctx); err != nil {
		c.logger.Errorw("Error stopping eBPF program", "error", err)
	}

	// Stop exporters
	for _, exp := range c.exporters {
		if err := exp.Stop(ctx); err != nil {
			c.logger.Errorw("Error stopping exporter", "error", err)
		}
	}

	return nil
}

// IsHealthy returns true if the collector is healthy
func (c *Collector) IsHealthy() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.health && c.running
}

// IsReady returns true if the collector is ready to process events
func (c *Collector) IsReady() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.ready && c.running
}

// Stats returns collector statistics
func (c *Collector) Stats() map[string]interface{} {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return map[string]interface{}{
		"running":       c.running,
		"healthy":       c.health,
		"ready":         c.ready,
		"traces_count":  c.tracesCount,
		"errors_count":  c.errorsCount,
		"uptime_seconds": int(time.Since(c.uptime).Seconds()),
	}
}
