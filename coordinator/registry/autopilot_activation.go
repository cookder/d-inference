package registry

import (
	"context"
	"encoding/json"
	"slices"
	"time"

	"github.com/eigeninference/d-inference/coordinator/protocol"
)

func providerAutopilotConsentedLocked(p *Provider) bool {
	s := p.ModelAutopilot
	return s != nil && s.Protocol == protocol.ModelAutopilotProtocol && s.Enabled &&
		s.CachedOnly && s.Revision != "" && len(s.Revision) <= 64 &&
		len(s.SelectedModels) > 0 && len(s.SelectedModels) <= 256
}

func providerAutopilotAllowsLocked(p *Provider, model string) bool {
	return providerAutopilotConsentedLocked(p) && slices.Contains(p.ModelAutopilot.SelectedModels, model)
}

// Pausing stops reservations immediately; accepted operations retain their
// owner, watchdog and heartbeat reconciliation until the final state is known.
func (r *Registry) SetAutopilotPaused(paused bool) bool {
	r.mu.RLock()
	c := r.autopilot
	r.mu.RUnlock()
	if c == nil {
		return false
	}
	c.paused.Store(paused)
	return true
}

func (c *modelAutopilotController) refreshControlLeases(now time.Time) {
	if c.config.ObserveOnly {
		return
	}
	type delivery struct {
		p       *Provider
		message protocol.ModelAutopilotControl
	}
	var pending []delivery
	r := c.registry
	r.mu.RLock()
	for _, p := range r.providers {
		p.mu.Lock()
		if providerAutopilotConsentedLocked(p) {
			active := c.config.Enabled && !c.config.ObserveOnly && !c.paused.Load() &&
				!p.ModelAutopilot.Paused && !p.PrivateOnly
			expiry := now.Add(3*c.config.Interval + 10*time.Second)
			if !active {
				expiry = now
			}
			p.autopilotControlUntil = expiry
			p.autopilotControlRevision = p.ModelAutopilot.Revision
			pending = append(pending, delivery{p, protocol.ModelAutopilotControl{
				Type: protocol.TypeModelAutopilotControl, SessionID: p.ID,
				Revision: p.ModelAutopilot.Revision, Enabled: active, ExpiresAtMS: expiry.UnixMilli(),
			}})
		}
		p.mu.Unlock()
	}
	r.mu.RUnlock()
	for _, d := range pending {
		body, err := json.Marshal(d.message)
		if err != nil {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), providerControlWriteTimeout)
		_ = d.p.WriteTextControl(ctx, body)
		cancel()
	}
}
