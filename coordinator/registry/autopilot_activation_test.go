package registry

import (
	"context"
	"testing"
	"time"

	"github.com/eigeninference/d-inference/coordinator/protocol"
)

func TestAutopilotConsentAloneDoesNotChangeServing(t *testing.T) {
	r, _, now := newAutopilotControllerTest(t, false)
	p := autopilotControllerProvider(t, r, "provider", now)
	p.mu.Lock()
	defer p.mu.Unlock()
	p.ModelAutopilot.Active = false
	if providerAutopilotRoutingBlockedLocked(p, autopilotTestTarget) || providerLegacyModelChangesBlockedLocked(p) {
		t.Fatal("consent without activation changed serving")
	}
	p.ModelAutopilot.Active = true
	p.autopilotControlUntil = now.Add(-time.Second)
	if providerAutopilotManagedLocked(p) {
		t.Fatal("expired lease retained managed ownership")
	}
	p.autopilotControlUntil = now.Add(time.Minute)
	p.ModelAutopilot.Revision = "changed"
	if providerAutopilotManagedLocked(p) {
		t.Fatal("old session activated a changed selection")
	}
}

func TestAutopilotPauseStopsReservationsAndPreservesPending(t *testing.T) {
	r, c, now := newAutopilotControllerTest(t, false)
	p := autopilotControllerProvider(t, r, "provider", now)
	action := autopilotControllerPlan(t, r, c, now)
	command, ok := r.reserveAutopilotAction(c, action, now)
	if !ok {
		t.Fatal("reserve")
	}
	r.SetAutopilotPaused(true)
	if _, ok := r.reserveAutopilotAction(c, action, now); ok {
		t.Fatal("paused controller issued new operation")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.autopilotPending == nil || p.autopilotPending.Command.CommandID != command.CommandID {
		t.Fatal("pause discarded in-flight ownership")
	}
}

func TestAutopilotShapesKeepOrdinaryRequestsIndependent(t *testing.T) {
	d := autopilotDemandTracker{}
	now := time.Now()
	ordinary := AutopilotDemandSample{Model: "mixed", ReceivedAt: now.Add(-time.Second), PromptTokens: 100, RequestedMaxTokens: 128}
	special := ordinary
	special.RequiresVision = true
	special.HasTools = true
	special.PromptTokens = 32000
	for range 20 {
		d.record(ordinary, now, 5*time.Minute)
	}
	d.record(special, now, 5*time.Minute)
	shapes := d.shapeSnapshot(now, 5*time.Minute)
	if len(shapes) != 2 {
		t.Fatalf("cohorts=%+v", shapes)
	}
	a, b := shapes[autopilotShapeKey(ordinary)], shapes[autopilotShapeKey(special)]
	if a.RequiresVision || a.HasTools || a.Requests != 20 || !b.RequiresVision || b.Requests != 1 {
		t.Fatalf("shape requirements leaked: %+v", shapes)
	}
	node := autopilotPlannerNode("plain", "mixed")
	node.Fits = map[string]autopilotModelFit{autopilotShapeKey(ordinary): autopilotPlannerFit(10, 1)}
	capacity := autopilotNodeContribution(node, node.Residents, shapes)
	if capacity[autopilotShapeKey(ordinary)] <= 0 || capacity[autopilotShapeKey(special)] != 0 {
		t.Fatalf("capacity=%+v", capacity)
	}
}

func TestAutopilotSnapshotKeepsLoadMeasurementAfterUnload(t *testing.T) {
	r, c, now := newAutopilotControllerTest(t, false)
	p := autopilotControllerProvider(t, r, "provider", now)
	p.mu.Lock()
	defer p.mu.Unlock()
	p.Models[0].WeightHash = "verified"
	p.ModelAutopilot.LoadHistory = []protocol.ModelAutopilotLoadTiming{{ModelID: autopilotTestTarget, WeightHash: "verified", LoadMS: 2700, MeasuredAtMS: now.Add(-time.Minute).UnixMilli()}}
	fit := r.autopilotModelFitLocked(p, autopilotTestTarget, autopilotDemandView{}, c.config)
	if fit.LoadSeconds != 2.7 {
		t.Fatalf("load history not used: %+v", fit)
	}
	p.ModelAutopilot.LoadHistory[0].WeightHash = "other-build"
	fit = r.autopilotModelFitLocked(p, autopilotTestTarget, autopilotDemandView{}, c.config)
	if fit.LoadSeconds != c.config.LoadTimePrior.Seconds() {
		t.Fatal("measurement from different bytes reused")
	}
}

func TestAutopilotPreservesConfiguredFloorsWhenWarmPoolDisabled(t *testing.T) {
	r := New(testLogger())
	cfg := testWarmPoolConfig()
	cfg.Enabled = false
	cfg.MinWarmByModel = map[string]int{"protected": 2}
	stop := r.StartWarmPoolController(context.Background(), cfg)
	defer stop()
	if err := r.ConfigureAutopilot(DefaultAutopilotConfig()); err != nil {
		t.Fatal(err)
	}
	f := r.autopilotFleetSnapshot(r.autopilot, time.Now())
	if f.Floors["protected"] != 2 {
		t.Fatalf("configured floor lost: %+v", f.Floors)
	}
	if r.RequestWarmPoolTrigger() || len(r.TriggerWarmPool()) != 0 {
		t.Fatal("disabled warm pool issued work")
	}
}

func TestAutopilotUsesResolvedAccountDeadlineWithoutAccountIdentity(t *testing.T) {
	r, c, now := newAutopilotControllerTest(t, false)
	p := autopilotControllerProvider(t, r, "provider", now)
	p.mu.Lock()
	defer p.mu.Unlock()
	d := autopilotDemandView{Requests: 10, PromptTokens: 1024, TailPromptTokens: 1024, RequestedMaxTokens: 64, DeadlineKnown: true, DeadlineSeconds: .001}
	if r.autopilotModelFitLocked(p, autopilotTestTarget, d, c.config).MeetsDeadline {
		t.Fatal("tight resolved SLA ignored")
	}
	d.DeadlineSeconds = 0
	if !r.autopilotModelFitLocked(p, autopilotTestTarget, d, c.config).MeetsDeadline {
		t.Fatal("deadline invented for exempt requests")
	}
	sample := AutopilotDemandSample{Model: "model", PromptTokens: 100, RequestedMaxTokens: 64, DeadlineKnown: true}
	exempt := autopilotShapeKey(sample)
	sample.FirstContentDeadline = 5 * time.Second
	if exempt == autopilotShapeKey(sample) {
		t.Fatal("SLA and exempt requests share a cohort")
	}
	tracker := autopilotDemandTracker{}
	sample.ReceivedAt = now.Add(-time.Second)
	tracker.record(sample, now, 5*time.Minute)
	sample.FirstContentDeadline = 0
	tracker.record(sample, now, 5*time.Minute)
	views := tracker.shapeSnapshot(now, 5*time.Minute)
	if !views[exempt].DeadlineKnown || views[exempt].DeadlineSeconds != 0 {
		t.Fatal("exempt deadline lost during aggregation")
	}
	sample.FirstContentDeadline = 5 * time.Second
	if views[autopilotShapeKey(sample)].DeadlineSeconds != 5 {
		t.Fatal("SLA budget lost during aggregation")
	}
}
