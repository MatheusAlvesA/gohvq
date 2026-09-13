package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestEnterQueueFull(t *testing.T) {
	s := newTestServer()
	s.repo.MaxQueueSize = 2
	first := newValidKey(t, s)
	second := newValidKey(t, s)
	w := httptest.NewRecorder()
	s.Server.Handler.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/enter", nil))
	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if w.Code != http.StatusServiceUnavailable || body["message"] != "Queue is full. Please try again later." || len(body) != 1 || w.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("unexpected response: %d %s", w.Code, w.Body.String())
	}
	if s.repo.GetCurrentQueueSize() != 2 || s.repo.GetAndPingItemByKey(first) == nil || s.repo.GetAndPingItemByKey(second) == nil {
		t.Fatal("rejected entry changed queue")
	}
	s.repo.FinishItems(1)
	newValidKey(t, s)
	if s.repo.GetCurrentQueueSize() != 2 {
		t.Fatal("released slot was not reused")
	}
}
