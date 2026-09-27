package api

import (
	"net/http"
	"testing"

	"github.com/eigeninference/d-inference/coordinator/registry"
)

func TestAutopilotAdminEndpointRequiresAuthAndValidPause(t *testing.T) {
	srv, _ := testServer(t)
	srv.SetAdminKey("autopilot-test-admin")
	if err := srv.registry.ConfigureAutopilot(registry.DefaultAutopilotConfig()); err != nil {
		t.Fatal(err)
	}
	path := "/v1/admin/autopilot"
	if w := doReq(srv, http.MethodPost, path, "", `{"paused":true}`); w.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated=%d", w.Code)
	}
	auth := "Bearer autopilot-test-admin"
	for _, body := range []string{`{}`, `{"paused":"yes"}`, `{"paused":true,"extra":1}`, `{"paused":true} {}`} {
		if w := doReq(srv, http.MethodPost, path, auth, body); w.Code != http.StatusBadRequest {
			t.Fatalf("body=%s code=%d", body, w.Code)
		}
	}
	if w := doReq(srv, http.MethodPost, path, auth, `{"paused":true}`); w.Code != http.StatusOK {
		t.Fatalf("pause=%d %s", w.Code, w.Body.String())
	}
	if !srv.registry.AutopilotSnapshot().Paused {
		t.Fatal("pause not applied")
	}
	if w := doReq(srv, http.MethodPost, path, auth, `{"paused":false}`); w.Code != http.StatusOK {
		t.Fatal(w.Code)
	}
	if srv.registry.AutopilotSnapshot().Paused {
		t.Fatal("resume not applied")
	}
}
