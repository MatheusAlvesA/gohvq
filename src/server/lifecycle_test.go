package server

import (
	"MatheusAlvesA/gohvq/src/log"
	"MatheusAlvesA/gohvq/src/repository"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestAdminTokenValidation(t *testing.T) {
	s := newTestServer()
	if s.CheckAdminToken("") {
		t.Fatal("unconfigured server accepted an empty token")
	}
	if !s.SetAdminAcessToken("valid_admin_token") {
		t.Fatal("valid token was rejected")
	}
	if s.SetAdminAcessToken("short") || !s.CheckAdminToken("valid_admin_token") {
		t.Fatal("invalid replacement changed the existing token")
	}
	for _, token := range []string{"", "wrong_admin_token", "valid_admin_token ", "VALID_ADMIN_TOKEN"} {
		if s.CheckAdminToken(token) {
			t.Errorf("invalid token %q was accepted", token)
		}
	}
}

func TestAdminEndpointsRejectUnauthorizedRequests(t *testing.T) {
	for _, endpoint := range []struct{ method, path string }{
		{http.MethodPost, "/admin/finishItems"},
		{http.MethodGet, "/admin/finishedItem/"},
		{http.MethodDelete, "/admin/finishedItem/"},
		{http.MethodDelete, "/admin/clearFinished"},
		{http.MethodDelete, "/admin/clearQueue"},
	} {
		t.Run(endpoint.method+" "+endpoint.path, func(t *testing.T) {
			s := newTestServer()
			s.SetAdminAcessToken("valid_admin_token")
			finished := newValidKey(t, s)
			s.repo.FinishItems(1)
			queued := newValidKey(t, s)
			path := endpoint.path
			if path == "/admin/finishedItem/" {
				path += finished
			}
			for _, token := range []string{"", "wrong_admin_token"} {
				req := httptest.NewRequest(endpoint.method, path, nil)
				req.Header.Set("Authorization", token)
				w := httptest.NewRecorder()
				s.Server.Handler.ServeHTTP(w, req)
				var body map[string]string
				if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
					t.Fatal(err)
				}
				if w.Code != http.StatusForbidden || body["message"] != "Access Denied" {
					t.Fatalf("unauthorized request returned %d: %s", w.Code, w.Body)
				}
				if s.repo.GetCurrentQueueSize() != 1 || s.repo.GetAndPingItemByKey(queued) == nil || s.repo.GetFinished(finished) == nil {
					t.Fatal("unauthorized request changed queue state")
				}
			}
		})
	}
}

func TestUnknownRouteReturnsJSONError(t *testing.T) {
	s := newTestServer()
	s.SetLogService(log.InitService())
	w := httptest.NewRecorder()
	s.Server.Handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/unknown", nil))
	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if w.Code != http.StatusNotFound || body["error"] != "Invalid route for Go Human Virtual Queue" {
		t.Fatalf("unexpected unknown-route response: %d %s", w.Code, w.Body)
	}
}

func TestServerStartAndStop(t *testing.T) {
	for _, configuredToken := range []string{"", "configured_admin_token"} {
		name := "generated token"
		if configuredToken != "" {
			name = "configured token"
		}
		t.Run(name, func(t *testing.T) {
			s := newTestServer()
			s.SetLogService(log.InitService())
			s.AccessTk = configuredToken
			s.Server.Addr = "127.0.0.1:0"
			listening := make(chan string, 1)
			s.Server.BaseContext = func(listener net.Listener) context.Context {
				listening <- listener.Addr().String()
				return context.Background()
			}
			stopped := make(chan struct{})
			s.Server.RegisterOnShutdown(func() { close(stopped) })
			s.Start()
			stopCalled := false
			t.Cleanup(func() {
				if !stopCalled {
					s.Stop()
				}
			})
			if configuredToken == "" {
				if !repository.IsValidKey(s.AccessTk) {
					t.Fatal("server did not generate a valid initial admin token")
				}
			} else if s.AccessTk != configuredToken {
				t.Fatal("server replaced the configured admin token")
			}
			var address string
			select {
			case address = <-listening:
			case <-time.After(5 * time.Second):
				t.Fatal("server did not start listening")
			}
			transport := &http.Transport{}
			defer transport.CloseIdleConnections()
			client := &http.Client{Transport: transport, Timeout: 2 * time.Second}
			res, err := client.Post("http://"+address+"/enter", "", nil)
			if err != nil {
				t.Fatal(err)
			}
			var body struct {
				Key string `json:"key"`
			}
			err = json.NewDecoder(res.Body).Decode(&body)
			res.Body.Close()
			if err != nil || res.StatusCode != http.StatusCreated || !repository.IsValidKey(body.Key) {
				t.Fatalf("running server failed to create a queue item: status %d, body %+v, error %v", res.StatusCode, body, err)
			}
			s.Stop()
			stopCalled = true
			select {
			case <-stopped:
			case <-time.After(5 * time.Second):
				t.Fatal("server did not signal shutdown")
			}
			res, err = client.Get("http://" + address + "/position?key=" + body.Key)
			if err == nil {
				res.Body.Close()
				t.Fatal("server still accepted requests after Stop returned")
			}
		})
	}
}
