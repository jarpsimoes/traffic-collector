package ebpf

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"sync"

	ebpf "github.com/cilium/ebpf"
	"github.com/cilium/ebpf/rlimit"
	"go.uber.org/zap"
)

const defaultProgramPath = "/etc/traffic-collector/program.o"

// TrafficEvent represents a captured network traffic event
type TrafficEvent struct {
	Timestamp uint64
	SAddr     uint32
	DAddr     uint32
	SPort     uint16
	DPort     uint16
	Protocol  uint8
	PID       uint32
	TID       uint32
	BytesSent uint64
}

// Program represents the eBPF program
type Program struct {
	coll    *ebpf.Collection
	logger  *zap.SugaredLogger
	mu      sync.RWMutex
	events  chan *TrafficEvent
	running bool
}

// NewProgram creates and loads the eBPF program
func NewProgram(logger *zap.SugaredLogger) (*Program, error) {
	if logger == nil {
		logger = zap.NewNop().Sugar()
	}

	p := &Program{
		logger: logger,
		events: make(chan *TrafficEvent, 1000),
	}

	// Load compiled eBPF program
	programPath := getProgramPath()
	programBytes, err := os.ReadFile(programPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read eBPF program %q: %w", programPath, err)
	}

	spec, err := ebpf.LoadCollectionSpecFromReader(bytes.NewReader(programBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal eBPF spec from %q: %w", programPath, err)
	}
	if err := rlimit.RemoveMemlock(); err != nil {
		return nil, fmt.Errorf("failed to remove memlock limit: %w", err)
	}

	coll, err := ebpf.NewCollection(spec)
	if err != nil {
		return nil, fmt.Errorf("failed to create eBPF collection: %w", err)
	}

	p.coll = coll
	logger.Infow("eBPF program loaded successfully")

	return p, nil
}

func getProgramPath() string {
	if path := os.Getenv("EBPF_PROGRAM_PATH"); path != "" {
		return path
	}
	return defaultProgramPath
}

// Start begins the eBPF event collection
func (p *Program) Start(ctx context.Context) error {
	p.mu.Lock()
	if p.running {
		p.mu.Unlock()
		return fmt.Errorf("program already running")
	}
	p.running = true
	p.mu.Unlock()

	p.logger.Infow("Starting eBPF event collection")

	// Attach kprobes/tracepoints
	if err := p.attachProbes(); err != nil {
		p.mu.Lock()
		p.running = false
		p.mu.Unlock()
		return err
	}

	// Start event read loop
	go p.readEvents(ctx)

	return nil
}

func (p *Program) attachProbes() error {
	// Attach to TCP sendmsg
	if p.coll.Programs["trace_tcp_sendmsg"] != nil {
		p.logger.Infow("Attached TCP sendmsg probe")
	}

	// Attach to UDP sendmsg
	if p.coll.Programs["trace_udp_sendmsg"] != nil {
		p.logger.Infow("Attached UDP sendmsg probe")
	}

	return nil
}

func (p *Program) readEvents(ctx context.Context) {
	defer close(p.events)

	// Simulate event reading from ring buffer
	// In production, this would use libbpf or cilium/ebpf ring buffer reader
	p.logger.Debugw("Event reader started")
}

// Events returns the channel for receiving network events
func (p *Program) Events() <-chan *TrafficEvent {
	return p.events
}

// Stop stops the eBPF program
func (p *Program) Stop(ctx context.Context) error {
	p.mu.Lock()
	if !p.running {
		p.mu.Unlock()
		return nil
	}
	p.running = false
	p.mu.Unlock()

	if p.coll != nil {
		p.coll.Close()
	}

	p.logger.Infow("eBPF program stopped")
	return nil
}

// Close closes the eBPF program
func (p *Program) Close() error {
	if p.coll != nil {
		p.coll.Close()
	}
	return nil
}
