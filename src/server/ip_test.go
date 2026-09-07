package server

import (
	"encoding/json"
	"net/http/httptest"
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
