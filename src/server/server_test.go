package server

import (
	"MatheusAlvesA/gohvq/src/repository"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newTestServer() *Server {
	s := InitServer()
	s.SetRepository(repository.InitRepository())
	return s
}

func newValidKey(t *testing.T, s *Server) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/enter", nil)
	rec := httptest.NewRecorder()
	s.Server.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("Fail to create item, status: %d", rec.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("Fail to parse response %q", err)
	}
	key, ok := body["key"].(string)
	if !ok {
		t.Fatalf("Response without key: %v", body)
	}
	return key
}

func TestHandleEnter(t *testing.T) {
	s := newTestServer()

	req := httptest.NewRequest(http.MethodPost, "/enter", nil)
	rec := httptest.NewRecorder()
	s.Server.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("Incorrect status, expected 201 got %d", rec.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("Fail to parse response %q", err)
	}
	key, _ := body["key"].(string)
	if !repository.IsValidKey(key) {
		t.Errorf("Invalid key on response: %q", key)
	}
	if body["position"] != 0.0 {
		t.Errorf("First item should be on position 0, got %v", body["position"])
	}

	second := newValidKey(t, s)
	if second == key {
		t.Errorf("Server should generate distinct keys")
	}
}

func TestEndpointsWithoutRepository(t *testing.T) {
	endpoints := []struct {
		method string
		url    string
	}{
		{http.MethodPost, "/enter"},
		{http.MethodGet, "/position?key=" + strings.Repeat("a", int(repository.KEY_SIZE))},
		{http.MethodPost, "/admin/finishItems"},
		{http.MethodGet, "/admin/finishedItem/" + strings.Repeat("a", int(repository.KEY_SIZE))},
		{http.MethodDelete, "/admin/finishedItem/" + strings.Repeat("a", int(repository.KEY_SIZE))},
		{http.MethodDelete, "/admin/clearFinished"},
		{http.MethodDelete, "/admin/clearQueue"},
	}

	for _, endpoint := range endpoints {
		s := InitServer()

		req := httptest.NewRequest(endpoint.method, endpoint.url, nil)
		rec := httptest.NewRecorder()
		s.Server.Handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusInternalServerError {
			t.Errorf("Incorrect status without repository for %s %s, expected 500 got %d",
				endpoint.method, endpoint.url, rec.Code)
		}
		var body map[string]string
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Errorf("Fail to parse response for %s %s: %q", endpoint.method, endpoint.url, err)
			continue
		}
		if body["message"] != "Repository not set" {
			t.Errorf("Incorrect message without repository for %s %s, got %q",
				endpoint.method, endpoint.url, body["message"])
		}
	}
}

func TestHandlePosition(t *testing.T) {
	s := newTestServer()
	key := newValidKey(t, s)

	req := httptest.NewRequest(http.MethodGet, "/position?key="+key, nil)
	rec := httptest.NewRecorder()
	s.Server.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Incorrect status, expected 200 got %d", rec.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("Fail to parse response %q", err)
	}
	if body["key"] != key {
		t.Errorf("Incorrect key on response, expected %q got %v", key, body["key"])
	}
	if body["position"] != 0.0 {
		t.Errorf("Incorrect position, expected 0 got %v", body["position"])
	}
}

func TestHandlePositionInvalidKey(t *testing.T) {
	s := newTestServer()

	req := httptest.NewRequest(http.MethodGet, "/position?key=invalid", nil)
	rec := httptest.NewRecorder()
	s.Server.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Incorrect status for invalid key, expected 400 got %d", rec.Code)
	}
}

func TestHandlePositionNotFound(t *testing.T) {
	s := newTestServer()
	unknownKey := strings.Repeat("a", int(repository.KEY_SIZE))

	req := httptest.NewRequest(http.MethodGet, "/position?key="+unknownKey, nil)
	rec := httptest.NewRecorder()
	s.Server.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("Incorrect status for unknown key, expected 404 got %d", rec.Code)
	}
}

func TestHandlePositionFinishedItem(t *testing.T) {
	s := newTestServer()
	s.SetAdminAcessToken("admin_testing_token")
	key := newValidKey(t, s)

	req := httptest.NewRequest(http.MethodPost, "/admin/finishItems", nil)
	req.Header.Set("authorization", "admin_testing_token")
	rec := httptest.NewRecorder()
	s.Server.Handler.ServeHTTP(rec, req)

	req = httptest.NewRequest(http.MethodGet, "/position?key="+key, nil)
	rec = httptest.NewRecorder()
	s.Server.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Finished item should be found, got status %d", rec.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("Fail to parse response %q", err)
	}
	if body["finishedAt"] == 0.0 {
		t.Errorf("Finished item should have finishedAt set")
	}
}

func TestHandleAdminFinish(t *testing.T) {
	s := newTestServer()
	s.SetAdminAcessToken("admin_testing_token")
	key1 := newValidKey(t, s)
	key2 := newValidKey(t, s)
	newValidKey(t, s)

	req := httptest.NewRequest(http.MethodPost, "/admin/finishItems?n=2", nil)
	req.Header.Set("authorization", "admin_testing_token")
	rec := httptest.NewRecorder()
	s.Server.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Incorrect status, expected 200 got %d", rec.Code)
	}
	var body []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("Fail to parse response %q", err)
	}
	if len(body) != 2 {
		t.Fatalf("Expected 2 finished items, got %d", len(body))
	}
	if body[0]["key"] != key1 || body[1]["key"] != key2 {
		t.Errorf("Finished items should follow queue order (FIFO)")
	}
}

func TestHandleAdminFinishDefault(t *testing.T) {
	s := newTestServer()
	s.SetAdminAcessToken("admin_testing_token")
	newValidKey(t, s)
	newValidKey(t, s)

	for _, url := range []string{"/admin/finishItems"} {
		newValidKey(t, s)
		req := httptest.NewRequest(http.MethodPost, url, nil)
		req.Header.Set("authorization", "admin_testing_token")
		rec := httptest.NewRecorder()
		s.Server.Handler.ServeHTTP(rec, req)

		var body []map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("Fail to parse response %q", err)
		}
		if len(body) != 1 {
			t.Errorf("Default finish should return 1 item, got %d for %q", len(body), url)
		}
	}
}

func TestHandleAdminGetFinished(t *testing.T) {
	s := newTestServer()
	s.SetAdminAcessToken("admin_testing_token")
	key := newValidKey(t, s)

	req := httptest.NewRequest(http.MethodPost, "/admin/finishItems", nil)
	req.Header.Set("authorization", "admin_testing_token")
	rec := httptest.NewRecorder()
	s.Server.Handler.ServeHTTP(rec, req)

	req = httptest.NewRequest(http.MethodGet, "/admin/finishedItem/"+key, nil)
	req.Header.Set("authorization", "admin_testing_token")
	rec = httptest.NewRecorder()
	s.Server.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Incorrect status, expected 200 got %d", rec.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("Fail to parse response %q", err)
	}
	if body["key"] != key {
		t.Errorf("Incorrect key on response, expected %q got %v", key, body["key"])
	}
}

func TestHandleAdminGetFinishedNotFound(t *testing.T) {
	s := newTestServer()
	s.SetAdminAcessToken("admin_testing_token")
	unknownKey := strings.Repeat("a", int(repository.KEY_SIZE))

	req := httptest.NewRequest(http.MethodGet, "/admin/finishedItem/"+unknownKey, nil)
	req.Header.Set("authorization", "admin_testing_token")
	rec := httptest.NewRecorder()
	s.Server.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("Incorrect status for unknown key, expected 404 got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/admin/finishedItem/invalid", nil)
	req.Header.Set("authorization", "admin_testing_token")
	rec = httptest.NewRecorder()
	s.Server.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Incorrect status for invalid key, expected 400 got %d", rec.Code)
	}
}

func TestHandleAdminDeleteFinished(t *testing.T) {
	s := newTestServer()
	s.SetAdminAcessToken("admin_testing_token")
	key := newValidKey(t, s)

	req := httptest.NewRequest(http.MethodPost, "/admin/finishItems", nil)
	req.Header.Set("authorization", "admin_testing_token")
	rec := httptest.NewRecorder()
	s.Server.Handler.ServeHTTP(rec, req)

	req = httptest.NewRequest(http.MethodDelete, "/admin/finishedItem/"+key, nil)
	req.Header.Set("authorization", "admin_testing_token")
	rec = httptest.NewRecorder()
	s.Server.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Incorrect status, expected 200 got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/admin/finishedItem/"+key, nil)
	req.Header.Set("authorization", "admin_testing_token")
	rec = httptest.NewRecorder()
	s.Server.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("Deleted item should not be found, got status %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodDelete, "/admin/finishedItem/invalid", nil)
	req.Header.Set("authorization", "admin_testing_token")
	rec = httptest.NewRecorder()
	s.Server.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Incorrect status for invalid key, expected 400 got %d", rec.Code)
	}
}

func TestHandleAdminClearFinished(t *testing.T) {
	s := newTestServer()
	s.SetAdminAcessToken("admin_testing_token")
	key := newValidKey(t, s)

	req := httptest.NewRequest(http.MethodPost, "/admin/finishItems", nil)
	req.Header.Set("authorization", "admin_testing_token")
	rec := httptest.NewRecorder()
	s.Server.Handler.ServeHTTP(rec, req)

	req = httptest.NewRequest(http.MethodDelete, "/admin/clearFinished", nil)
	req.Header.Set("authorization", "admin_testing_token")
	rec = httptest.NewRecorder()
	s.Server.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Incorrect status, expected 200 got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/admin/finishedItem/"+key, nil)
	req.Header.Set("authorization", "admin_testing_token")
	rec = httptest.NewRecorder()
	s.Server.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("Finished items should be cleared, got status %d", rec.Code)
	}
}

func TestHandleAdminClearQueue(t *testing.T) {
	s := newTestServer()
	s.SetAdminAcessToken("admin_testing_token")
	key := newValidKey(t, s)

	req := httptest.NewRequest(http.MethodDelete, "/admin/clearQueue", nil)
	req.Header.Set("authorization", "admin_testing_token")
	rec := httptest.NewRecorder()
	s.Server.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Incorrect status, expected 200 got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/position?key="+key, nil)
	req.Header.Set("authorization", "admin_testing_token")
	rec = httptest.NewRecorder()
	s.Server.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("Queue should be cleared, got status %d", rec.Code)
	}
}

func BenchmarkHandleEnter(b *testing.B) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodPost, "/enter", nil)

	for b.Loop() {
		s.Server.Handler.ServeHTTP(httptest.NewRecorder(), req)
	}
}

func BenchmarkHandlePosition(b *testing.B) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodPost, "/enter", nil)
	rec := httptest.NewRecorder()
	s.Server.Handler.ServeHTTP(rec, req)
	var body map[string]any
	json.Unmarshal(rec.Body.Bytes(), &body)
	key, _ := body["key"].(string)

	req = httptest.NewRequest(http.MethodGet, "/position?key="+key, nil)

	for b.Loop() {
		s.Server.Handler.ServeHTTP(httptest.NewRecorder(), req)
	}
}

func BenchmarkHandleAdminFinish(b *testing.B) {
	s := newTestServer()
	s.SetAdminAcessToken("admin_testing_token")
	reqEnter := httptest.NewRequest(http.MethodPost, "/enter", nil)
	reqEnter.Header.Set("authorization", "admin_testing_token")
	reqFinish := httptest.NewRequest(http.MethodPost, "/admin/finishItems", nil)
	reqEnter.Header.Set("authorization", "admin_testing_token")

	for b.Loop() {
		s.Server.Handler.ServeHTTP(httptest.NewRecorder(), reqEnter)
		s.Server.Handler.ServeHTTP(httptest.NewRecorder(), reqFinish)
	}
}

func TestFinishRejectsInvalidCountWithoutMutation(t *testing.T) {
	for _, query := range []string{"n=0", "n=-1", "n=abc", "n=", "n", "n=1.5", "n=999999999999999999999999", "n=1&n=2", "n=%zz"} {
		t.Run(query, func(t *testing.T) {
			s := newTestServer()
			s.SetAdminAcessToken("admin_testing_token")
			key := newValidKey(t, s)
			req := httptest.NewRequest(http.MethodPost, "/admin/finishItems?"+query, nil)
			req.Header.Set("Authorization", "admin_testing_token")
			w := httptest.NewRecorder()
			s.Server.Handler.ServeHTTP(w, req)
			if w.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d: %s", w.Code, w.Body)
			}
			if s.repo.GetCurrentQueueSize() != 1 || s.repo.GetAndPingItemByKey(key) == nil || s.repo.GetFinished(key) != nil {
				t.Fatal("invalid count changed queue state")
			}
		})
	}
}
