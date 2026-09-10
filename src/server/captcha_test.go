package server

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type captchaTransport func(*http.Request) (*http.Response, error)

func (f captchaTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestCaptchaAdmission(t *testing.T) {
	for _, provider := range []string{"google", "cloudflare"} {
		for _, tc := range []struct {
			name, body, contentType, response string
			upstreamStatus, want              int
			networkError                      bool
		}{
			{"valid JSON", `{"captchaToken":"test-token"}`, "application/json", `{"success":true}`, 200, 201, false},
			{"valid form", "TOKEN_FIELD=test-token", "application/x-www-form-urlencoded", `{"success":true}`, 200, 201, false},
			{"rejected", `{"captchaToken":"test-token"}`, "application/json", `{"success":false,"error-codes":["timeout-or-duplicate"]}`, 200, 403, false},
			{"missing success", `{"captchaToken":"test-token"}`, "application/json", `{}`, 200, 403, false},
			{"invalid response", `{"captchaToken":"test-token"}`, "application/json", `{"success":true}garbage`, 200, 503, false},
			{"wrong response type", `{"captchaToken":"test-token"}`, "application/json", `{"success":"true"}`, 200, 503, false},
			{"upstream error", `{"captchaToken":"test-token"}`, "application/json", `{"success":true}`, 500, 503, false},
			{"network error", `{"captchaToken":"test-token"}`, "application/json", "", 0, 503, true},
			{"missing token", `{}`, "application/json", "", 0, 400, false},
			{"empty token", `{"captchaToken":" "}`, "application/json", "", 0, 400, false},
			{"wrong token type", `{"captchaToken":123}`, "application/json", "", 0, 400, false},
			{"duplicate JSON", `{"captchaToken":"a","captchaToken":"b"}`, "application/json", "", 0, 400, false},
			{"trailing JSON", `{"captchaToken":"test-token"}{}`, "application/json", "", 0, 400, false},
			{"duplicate form", "TOKEN_FIELD=a&TOKEN_FIELD=b", "application/x-www-form-urlencoded", "", 0, 400, false},
			{"query only", "", "application/x-www-form-urlencoded", "", 0, 400, false},
			{"unsupported type", "test-token", "text/plain", "", 0, 400, false},
			{"oversized body", strings.Repeat("a", 17000), "application/json", "", 0, 400, false},
		} {
			t.Run(provider+"/"+tc.name, func(t *testing.T) {
				s := newTestServer()
				key := newValidKey(t, s)
				endpoint, field := recaptchaVerifyURL, "g-recaptcha-response"
				if provider == "google" {
					s.RecaptchaSecretKey = "private-secret"
				} else {
					s.TurnstileSecretKey = "private-secret"
					endpoint, field = turnstileVerifyURL, "cf-turnstile-response"
				}
				calls := 0
				s.captchaClient = &http.Client{Transport: captchaTransport(func(r *http.Request) (*http.Response, error) {
					calls++
					if r.URL.String() != endpoint || r.Method != http.MethodPost {
						t.Fatalf("unexpected verification request: %s %s", r.Method, r.URL)
					}
					if _, ok := r.Context().Deadline(); !ok {
						t.Error("missing verification deadline")
					}
					if err := r.ParseForm(); err != nil {
						t.Fatal(err)
					}
					if r.PostForm.Get("secret") != "private-secret" || r.PostForm.Get("response") != "test-token" {
						t.Fatalf("incorrect verification parameters")
					}
					if tc.networkError {
						return nil, errors.New("unavailable")
					}
					return &http.Response{StatusCode: tc.upstreamStatus, Body: io.NopCloser(strings.NewReader(tc.response)), Header: make(http.Header)}, nil
				})}
				req := httptest.NewRequest(http.MethodPost, "/enter?"+field+"=test-token", strings.NewReader(strings.ReplaceAll(tc.body, "TOKEN_FIELD", field)))
				req.Header.Set("Content-Type", tc.contentType)
				w := httptest.NewRecorder()
				s.Server.Handler.ServeHTTP(w, req)
				if w.Code != tc.want {
					t.Fatalf("got %d: %s, want %d", w.Code, w.Body, tc.want)
				}
				expectedSize := uint64(1)
				if tc.want == 201 {
					expectedSize++
				}
				if uint64(s.repo.GetCurrentQueueSize()) != expectedSize || s.repo.GetAndPingItemByKey(key) == nil {
					t.Fatal("unexpected queue mutation")
				}
				expectedCalls := 1
				if tc.want == 400 {
					expectedCalls = 0
				}
				if calls != expectedCalls {
					t.Fatalf("verification calls = %d, want %d", calls, expectedCalls)
				}
				if strings.Contains(w.Body.String(), "private-secret") {
					t.Fatal("secret leaked")
				}
			})
		}
	}
}

func TestCaptchaDisabledConflictAndOtherRoutes(t *testing.T) {
	s := newTestServer()
	s.captchaClient = &http.Client{Transport: captchaTransport(func(*http.Request) (*http.Response, error) { t.Fatal("unexpected provider call"); return nil, nil })}
	key := newValidKey(t, s)
	s.RecaptchaSecretKey, s.TurnstileSecretKey = "google", "cloudflare"
	w := httptest.NewRecorder()
	s.Server.Handler.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/enter", nil))
	if w.Code != 503 || s.repo.GetCurrentQueueSize() != 1 {
		t.Fatalf("conflict: %d %s", w.Code, w.Body)
	}
	s.AccessTk = "admin_testing_token"
	for _, tc := range []struct {
		method, path string
		status       int
	}{
		{"GET", "/position?key=" + key, 200},
		{"OPTIONS", "/enter", 204},
		{"POST", "/admin/finishItems", 200},
	} {
		w = httptest.NewRecorder()
		req := httptest.NewRequest(tc.method, tc.path, nil)
		req.Header.Set("Authorization", s.AccessTk)
		s.Server.Handler.ServeHTTP(w, req)
		if w.Code != tc.status {
			t.Fatalf("%s: %d %s", tc.path, w.Code, w.Body)
		}
	}
}

func TestCaptchaCancellation(t *testing.T) {
	s := newTestServer()
	s.TurnstileSecretKey = "secret"
	s.captchaClient = &http.Client{Transport: captchaTransport(func(r *http.Request) (*http.Response, error) {
		<-r.Context().Done()
		return nil, r.Context().Err()
	})}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := httptest.NewRequest(http.MethodPost, "/enter", strings.NewReader(`{"captchaToken":"token"}`)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.Server.Handler.ServeHTTP(w, req)
	if w.Code != 503 || s.repo.GetCurrentQueueSize() != 0 {
		t.Fatalf("cancelled admission: %d", w.Code)
	}
}

func TestRecaptchaV3Rejected(t *testing.T) {
	s := newTestServer()
	s.RecaptchaSecretKey = "secret"
	s.captchaClient = &http.Client{Transport: captchaTransport(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"success":true,"score":0.9,"action":"enter"}`)), Header: make(http.Header)}, nil
	})}
	req := httptest.NewRequest(http.MethodPost, "/enter", strings.NewReader(`{"captchaToken":"token"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.Server.Handler.ServeHTTP(w, req)
	if w.Code != 403 || s.repo.GetCurrentQueueSize() != 0 {
		t.Fatalf("v3 token admitted: %d", w.Code)
	}
}
