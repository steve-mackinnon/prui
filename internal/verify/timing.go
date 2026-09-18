package verify

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"
)

// OpenTimingEnvironment enables the private timing record emitted by open for
// the verifier. It is intentionally unset for normal user-facing commands.
const OpenTimingEnvironment = "PR_REVIEW_VERIFY_TIMING"

const openTimingPrefix = "pr-review-verify-timing:v1 "

// OpenTiming is the private process boundary between verify and open. Its
// durations remain typed until the report is serialized.
type OpenTiming struct {
	GitHubMetadata  time.Duration `json:"github_metadata_ns"`
	PinAndInventory time.Duration `json:"pin_and_inventory_ns"`
	ViewSetup       time.Duration `json:"view_setup_ns"`
	Fetch           time.Duration `json:"fetch_ns"`
	MergeBase       time.Duration `json:"merge_base_ns"`
	Inventory       time.Duration `json:"inventory_ns"`
	Evidence        time.Duration `json:"evidence_ns"`
}

func WriteOpenTiming(w io.Writer, timing OpenTiming) error {
	b, err := json.Marshal(timing)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(w, openTimingPrefix+string(b))
	return err
}

func parseOpenTiming(stderr []byte) (OpenTiming, error) {
	var result OpenTiming
	found := false
	for _, line := range strings.Split(string(stderr), "\n") {
		if !strings.HasPrefix(line, openTimingPrefix) {
			continue
		}
		if found {
			return OpenTiming{}, fmt.Errorf("multiple open timing records")
		}
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, openTimingPrefix)), &result); err != nil {
			return OpenTiming{}, fmt.Errorf("invalid open timing record: %w", err)
		}
		if result.GitHubMetadata < 0 || result.PinAndInventory < 0 || result.ViewSetup < 0 || result.Fetch < 0 || result.MergeBase < 0 || result.Inventory < 0 || result.Evidence < 0 {
			return OpenTiming{}, fmt.Errorf("invalid negative open timing")
		}
		found = true
	}
	if !found {
		return OpenTiming{}, fmt.Errorf("missing open timing record")
	}
	return result, nil
}

// CacheState describes whether a verifier run reused its private session store.
type CacheState string

const (
	ColdCache CacheState = "cold"
	WarmCache CacheState = "warm"
)

// TimingRun keeps durations typed while a verifier is running. Network timings
// are deliberately separate from local process startup timings.
type TimingRun struct {
	Cache   CacheState
	Local   LocalTiming
	Network NetworkTiming
}

type LocalTiming struct {
	ProcessStart     time.Duration
	FirstReviewFrame time.Duration
}

type NetworkTiming struct {
	GitHubMetadata  time.Duration
	PinAndInventory time.Duration
	Stages          map[string]time.Duration
}

type Timing struct {
	RunCount  int
	Runs      []TimingRun
	Aggregate TimingAggregate
}

type TimingAggregate struct {
	Local   LocalTimingAggregate
	Network NetworkTimingAggregate
}

type LocalTimingAggregate struct {
	ProcessStart     DurationAggregate
	FirstReviewFrame DurationAggregate
}

type NetworkTimingAggregate struct {
	GitHubMetadata  DurationAggregate
	PinAndInventory DurationAggregate
}

type DurationAggregate struct {
	Median time.Duration
	Max    time.Duration
}

func NewTimingRun(cache CacheState) (TimingRun, error) {
	if cache != ColdCache && cache != WarmCache {
		return TimingRun{}, fmt.Errorf("unknown cache state %q", cache)
	}
	return TimingRun{Cache: cache}, nil
}

// AggregateTiming produces deterministic median and maximum values. A zero
// duration means that lifecycle step did not run for that measurement.
func AggregateTiming(runs []TimingRun) Timing {
	result := Timing{RunCount: len(runs), Runs: append([]TimingRun(nil), runs...)}
	result.Aggregate = TimingAggregate{
		Local: LocalTimingAggregate{
			ProcessStart:     aggregateDurations(runs, func(run TimingRun) time.Duration { return run.Local.ProcessStart }),
			FirstReviewFrame: aggregateDurations(runs, func(run TimingRun) time.Duration { return run.Local.FirstReviewFrame }),
		},
		Network: NetworkTimingAggregate{
			GitHubMetadata:  aggregateDurations(runs, func(run TimingRun) time.Duration { return run.Network.GitHubMetadata }),
			PinAndInventory: aggregateDurations(runs, func(run TimingRun) time.Duration { return run.Network.PinAndInventory }),
		},
	}
	return result
}

func aggregateDurations(runs []TimingRun, duration func(TimingRun) time.Duration) DurationAggregate {
	if len(runs) == 0 {
		return DurationAggregate{}
	}
	values := make([]time.Duration, len(runs))
	for i, run := range runs {
		values[i] = duration(run)
	}
	sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
	median := values[len(values)/2]
	if len(values)%2 == 0 {
		median = values[len(values)/2-1]/2 + values[len(values)/2]/2
	}
	return DurationAggregate{Median: median, Max: values[len(values)-1]}
}

func (t Timing) MarshalJSON() ([]byte, error) {
	type durationJSON struct {
		MedianMS int64 `json:"median_ms"`
		MaxMS    int64 `json:"max_ms"`
	}
	type localRunJSON struct {
		ProcessStartMS     int64 `json:"process_start_ms"`
		FirstReviewFrameMS int64 `json:"first_review_frame_ms"`
	}
	type networkRunJSON struct {
		GitHubMetadataMS  int64            `json:"github_metadata_ms"`
		PinAndInventoryMS int64            `json:"pin_and_inventory_ms"`
		Stages            map[string]int64 `json:"stages,omitempty"`
	}
	type runJSON struct {
		Cache   CacheState     `json:"cache"`
		Local   localRunJSON   `json:"local"`
		Network networkRunJSON `json:"network"`
	}
	type aggregateJSON struct {
		Local struct {
			ProcessStart     durationJSON `json:"process_start"`
			FirstReviewFrame durationJSON `json:"first_review_frame"`
		} `json:"local"`
		Network struct {
			GitHubMetadata  durationJSON `json:"github_metadata"`
			PinAndInventory durationJSON `json:"pin_and_inventory"`
		} `json:"network"`
	}
	aggregateDuration := func(d DurationAggregate) durationJSON {
		return durationJSON{MedianMS: d.Median.Milliseconds(), MaxMS: d.Max.Milliseconds()}
	}
	runs := make([]runJSON, len(t.Runs))
	for i, run := range t.Runs {
		stages := make(map[string]int64, len(run.Network.Stages))
		for name, duration := range run.Network.Stages {
			stages[name] = duration.Milliseconds()
		}
		runs[i] = runJSON{Cache: run.Cache,
			Local:   localRunJSON{ProcessStartMS: run.Local.ProcessStart.Milliseconds(), FirstReviewFrameMS: run.Local.FirstReviewFrame.Milliseconds()},
			Network: networkRunJSON{GitHubMetadataMS: run.Network.GitHubMetadata.Milliseconds(), PinAndInventoryMS: run.Network.PinAndInventory.Milliseconds(), Stages: stages},
		}
	}
	var aggregate aggregateJSON
	aggregate.Local.ProcessStart = aggregateDuration(t.Aggregate.Local.ProcessStart)
	aggregate.Local.FirstReviewFrame = aggregateDuration(t.Aggregate.Local.FirstReviewFrame)
	aggregate.Network.GitHubMetadata = aggregateDuration(t.Aggregate.Network.GitHubMetadata)
	aggregate.Network.PinAndInventory = aggregateDuration(t.Aggregate.Network.PinAndInventory)
	return json.Marshal(struct {
		RunCount  int           `json:"run_count"`
		Runs      []runJSON     `json:"runs"`
		Aggregate aggregateJSON `json:"aggregate"`
	}{RunCount: t.RunCount, Runs: runs, Aggregate: aggregate})
}
