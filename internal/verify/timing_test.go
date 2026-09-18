package verify

import (
	"encoding/json"
	"testing"
	"time"
)

func TestTimingAggregateSeparatesLocalAndNetworkMeasurements(t *testing.T) {
	timing := AggregateTiming([]TimingRun{
		{Cache: ColdCache, Local: LocalTiming{ProcessStart: 30 * time.Millisecond, FirstReviewFrame: 90 * time.Millisecond}, Network: NetworkTiming{GitHubMetadata: 400 * time.Millisecond, PinAndInventory: 800 * time.Millisecond}},
		{Cache: WarmCache, Local: LocalTiming{ProcessStart: 10 * time.Millisecond, FirstReviewFrame: 50 * time.Millisecond}, Network: NetworkTiming{GitHubMetadata: 200 * time.Millisecond, PinAndInventory: 600 * time.Millisecond}},
		{Cache: WarmCache, Local: LocalTiming{ProcessStart: 20 * time.Millisecond, FirstReviewFrame: 70 * time.Millisecond}, Network: NetworkTiming{GitHubMetadata: 300 * time.Millisecond, PinAndInventory: 700 * time.Millisecond}},
	})

	if timing.RunCount != 3 {
		t.Fatalf("run count = %d, want 3", timing.RunCount)
	}
	if timing.Aggregate.Local.ProcessStart.Median != 20*time.Millisecond || timing.Aggregate.Local.ProcessStart.Max != 30*time.Millisecond {
		t.Fatalf("local process start = %+v", timing.Aggregate.Local.ProcessStart)
	}
	if timing.Aggregate.Network.GitHubMetadata.Median != 300*time.Millisecond || timing.Aggregate.Network.GitHubMetadata.Max != 400*time.Millisecond {
		t.Fatalf("network metadata = %+v", timing.Aggregate.Network.GitHubMetadata)
	}
}

func TestTimingJSONUsesMillisecondsAndZeroForStepsThatDidNotRun(t *testing.T) {
	timing := AggregateTiming([]TimingRun{{
		Cache: ColdCache,
		Local: LocalTiming{ProcessStart: 1250 * time.Microsecond},
	}})

	b, err := json.Marshal(timing)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"run_count":1,"runs":[{"cache":"cold","local":{"process_start_ms":1,"first_review_frame_ms":0},"network":{"github_metadata_ms":0,"pin_and_inventory_ms":0}}],"aggregate":{"local":{"process_start":{"median_ms":1,"max_ms":1},"first_review_frame":{"median_ms":0,"max_ms":0}},"network":{"github_metadata":{"median_ms":0,"max_ms":0},"pin_and_inventory":{"median_ms":0,"max_ms":0}}}}`
	if string(b) != want {
		t.Fatalf("timing JSON = %s\nwant %s", b, want)
	}
}

func TestAggregateTimingRejectsUnknownCacheState(t *testing.T) {
	if _, err := NewTimingRun(CacheState("other")); err == nil {
		t.Fatal("NewTimingRun accepted unknown cache state")
	}
}

func TestTimingAggregateUsesArithmeticMedianForAnEvenRunCount(t *testing.T) {
	timing := AggregateTiming([]TimingRun{
		{Cache: ColdCache, Local: LocalTiming{ProcessStart: 10 * time.Millisecond}},
		{Cache: WarmCache, Local: LocalTiming{ProcessStart: 30 * time.Millisecond}},
	})
	if got, want := timing.Aggregate.Local.ProcessStart.Median, 20*time.Millisecond; got != want {
		t.Fatalf("median = %s, want %s", got, want)
	}
}

func TestReportIncludesTimingWhenMeasured(t *testing.T) {
	timing := AggregateTiming([]TimingRun{{Cache: ColdCache, Local: LocalTiming{ProcessStart: time.Millisecond}}})
	b, err := json.Marshal(Report{Status: Passed, Timing: &timing})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(b), `{"schema_version":1,"status":"passed","checks":null,"artifacts":{},"timing":{"run_count":1,"runs":[{"cache":"cold","local":{"process_start_ms":1,"first_review_frame_ms":0},"network":{"github_metadata_ms":0,"pin_and_inventory_ms":0}}],"aggregate":{"local":{"process_start":{"median_ms":1,"max_ms":1},"first_review_frame":{"median_ms":0,"max_ms":0}},"network":{"github_metadata":{"median_ms":0,"max_ms":0},"pin_and_inventory":{"median_ms":0,"max_ms":0}}}}}`; got != want {
		t.Fatalf("report JSON = %s\nwant %s", got, want)
	}
}
