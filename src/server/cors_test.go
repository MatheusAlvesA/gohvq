package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestConfiguredCORS(t *testing.T) {
	for _, origin := range []string{"https://app.example.com:8443", "", "*"} {
		for _, method := range []string{http.MethodOptions, http.MethodDelete} {
			t.Run(origin+"/"+method, func(t *testing.T) {
				s := newTestServer()
				s.CORSAllowedOrigin = origin
				key := newValidKey(t, s)
				req := httptest.NewRequest(method, "/admin/clearQueue", nil)
				req.Header.Set("Origin", "https://untrusted.example.com")
				req.Header.Set("Access-Control-Request-Method", http.MethodDelete)
				rec := httptest.NewRecorder()
				s.Server.Handler.ServeHTTP(rec, req)

				wantOrigin, wantCredentials := origin, "true"
				if origin == "" || origin == "*" {
					wantOrigin, wantCredentials = "", ""
				}
				if rec.Header().Get("Access-Control-Allow-Origin") != wantOrigin ||
					rec.Header().Get("Access-Control-Allow-Credentials") != wantCredentials {
					t.Fatalf("unexpected CORS headers: %v", rec.Header())
				}
				if method == http.MethodOptions {
					if rec.Code != http.StatusNoContent ||
						rec.Header().Get("Access-Control-Allow-Methods") != "GET, POST, DELETE, OPTIONS" ||
						rec.Header().Get("Access-Control-Allow-Headers") != "Authorization, Content-Type" {
						t.Fatalf("unexpected preflight response: %d %v", rec.Code, rec.Header())
					}
				} else if rec.Code != http.StatusForbidden {
					t.Fatalf("unauthorized request returned %d", rec.Code)
				}
				check := httptest.NewRecorder()
				s.Server.Handler.ServeHTTP(check, httptest.NewRequest(http.MethodGet, "/position?key="+key, nil))
				if check.Code != http.StatusOK {
					t.Fatalf("request changed queue state: position returned %d", check.Code)
				}
			})
		}
	}
}
