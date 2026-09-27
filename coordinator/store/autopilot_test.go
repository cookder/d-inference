package store

import (
	"context"
	"testing"
	"time"
)

func TestAutopilotLedgerIdempotentAndOwnsArrays(t *testing.T) {
	s := NewMemory(Config{})
	now := time.Now()
	r := AutopilotRecord{CommandID: "one", ProviderID: "session", Phase: "reserved", At: now, Before: []string{"old"}, Unload: []string{"old"}, Load: "new"}
	if err := s.RecordAutopilot(context.Background(), []AutopilotRecord{r, r}); err != nil {
		t.Fatal(err)
	}
	r.Before[0] = "mutated"
	records, err := s.AutopilotRecords(context.Background(), now.Add(-time.Second), 10)
	if err != nil || len(records) != 1 || records[0].Before[0] != "old" {
		t.Fatalf("records=%+v err=%v", records, err)
	}
	records[0].Before[0] = "mutated-again"
	records, _ = s.AutopilotRecords(context.Background(), now.Add(-time.Second), 10)
	if records[0].Before[0] != "old" {
		t.Fatal("reader mutated ledger")
	}
}
