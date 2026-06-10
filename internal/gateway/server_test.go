package gateway

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestListMCPTools(t *testing.T) {
	server := NewServer()
	req := httptest.NewRequest(http.MethodGet, "/mcp/tools/list", nil)
	rec := httptest.NewRecorder()

	server.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "search_grammar") {
		t.Fatalf("expected search_grammar in response: %s", rec.Body.String())
	}
}

func TestTenantModelHotUpdate(t *testing.T) {
	server := NewServer()
	body := strings.NewReader(`{"provider":"mock","model":"mock-v2","temperature":0.4}`)
	req := httptest.NewRequest(http.MethodPatch, "/api/tenants/tenant-jp/model", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "mock-v2") {
		t.Fatalf("expected updated model in response: %s", rec.Body.String())
	}
}
