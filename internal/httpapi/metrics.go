package httpapi

import (
	"sync"
	"sync/atomic"
	"time"
)

type BatchResult struct {
	ID         uint64  `json:"id"`
	At         string  `json:"at"`
	Inserted   int     `json:"inserted"`
	Errors     int     `json:"errors"`
	DurationMs float64 `json:"duration_ms"`
	Status     string  `json:"status"`
}

type Metrics struct {
	started       time.Time
	totalInserted atomic.Uint64
	totalErrors   atomic.Uint64
	totalBatches  atomic.Uint64
	sequence      atomic.Uint64
	mu            sync.RWMutex
	lastDuration  float64
	lastInserted  int
	totalDuration float64
	recent        []BatchResult
}

func NewMetrics() *Metrics {
	return &Metrics{started: time.Now(), recent: make([]BatchResult, 0, 20)}
}

func (m *Metrics) Observe(inserted int, ok bool, duration time.Duration) {
	result := BatchResult{
		ID:         m.sequence.Add(1),
		At:         time.Now().UTC().Format(time.RFC3339),
		Inserted:   inserted,
		DurationMs: float64(duration) / float64(time.Millisecond),
		Status:     "success",
	}
	if !ok {
		result.Errors = 1
		result.Status = "error"
		m.totalErrors.Add(1)
	} else {
		m.totalInserted.Add(uint64(inserted))
		m.totalBatches.Add(1)
	}

	m.mu.Lock()
	m.record(result)
	m.mu.Unlock()
}

func (m *Metrics) ObserveInsert(ok bool, duration time.Duration) {
	result := BatchResult{
		ID:         m.sequence.Add(1),
		At:         time.Now().UTC().Format(time.RFC3339),
		Inserted:   1,
		DurationMs: float64(duration) / float64(time.Millisecond),
		Status:     "success",
	}
	if ok {
		m.totalInserted.Add(1)
	} else {
		result.Inserted = 0
		result.Errors = 1
		result.Status = "error"
		m.totalErrors.Add(1)
	}

	m.mu.Lock()
	m.record(result)
	m.mu.Unlock()
}

func (m *Metrics) record(result BatchResult) {
	m.lastDuration = result.DurationMs
	m.lastInserted = result.Inserted
	m.totalDuration += result.DurationMs
	m.recent = append([]BatchResult{result}, m.recent...)
	if len(m.recent) > 20 {
		m.recent = m.recent[:20]
	}
}

func (m *Metrics) Snapshot() map[string]any {
	m.mu.RLock()
	defer m.mu.RUnlock()
	recent := append([]BatchResult(nil), m.recent...)
	return map[string]any{
		"total_inserted":      m.totalInserted.Load(),
		"total_errors":        m.totalErrors.Load(),
		"total_batches":       m.totalBatches.Load(),
		"last_duration_ms":    m.lastDuration,
		"last_batch_inserted": m.lastInserted,
		"total_duration_ms":   m.totalDuration,
		"uptime_seconds":      int(time.Since(m.started).Seconds()),
		"recent_batches":      recent,
	}
}
