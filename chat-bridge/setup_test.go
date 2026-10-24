package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestChatOnlyStatusDoesNotClaimImages(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			t.Errorf("unconfigured image engine was contacted: %s", r.URL.Path)
		}
		w.Write([]byte(`{}`))
	}))
	defer backend.Close()
	b := &bridge{llm: backend.URL, nightmare: backend.URL, client: backend.Client(), data: t.TempDir()}
	req := httptest.NewRequest("GET", "http://127.0.0.1:8085/api/status", nil)
	rec := httptest.NewRecorder()
	b.handler().ServeHTTP(rec, req)
	var status struct {
		Gemma  bool
		Images bool
	}
	if rec.Code != 200 {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if !status.Gemma || status.Images {
		t.Fatalf("unexpected status: %+v", status)
	}
}
