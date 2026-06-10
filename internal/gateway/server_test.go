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

func TestMCPJSONRPCToolsList(t *testing.T) {
	server := NewServer()
	body := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	req := httptest.NewRequest(http.MethodPost, "/mcp", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"jsonrpc":"2.0"`) {
		t.Fatalf("expected json-rpc response: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "search_memory") {
		t.Fatalf("expected search_memory in response: %s", rec.Body.String())
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

func TestStreamAgentRejectsEmptyMessage(t *testing.T) {
	server := NewServer()
	body := strings.NewReader(`{"tenant_id":"tenant-jp","user_id":"user-001","session_id":"sess-empty","message":"   "}`)
	req := httptest.NewRequest(http.MethodPost, "/api/agent/stream", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "tenant_id, user_id and message are required") {
		t.Fatalf("expected validation error, got: %s", rec.Body.String())
	}
}
