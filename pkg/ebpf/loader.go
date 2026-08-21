package ebpf

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"os"
	"sync"

	ebpf "github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/ringbuf"
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

func (e *TrafficEvent) SourceIP() net.IP {
	return ipv4FromUint32(e.SAddr)
}

func (e *TrafficEvent) DestinationIP() net.IP {
	return ipv4FromUint32(e.DAddr)
}

func (e *TrafficEvent) ProtocolName() string {
	switch e.Protocol {
	case 6:
		return "tcp"
	case 17:
		return "udp"
	default:
		return fmt.Sprintf("ip-%d", e.Protocol)
	}
}

// Program represents the eBPF program
type Program struct {
	coll    *ebpf.Collection
	links   []link.Link
	reader  *ringbuf.Reader
	logger  *zap.SugaredLogger
	mu      sync.RWMutex
	events  chan *TrafficEvent
	stopCh  chan struct{}
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
		stopCh: make(chan struct{}),
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

	eventsMap := p.coll.Maps["events"]
	if eventsMap == nil {
		p.mu.Lock()
		p.running = false
		p.mu.Unlock()
		return fmt.Errorf("eBPF events map not found")
	}

	reader, err := ringbuf.NewReader(eventsMap)
	if err != nil {
		p.mu.Lock()
		p.running = false
		p.mu.Unlock()
		return fmt.Errorf("failed to create eBPF ring buffer reader: %w", err)
	}
	p.reader = reader

	// Start event read loop
	go p.readEvents(ctx)

	return nil
}

func (p *Program) attachProbes() error {
	tcpProgram := p.coll.Programs["trace_tcp_sendmsg"]
	if tcpProgram == nil {
		return fmt.Errorf("eBPF program trace_tcp_sendmsg not found")
	}
	tcpLink, err := link.Kprobe("tcp_sendmsg", tcpProgram, nil)
	if err != nil {
		return fmt.Errorf("failed to attach tcp_sendmsg kprobe: %w", err)
	}
	p.links = append(p.links, tcpLink)
	p.logger.Infow("Attached TCP sendmsg probe")

	udpProgram := p.coll.Programs["trace_udp_sendmsg"]
	if udpProgram == nil {
		return fmt.Errorf("eBPF program trace_udp_sendmsg not found")
	}
	udpLink, err := link.Kprobe("udp_sendmsg", udpProgram, nil)
	if err != nil {
		return fmt.Errorf("failed to attach udp_sendmsg kprobe: %w", err)
	}
	p.links = append(p.links, udpLink)
	p.logger.Infow("Attached UDP sendmsg probe")

	return nil
}

func (p *Program) readEvents(ctx context.Context) {
	defer close(p.events)

	p.logger.Debugw("Event reader started")

	for {
		record, err := p.reader.Read()
		if err != nil {
			if errors.Is(err, ringbuf.ErrClosed) || errors.Is(err, os.ErrClosed) {
				return
			}
			p.logger.Debugw("Error reading eBPF event", "error", err)
			continue
		}

		event, err := decodeTrafficEvent(record.RawSample)
		if err != nil {
			p.logger.Debugw("Error decoding eBPF event", "error", err)
			continue
		}

		select {
		case p.events <- event:
		case <-ctx.Done():
			return
		case <-p.stopCh:
			return
		}
	}
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
	close(p.stopCh)
	if p.reader != nil {
		_ = p.reader.Close()
	}
	p.mu.Unlock()

	for _, attachedLink := range p.links {
		if err := attachedLink.Close(); err != nil {
			p.logger.Debugw("Error closing eBPF link", "error", err)
		}
	}
	p.links = nil

	if p.coll != nil {
		p.coll.Close()
	}

	p.logger.Infow("eBPF program stopped")
	return nil
}

// Close closes the eBPF program
func (p *Program) Close() error {
	if p.reader != nil {
		_ = p.reader.Close()
	}
	for _, attachedLink := range p.links {
		_ = attachedLink.Close()
	}
	if p.coll != nil {
		p.coll.Close()
	}
	return nil
}

func decodeTrafficEvent(sample []byte) (*TrafficEvent, error) {
	if len(sample) < 40 {
		return nil, fmt.Errorf("traffic event sample too short: got %d bytes", len(sample))
	}

	return &TrafficEvent{
		Timestamp: binary.LittleEndian.Uint64(sample[0:8]),
		SAddr:     binary.LittleEndian.Uint32(sample[8:12]),
		DAddr:     binary.LittleEndian.Uint32(sample[12:16]),
		SPort:     binary.LittleEndian.Uint16(sample[16:18]),
		DPort:     binary.LittleEndian.Uint16(sample[18:20]),
		Protocol:  sample[20],
		PID:       binary.LittleEndian.Uint32(sample[24:28]),
		TID:       binary.LittleEndian.Uint32(sample[28:32]),
		BytesSent: binary.LittleEndian.Uint64(sample[32:40]),
	}, nil
}

func ipv4FromUint32(value uint32) net.IP {
	return net.IPv4(byte(value), byte(value>>8), byte(value>>16), byte(value>>24))
}
