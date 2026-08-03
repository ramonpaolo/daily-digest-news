package health

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandlerExposesOnlyHealthEndpoint(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	resp := httptest.NewRecorder()
	Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("/health status = %d, want 200", resp.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode health body: %v", err)
	}
	if body["status"] != "ok" {
		t.Fatalf("health body = %v, want status ok", body)
	}

	notFound := httptest.NewRecorder()
	Handler().ServeHTTP(notFound, httptest.NewRequest(http.MethodGet, "/", nil))
	if notFound.Code != http.StatusNotFound {
		t.Fatalf("/ status = %d, want 404", notFound.Code)
	}
}
