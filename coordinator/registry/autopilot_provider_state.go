package registry

import (
	"time"

	"github.com/eigeninference/d-inference/coordinator/autopilot"
	"github.com/eigeninference/d-inference/coordinator/protocol"
	"github.com/eigeninference/d-inference/coordinator/store"
)

func providerAutopilotManagedLocked(p *Provider) bool {
	return providerAutopilotConsentedLocked(p) && (p.ModelAutopilot.Paused || (p.ModelAutopilot.Active &&
		p.ModelAutopilot.SessionID == p.ID && p.ModelAutopilot.Revision == p.autopilotControlRevision && time.Now().Before(p.autopilotControlUntil)))
}
func providerAutopilotTransitionLocked(p *Provider) bool {
	return p.autopilotPending != nil || (p.ModelAutopilot != nil && p.ModelAutopilot.ActiveCommandID != "")
}

func autopilotStateMatchesCapacity(p *Provider) bool {
	return autopilot.StateMatchesCapacity(p.ModelAutopilot, p.BackendCapacity, p.capacitySeq)
}

// Called only after the accepted capacity-sequence gate, under p.mu. Status
// messages alone never clear reservations or manufacture warm slot capacity.
func (r *Registry) reconcileAutopilotHeartbeatLocked(p *Provider, state *protocol.ModelAutopilotState, now time.Time) {
	p.ModelAutopilot = autopilot.CloneState(state)
	pending := p.autopilotPending
	if pending == nil || state == nil || p.capacitySeq <= pending.CapacitySeq || state.ActiveCommandID != "" || state.LastCommandID != pending.Command.CommandID {
		return
	}
	if state.LastCommandStatus != protocol.LoadModelStatusSucceeded && state.LastCommandStatus != protocol.LoadModelStatusFailed {
		return
	}
	// A disabled report can acknowledge opt-out completion; its resident list
	// must still exactly match the same accepted backend snapshot.
	enabled := state.Enabled
	if !enabled {
		copy := autopilot.CloneState(state)
		copy.Enabled = true
		p.ModelAutopilot = copy
	}
	matches := autopilotStateMatchesCapacity(p)
	p.ModelAutopilot = autopilot.CloneState(state)
	if !matches {
		return
	}
	if state.LastCommandStatus == protocol.LoadModelStatusFailed {
		backoff := pending.FailureBackoff
		if backoff <= 0 {
			backoff = 2 * time.Minute
		}
		p.autopilotBackoffUntil = now.Add(backoff)
	}
	r.queueAutopilotEvent(store.AutopilotRecord{CommandID: pending.Command.CommandID, At: now, ProviderID: p.ID, Phase: state.LastCommandStatus, Load: pending.Command.LoadModelID, Unload: pending.Command.UnloadModelIDs, Before: pending.Command.ExpectedResidentModels, After: autopilot.ResidentIDs(state), ElapsedMS: now.Sub(pending.SentAt).Milliseconds(), LoadMS: max(0, min(state.LastLoadMS, 1800000)), ReleaseMS: max(0, min(state.LastReleaseMS, 1800000))})
	p.autopilotPending = nil
}
