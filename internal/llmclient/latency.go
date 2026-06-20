package llmclient

import (
	"sync"
	"time"
)

// defaultLatencyAlpha is the EWMA smoothing factor used for p50 latency.
// A smaller alpha makes the average more responsive to recent changes.
const defaultLatencyAlpha = 0.1

// ewma is a concurrency-safe exponentially weighted moving average.
// The first sample initializes the value directly (tracked by the
// initialized bool so that a genuine zero-valued sample is not
// mistaken for the uninitialized state); subsequent samples are
// smoothed using value = alpha*sample + (1-alpha)*value.
type ewma struct {
	mu          sync.Mutex
	value       float64
	alpha       float64
	initialized bool
}

// newEWMA returns an EWMA with the given smoothing factor alpha.
// alpha must be in the range (0, 1]; values closer to 1 weight recent
// samples more heavily.
func newEWMA(alpha float64) *ewma {
	return &ewma{alpha: alpha}
}

// Add incorporates a new sample into the moving average. The first sample
// seeds the value directly so the average is not biased toward zero.
func (e *ewma) Add(sample float64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.initialized {
		e.value = sample
		e.initialized = true
		return
	}
	e.value = e.alpha*sample + (1-e.alpha)*e.value
}

// Value returns the current EWMA value. Returns zero before any sample
// has been added.
func (e *ewma) Value() float64 {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.value
}

// LatencyTracker maintains EWMA-based latency and error-rate statistics
// for a provider or model. It is intended to feed latency-aware routing
// decisions with cheap, in-memory, concurrency-safe signals.
//
// WARNING: Statistics are in-memory only — they reset on restart and are
// NOT shared across replicas. This is by design: the tracker produces
// approximate relative rankings for routing decisions, not absolute SLA
// measurements. Callers should call Record once per completed request.
//
// Smoothing factors:
//   - p50 uses alpha=0.1 for a responsive mid-latency signal
//   - p99 uses alpha=0.05 for a smoother tail-latency signal
//   - errorRate uses alpha=0.2 to react quickly to error bursts
//
// Note: the EWMA-based p50/p99 are approximate percentiles rather than
// true percentile estimators; they are sufficient for relative comparison
// across providers and not intended for SLA reporting.
type LatencyTracker struct {
	p50       *ewma
	p99       *ewma
	errorRate *ewma
}

// NewLatencyTracker returns a LatencyTracker with default smoothing factors
// tuned for latency-aware routing.
func NewLatencyTracker() *LatencyTracker {
	return &LatencyTracker{
		p50:       newEWMA(defaultLatencyAlpha),
		p99:       newEWMA(defaultLatencyAlpha / 2),
		errorRate: newEWMA(0.2),
	}
}

// Record observes the duration and error status of a completed request.
// duration is the total request latency; isError indicates whether the
// request was considered failed for routing purposes.
func (t *LatencyTracker) Record(duration time.Duration, isError bool) {
	ms := float64(duration.Nanoseconds()) / 1e6
	t.p50.Add(ms)
	t.p99.Add(ms)
	t.errorRate.Add(boolToFloat64(isError))
}

// P50 returns the approximate p50 latency as a time.Duration.
// Returns zero before any sample has been recorded.
func (t *LatencyTracker) P50() time.Duration {
	return time.Duration(t.p50.Value()) * time.Millisecond
}

// P99 returns the approximate p99 latency as a time.Duration.
// Returns zero before any sample has been recorded.
func (t *LatencyTracker) P99() time.Duration {
	return time.Duration(t.p99.Value()) * time.Millisecond
}

// ErrorRate returns the EWMA-smoothed error rate in the range [0, 1].
// Returns zero before any sample has been recorded.
func (t *LatencyTracker) ErrorRate() float64 {
	return t.errorRate.Value()
}

// boolToFloat64 converts a boolean to 1.0 (true) or 0.0 (false) for use
// as an EWMA sample.
func boolToFloat64(b bool) float64 {
	if b {
		return 1
	}
	return 0
}
