package autopilot

import (
	"testing"
	"time"
)

func TestAutopilotShapesKeepOrdinaryRequestsIndependent(t *testing.T) {
	d := DemandTracker{}
	now := time.Now()
	ordinary := DemandSample{Model: "mixed", ReceivedAt: now.Add(-time.Second), PromptTokens: 100, RequestedMaxTokens: 128}
	special := ordinary
	special.RequiresVision = true
	special.HasTools = true
	special.PromptTokens = 32000
	for range 20 {
		d.Record(ordinary, now, 5*time.Minute)
	}
	d.Record(special, now, 5*time.Minute)
	shapes := d.ShapeSnapshot(now, 5*time.Minute)
	if len(shapes) != 2 {
		t.Fatalf("cohorts=%+v", shapes)
	}
	a, b := shapes[ShapeKey(ordinary)], shapes[ShapeKey(special)]
	if a.RequiresVision || a.HasTools || a.Requests != 20 || !b.RequiresVision || b.Requests != 1 {
		t.Fatalf("shape requirements leaked: %+v", shapes)
	}
	node := Node{ID: "plain", Residents: []string{"mixed"}}
	node.Fits = map[string]ModelFit{ShapeKey(ordinary): ModelFit{Rate: 10, ServiceSeconds: 1, MeetsDeadline: true}}
	capacity := NodeContribution(node, node.Residents, shapes)
	if capacity[ShapeKey(ordinary)] <= 0 || capacity[ShapeKey(special)] != 0 {
		t.Fatalf("capacity=%+v", capacity)
	}
}
