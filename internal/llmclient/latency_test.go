package llmclient

import (
	"testing"
	"time"
)

func TestLatencyTracker_FirstSampleSeedsValue(t *testing.T) {
	tr := NewLatencyTracker()
	tr.Record(100*time.Millisecond, false)
	if got := tr.P50(); got != 100*time.Millisecond {
		t.Fatalf("first sample should seed value, got %v", got)
	}
}

func TestLatencyTracker_ConvergesTowardRecentSamples(t *testing.T) {
	tr := NewLatencyTracker()
	tr.Record(1000*time.Millisecond, false) // seed
	// Feed many short samples; EWMA should move well below the seed.
	for i := 0; i < 50; i++ {
		tr.Record(10*time.Millisecond, false)
	}
	if got := tr.P50(); got > 200*time.Millisecond {
		t.Fatalf("expected p50 to converge toward 10ms, got %v", got)
	}
}

func TestLatencyTracker_ErrorRateReflectsFailures(t *testing.T) {
	tr := NewLatencyTracker()
	for i := 0; i < 5; i++ {
		tr.Record(10*time.Millisecond, i%2 == 0) // 3 errors of 5
	}
	rate := tr.ErrorRate()
	if rate < 0.3 || rate > 0.9 {
		t.Fatalf("expected error rate near 0.4-0.6, got %v", rate)
	}
}

func TestLatencyTracker_LeadingSuccessesDoNotSkewErrorRate(t *testing.T) {
	tr := NewLatencyTracker()
	// Several successes before one failure: the error rate should stay low,
	// not spike to 0.5+. This regression test guards against the older
	// value==0 initialization sentinel that treated a zero first sample as
	// uninitialized then blended the failure 50/50 with a phantom zero.
	for i := 0; i < 9; i++ {
		tr.Record(10*time.Millisecond, false) // success
	}
	tr.Record(10*time.Millisecond, true) // one failure
	rate := tr.ErrorRate()
	if rate > 0.5 {
		t.Fatalf("expected error rate well below 0.5 after 9/10 success, got %v", rate)
	}
}

func TestLatencyTracker_P50BeforeSamples(t *testing.T) {
	tr := NewLatencyTracker()
	if got := tr.P50(); got != 0 {
		t.Fatalf("expected zero p50 before samples, got %v", got)
	}
	if got := tr.ErrorRate(); got != 0 {
		t.Fatalf("expected zero error rate before samples, got %v", got)
	}
}

func TestLatencyTracker_ConcurrentSafe(t *testing.T) {
	tr := NewLatencyTracker()
	done := make(chan struct{})
	for g := 0; g < 10; g++ {
		go func() {
			for i := 0; i < 100; i++ {
				tr.Record(time.Millisecond, false)
				_ = tr.P50()
				_ = tr.ErrorRate()
			}
			done <- struct{}{}
		}()
	}
	for g := 0; g < 10; g++ {
		<-done
	}
}
