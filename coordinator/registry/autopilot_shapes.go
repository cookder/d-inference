package registry

import (
	"fmt"
	"strings"
	"time"
)

// Cohorts are bounded, prompt-free capability and work-size bins. The model
// remains an exact catalog build; a cohort key is never sent to a provider.
func autopilotModel(key string) string { model, _, _ := strings.Cut(key, "\x1f"); return model }
func autopilotShapeKey(s AutopilotDemandSample) string {
	flags := 0
	if s.RequiresVision {
		flags |= 1
	}
	if s.HasTools {
		flags |= 2
	}
	if s.RequiresToolConstraint {
		flags |= 4
	}
	input, output := 0, 0
	for input < len(autopilotPromptBounds)-1 && s.PromptTokens > autopilotPromptBounds[input] {
		input++
	}
	for output < 3 && s.RequestedMaxTokens > []int{256, 2048, 8192}[output] {
		output++
	}
	deadline := -1
	if s.DeadlineKnown {
		deadline = 0
		if s.FirstContentDeadline > 0 {
			deadline = 1
			for deadline <= 5 && s.FirstContentDeadline.Seconds() > []float64{5, 15, 30, 60, 120}[deadline-1] {
				deadline++
			}
		}
	}
	return fmt.Sprintf("%s\x1f%d:%d:%d:%d", s.Model, flags, input, output, deadline)
}

func (d *autopilotDemandTracker) shapeSnapshot(now time.Time, window time.Duration) map[string]autopilotDemandView {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := map[string]autopilotDemandView{}
	for key, tracker := range d.shapes {
		samples := tracker.snapshot(now, window)
		if sample, ok := samples[autopilotModel(key)]; ok && sample.Requests > 0 {
			out[key] = sample
		} else {
			delete(d.shapes, key)
		}
	}
	return out
}

func autopilotDemandForModel(demand map[string]autopilotDemandView, model string) autopilotDemandView {
	var out autopilotDemandView
	for key, d := range demand {
		if autopilotModel(key) != model {
			continue
		}
		out.Rate += d.Rate
		out.Requests += d.Requests
		out.CapacityShed += d.CapacityShed
		if d.LastDemand.After(out.LastDemand) {
			out.LastDemand = d.LastDemand
		}
	}
	return out
}

func autopilotDemandShare(f autopilotFleet, key string) float64 {
	total := autopilotDemandForModel(f.Demand, autopilotModel(key)).Rate
	if total <= 0 {
		return 1
	}
	return f.Demand[key].Rate / total
}

func autopilotShapeLabel(key string) string { _, shape, _ := strings.Cut(key, "\x1f"); return shape }
