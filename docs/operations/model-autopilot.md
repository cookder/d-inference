# Experimental Autopilot operation and recovery

> Last updated: 2026-09-27 · commit `becb09c8a`

Use this runbook to operate active, explicitly enrolled providers and inspect
residency outcomes. Provider Autopilot defaults off; setup enables active control
without a mandatory shadow stage.

## When to use

Use with compatible protocol-2 coordinator and provider releases. See the
[architecture](../architecture/model-autopilot.md) for ownership and eligibility.

## Prerequisites

- Complete coordinator/provider validation and the normal release process.
- Production deployment, configuration, traffic changes and fleet restarts
  require the specific approval described in [coordinator deployment](coordinator-deploy.md).
- Establish a comparable non-enrolled holdout and observation window. Compare
  qualified logical-request completion and first-content outcomes, donor
  capacity, local-work interference, churn and failures. Opt-in selection itself
  can bias comparisons; report cohort differences and denominators.

## Steps

1. Run `darkbloom start`. Answer Yes to **Autopilot — Experimental** and select at
   least one eligible model. Missing builds download and verify before enrollment
   is saved. Already-downloaded selections are verified too. No starts with
   missing settings or unattended upgrades enable the feature automatically.
2. Run `darkbloom autopilot status`. Confirm the selected set and live `active`
   state. `waiting` means consent exists but a coordinator lease is unavailable.
   Cached inventory is not proof of ready capacity.
   Freshness follows the configured heartbeat interval, so an intentionally
   slower daemon refresh does not appear as a missing live report.
3. Use `darkbloom autopilot pin MODEL_ID` to protect a selected model. Use `unpin`
   to remove that protection. Changes are applied at the next capacity poll;
   status reports a configuration revision waiting to apply when appropriate.
4. Use `darkbloom autopilot pause` to stop new demand-based residency changes while
   retaining current ready models; `resume` asks for active control again. Retired,
   unadvertised models may still be released once unpinned and unused.
   `darkbloom autopilot models` changes the approved selection through the picker,
   downloads, verification, drain and restart flow.
5. Inspect authenticated `GET /v1/admin/autopilot`. It returns controller summary
   and up to 200 ledger events from the last 24 hours. Compare planned benefit
   with terminal capacity and request outcomes. Provider status includes local
   resident models and latest transition result.

Coordinator defaults accept opted-in providers in active mode. The action and
operation bounds are in [configuration](../reference/configuration.md#model-autopilot).
Optional `EIGENINFERENCE_AUTOPILOT_OBSERVE_ONLY=true` is an operator diagnostic;
it sends no control leases or commands and is not a provider onboarding step.

## Verification

- Run `go test ./coordinator/...` and focused race tests for Autopilot and routing.
- Run `make provider-test` with the source-matched Metal library.
- Exercise expired/old-session commands, selection changes, opt-out during a
  transition, local/network work, protected donor floors, mixed shapes,
  optional-assistant fallback, partial failures and ledger unavailability.
- Verify the exact released builds separately. Local tests and completed load
  commands do not establish production improvement.

## Rollback

1. Submit authenticated `POST /v1/admin/autopilot` with `{"paused":true}` to stop
   new reservations. Existing operations retain retries and reconciliation;
   pausing is not a reversal of already accepted work. Resume with `false`.
2. On a provider, run `darkbloom autopilot disable`. The daemon consumes the new
   revision at its next capacity poll, completes any accepted operation and
   restores its saved idle policy. Selected files remain downloaded.
3. Inspect actual resident sets and uncertain records. A failed operation may
   have released a model before failing to load another. Do not report rollback
   success until the resulting capacity is confirmed.

## Related

- [Autopilot architecture](../architecture/model-autopilot.md)
- [API contracts](../reference/api-contracts.md)
- [Provider release](provider-release.md)
- [Historical capacity evidence](../reports/2026-09-11-autopilot-capacity-evidence.md)
