package server

import (
	"MatheusAlvesA/gohvq/src/log"
	"encoding/json"
	"fmt"
	"io"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestClientIPVisibility(t *testing.T) {
	for _, tc := range []struct{ remote, ip string }{
		{"192.0.2.10:1234", "192.0.2.10"},
		{"[2001:db8::1234]:1234", "2001:db8::1234"},
		{"[::ffff:192.0.2.10]:1234", "192.0.2.10"},
		{"[fe80::1%eth0]:1234", "fe80::1"},
	} {
		t.Run(tc.remote, func(t *testing.T) {
			s := newTestServer()
			s.SetAdminAcessToken("test-admin-token")
			request := func(method, path, remote string, admin bool) *httptest.ResponseRecorder {
				req := httptest.NewRequest(method, path, nil)
				req.RemoteAddr = remote
				req.Header.Set("X-Forwarded-For", "203.0.113.99")
				req.Header.Set("X-Real-IP", "203.0.113.99")
				if admin {
					req.Header.Set("Authorization", s.AccessTk)
				}
				rec := httptest.NewRecorder()
				s.Server.Handler.ServeHTTP(rec, req)
				return rec
			}
			decode := func(rec *httptest.ResponseRecorder) map[string]any {
				t.Helper()
				var body map[string]any
				if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
					t.Fatal(err)
				}
				return body
			}
			rec := request("POST", "/enter", tc.remote, false)
			if rec.Code != 201 {
				t.Fatal(rec.Body.String())
			}
			body := decode(rec)
			if _, exists := body["ip"]; exists {
				t.Fatal("enter exposed IP")
			}
			key := body["key"].(string)
			for _, finished := range []bool{false, true} {
				if finished {
					denied := request("POST", "/admin/finishItems", tc.remote, false)
					if denied.Code != 403 || s.repo.GetCurrentQueueSize() != 1 {
						t.Fatal("unauthorized finish mutated queue")
					}
					rec = request("POST", "/admin/finishItems", tc.remote, true)
					var items []map[string]any
					if err := json.Unmarshal(rec.Body.Bytes(), &items); err != nil {
						t.Fatal(err)
					}
					if rec.Code != 200 || len(items) != 1 || items[0]["ip"] != tc.ip {
						t.Fatal(rec.Body.String())
					}
				}
				rec = request("GET", "/position?key="+key, "203.0.113.1:5678", false)
				if rec.Code != 200 {
					t.Fatal(rec.Body.String())
				}
				if _, exists := decode(rec)["ip"]; exists {
					t.Fatal("position exposed IP")
				}
			}
			rec = request("GET", "/admin/finishedItem/"+key, tc.remote, false)
			if rec.Code != 403 {
				t.Fatal("unauthorized IP access")
			}
			rec = request("GET", "/admin/finishedItem/"+key, tc.remote, true)
			if rec.Code != 200 || decode(rec)["ip"] != tc.ip {
				t.Fatal(rec.Body.String())
			}
		})
	}
}

func TestEnterIPLimit(t *testing.T) {
	s := newTestServer()
	s.repo.MaxEntriesPerIP = 1
	for i, remote := range []string{"192.0.2.1:1234", "[::ffff:192.0.2.1]:5678", "192.0.2.2:1234"} {
		req := httptest.NewRequest("POST", "/enter", nil)
		req.RemoteAddr = remote
		req.Header.Set("X-Forwarded-For", "203.0.113.1")
		rec := httptest.NewRecorder()
		s.Server.Handler.ServeHTTP(rec, req)
		want, size := 201, uint64(1)
		if i == 1 {
			want = 429
		}
		if i == 2 {
			size = 2
		}
		if rec.Code != want || s.repo.GetCurrentQueueSize() != size {
			t.Fatalf("status=%d size=%d", rec.Code, s.repo.GetCurrentQueueSize())
		}
		if !json.Valid(rec.Body.Bytes()) {
			t.Fatal("invalid JSON response")
		}
	}
}

func TestConfiguredClientIPHeader(t *testing.T) {
	for _, tc := range []struct{ value, want string }{
		{"203.0.113.7", "203.0.113.7"},
		{" 2001:db8::7 ", "2001:db8::7"},
		{"::ffff:203.0.113.7", "203.0.113.7"},
	} {
		t.Run(tc.value, func(t *testing.T) {
			s := newTestServer()
			s.ClientIPHeader = "x-client-ip"
			s.repo.MaxEntriesPerIP = 1
			for i := range 2 {
				req := httptest.NewRequest("POST", "/enter", nil)
				req.RemoteAddr = "invalid connection address"
				req.Header.Set("X-Client-IP", tc.value)
				req.Header.Set("X-Real-IP", "192.0.2.99")
				rec := httptest.NewRecorder()
				s.Server.Handler.ServeHTTP(rec, req)
				if i == 1 {
					if rec.Code != 429 || s.repo.GetCurrentQueueSize() != 1 {
						t.Fatalf("header IP limit not enforced: %d %s", rec.Code, rec.Body.String())
					}
					continue
				}
				var body struct{ Key string }
				if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
					t.Fatal(err)
				}
				if rec.Code != 201 {
					t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
				}
				item := s.repo.GetAndPingItemByKey(body.Key)
				if item == nil || item.IP != tc.want {
					t.Fatalf("unexpected item: %+v", item)
				}
			}
		})
	}
}

func TestClientIPHeaderRejection(t *testing.T) {
	for _, values := range [][]string{nil, {""}, {"   "}, {"not-an-ip"}, {"192.0.2.1:80"}, {"192.0.2.1, 192.0.2.2"}, {"192.0.2.1", "192.0.2.2"}} {
		for _, route := range []struct{ method, path string }{
			{"POST", "/enter"},
			{"GET", "/position"},
		} {
			t.Run(fmt.Sprintf("%s/%v", route.path, values), func(t *testing.T) {
				s := newTestServer()
				s.ClientIPHeader = "X-Client-IP"
				s.SetLogService(log.InitService())
				s.SetAdminAcessToken("test-admin-token")
				item, err := s.repo.CreateItem("192.0.2.1")
				if err != nil {
					t.Fatal(err)
				}
				output, err := os.CreateTemp(t.TempDir(), "log")
				if err != nil {
					t.Fatal(err)
				}
				defer output.Close()
				old := os.Stdout
				os.Stdout = output
				defer func() { os.Stdout = old }()
				req := httptest.NewRequest(route.method, route.path, nil)
				req.Header.Set("Authorization", s.AccessTk)
				req.Header.Set("X-Real-IP", "192.0.2.2")
				if values != nil {
					req.Header["X-Client-Ip"] = values
				}
				rec := httptest.NewRecorder()
				s.Server.Handler.ServeHTTP(rec, req)
				if rec.Code != 400 || s.repo.GetCurrentQueueSize() != 1 || s.repo.GetAndPingItemByKey(item.Key) == nil {
					t.Fatalf("rejection changed queue or wrong status: %d %s", rec.Code, rec.Body.String())
				}
				var body map[string]string
				if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(body["message"], "X-Client-IP") {
					t.Fatalf("missing reason: %v", body)
				}
				if _, err := output.Seek(0, 0); err != nil {
					t.Fatal(err)
				}
				logged, err := io.ReadAll(output)
				if err != nil {
					t.Fatal(err)
				}
				for _, want := range []string{"Rejected request", route.method, route.path, req.RemoteAddr, "X-Client-IP"} {
					if !strings.Contains(string(logged), want) {
						t.Fatalf("log missing %q: %s", want, logged)
					}
				}
			})
		}
	}
}

func TestAdminRoutesIgnoreClientIPHeader(t *testing.T) {
	for _, header := range []string{"", "invalid-ip"} {
		for _, route := range []struct{ method, path string }{
			{"POST", "/admin/finishItems"},
			{"GET", "/admin/finishedItem/"},
			{"DELETE", "/admin/finishedItem/"},
			{"DELETE", "/admin/clearFinished"},
			{"DELETE", "/admin/clearQueue"},
		} {
			t.Run(route.method+route.path+"/"+header, func(t *testing.T) {
				s := newTestServer()
				s.ClientIPHeader = "X-Client-IP"
				s.SetAdminAcessToken("test-admin-token")
				finished, err := s.repo.CreateItem("192.0.2.1")
				if err != nil {
					t.Fatal(err)
				}
				s.repo.FinishItems(1)
				if _, err := s.repo.CreateItem("192.0.2.2"); err != nil {
					t.Fatal(err)
				}
				path := route.path
				if strings.HasSuffix(path, "/") {
					path += finished.Key
				}
				for _, authorized := range []bool{false, true} {
					req := httptest.NewRequest(route.method, path, nil)
					if header != "" {
						req.Header.Set(s.ClientIPHeader, header)
					}
					if authorized {
						req.Header.Set("Authorization", s.AccessTk)
					}
					rec := httptest.NewRecorder()
					s.Server.Handler.ServeHTTP(rec, req)
					want := 403
					if authorized {
						want = 200
					}
					if rec.Code != want {
						t.Fatalf("status=%d want=%d body=%s", rec.Code, want, rec.Body.String())
					}
					if !authorized && (s.repo.GetCurrentQueueSize() != 1 || s.repo.GetFinished(finished.Key) == nil) {
						t.Fatal("unauthorized request changed queue state")
					}
				}
			})
		}
	}
}
