package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// Execute another queue operation after insertion but before response encoding.
// This forces the interleaving without relying on goroutine scheduling.
type enterInterleavingWriter struct {
	*httptest.ResponseRecorder
	afterInsert func()
}

func (w *enterInterleavingWriter) WriteHeader(code int) {
	w.afterInsert()
	w.ResponseRecorder.WriteHeader(code)
}

func TestEnterReturnsPositionAtInsertion(t *testing.T) {
	for _, operation := range []string{"finish", "clear", "enter"} {
		t.Run(operation, func(t *testing.T) {
			s := newTestServer()
			w := &enterInterleavingWriter{
				ResponseRecorder: httptest.NewRecorder(),
				afterInsert: func() {
					switch operation {
					case "finish":
						s.repo.FinishItems(1)
					case "clear":
						s.repo.ClearQueue()
					case "enter":
						s.repo.CreateItem("")
					}
				},
			}
			s.Server.Handler.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/enter", nil))
			var body struct {
				Key      string `json:"key"`
				Position uint64 `json:"position"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if w.Code != http.StatusCreated || body.Key == "" || body.Position != 0 {
				t.Fatalf("invalid insertion response: status %d, body %s", w.Code, w.Body)
			}
		})
	}
}

func TestConcurrentPositionAndFinish(t *testing.T) {
	s := newTestServer()
	var wg sync.WaitGroup
	for range 500 {
		item, err := s.repo.CreateItem("")
		if err != nil {
			t.Fatal(err)
		}
		wg.Add(2)
		go func() {
			defer wg.Done()
			w := httptest.NewRecorder()
			s.Server.Handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/position?key="+item.Key, nil))
			var body struct {
				Key        string `json:"key"`
				FinishedAt int64  `json:"finishedAt"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Error(err)
				return
			}
			if w.Code != http.StatusOK || body.Key != item.Key || body.FinishedAt < 0 {
				t.Errorf("invalid concurrent position response: %s", w.Body)
			}
		}()
		go func() {
			defer wg.Done()
			s.repo.FinishItems(1)
		}()
	}
	wg.Wait()
}
