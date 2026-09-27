package registry

import (
	"github.com/eigeninference/d-inference/coordinator/registry/autopilot"
	"time"
)

func publicAutopilotPending(p *PendingRequest) bool {
	if p == nil || p.SelfRouteOnly || p.PreferOwner || len(p.AllowedProviderSerials) > 0 || len(p.ExcludedProviderIDs) > 0 {
		return false
	}
	profile := p.Profile
	if profile != nil && profile.ProviderCompleteObserved.Load() {
		return false
	}
	parent := profile.Parent()
	return parent == nil || (parent.ClientGoneUS.Load() == 0 && parent.DoneFlushedUS.Load() == 0)
}

// Registry membership is locked by the caller; copy immutable request fields
// under each provider lock. Opaque IDs exist only for transient queue/active
// deduplication and never enter the policy package, logs or demand history.
func (r *Registry) autopilotActiveSamplesLocked() ([]autopilot.DemandSample, map[string]bool, map[string]bool) {
	var samples []autopilot.DemandSample
	ids, unscoped := map[string]bool{}, map[string]bool{}
	for _, p := range r.providers {
		p.mu.Lock()
		accepted := map[string]int{}
		if !p.PrivateOnly {
			for id, request := range p.pendingReqs {
				if !publicAutopilotPending(request) {
					continue
				}
				profile := request.Profile
				parent := profile.Parent()
				// Enabled Autopilot observations create compact profiles even
				// with heavy profiling off. Incomplete provenance stays unscoped.
				if parent == nil || parent.T0.IsZero() || parent.HandlerEntryUS.Load() <= 0 {
					continue
				}
				sample := autopilot.DemandSample{Model: request.Model, PromptTokens: request.EstimatedPromptTokens,
					RequestedMaxTokens: request.RequestedMaxTokens,
					Requirements:       request.Traits.AutopilotRequirements(request.RequiresVision), DeadlineKnown: true}
				if parent != nil && parent.HandlerEntryUS.Load() > 0 {
					sample.ReceivedAt = parent.T0.Add(time.Duration(parent.HandlerEntryUS.Load()) * time.Microsecond)
				}
				if !request.FirstContentDeadline.IsZero() {
					// The request-start stamp is immutable; never read mutable
					// attempt budgets or dispatch-owned timing fields here.
					if sample.ReceivedAt.IsZero() {
						continue
					}
					sample.FirstContentDeadline = request.FirstContentDeadline.Sub(sample.ReceivedAt)
				}
				if !sample.ValidEnvelope() || sample.FirstContentDeadline < 0 {
					continue
				}
				samples = append(samples, sample)
				ids[id] = true
				if profile != nil && profile.AcceptedUS.Load() > 0 {
					accepted[request.Model]++
				}
			}
		}
		if p.BackendCapacity != nil {
			for _, slot := range p.BackendCapacity.Slots {
				if max(0, slot.NumRunning)+max(0, slot.NumWaiting) > accepted[slot.Model] {
					// Unattributed/local/private work retains its GPU claim but
					// creates neither public demand nor spare public capacity.
					unscoped[p.ID] = true
					break
				}
			}
		}
		p.mu.Unlock()
	}
	return samples, ids, unscoped
}
