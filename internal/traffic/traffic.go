package traffic

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Sample is one interval of interface throughput in bytes per second.
type Sample struct {
	Rx int64 `json:"rx"`
	Tx int64 `json:"tx"`
}

// Snapshot is one view of the recorded traffic history, oldest first.
type Snapshot struct {
	Samples []Sample `json:"samples"`
	RxTotal int64    `json:"rx_total"`
	TxTotal int64    `json:"tx_total"`
}

// historyLen is the number of one second samples the sampler keeps.
const historyLen = 600

// Sampler records interface throughput once per second from the moment it
// starts, so a chart always has history ready when a client asks.
type Sampler struct {
	mu      sync.Mutex
	device  string
	samples []Sample
	rxTotal int64
	txTotal int64
	rxLast  int64
	txLast  int64
	seen    bool
}

// New starts sampling the given network interface in the background.
func New(device string) *Sampler {
	s := &Sampler{device: device, samples: make([]Sample, 0, historyLen)}
	go s.run()
	return s
}

// Snapshot returns a copy of the recorded history and the totals.
func (s *Sampler) Snapshot() Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := Snapshot{
		Samples: append([]Sample(nil), s.samples...),
		RxTotal: s.rxTotal,
		TxTotal: s.txTotal,
	}
	return out
}

func (s *Sampler) run() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for range ticker.C {
		s.sample()
	}
}

func (s *Sampler) sample() {
	rx, okRx := s.readCounter("rx_bytes")
	tx, okTx := s.readCounter("tx_bytes")
	s.mu.Lock()
	defer s.mu.Unlock()
	if !okRx || !okTx {
		if len(s.samples) > 0 || s.seen {
			s.samples = append(s.samples, Sample{})
			s.trim()
		}
		s.seen = false
		return
	}
	sample := Sample{}
	if s.seen {
		sample.Rx = max(rx-s.rxLast, 0)
		sample.Tx = max(tx-s.txLast, 0)
		s.rxTotal += sample.Rx
		s.txTotal += sample.Tx
	}
	s.seen = true
	s.rxLast = rx
	s.txLast = tx
	s.samples = append(s.samples, sample)
	s.trim()
}

func (s *Sampler) trim() {
	if len(s.samples) > historyLen {
		s.samples = s.samples[len(s.samples)-historyLen:]
	}
}

// readCounter reads one raw kernel traffic counter of the sampled
// interface.
func (s *Sampler) readCounter(name string) (int64, bool) {
	data, err := os.ReadFile(filepath.Join("/sys/class/net", s.device, "statistics", name))
	if err != nil {
		return 0, false
	}
	value, err := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
	if err != nil {
		return 0, false
	}
	return value, true
}
