package server

import (
	"context"
	"encoding/json/v2"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const recaptchaVerifyURL = "https://www.google.com/recaptcha/api/siteverify"
const turnstileVerifyURL = "https://challenges.cloudflare.com/turnstile/v0/siteverify"

var defaultCaptchaClient = &http.Client{
	Timeout:       5 * time.Second,
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
}

// requireCaptcha runs before queue mutation and never holds repository locks.
func (s *Server) requireCaptcha(w http.ResponseWriter, r *http.Request) bool {
	if s.RecaptchaSecretKey == "" && s.TurnstileSecretKey == "" {
		return true
	}
	reject := func(status int, message string) bool {
		w.WriteHeader(status)
		json.MarshalWrite(w, map[string]string{"message": message})
		return false
	}
	if s.RecaptchaSecretKey != "" && s.TurnstileSecretKey != "" {
		return reject(http.StatusServiceUnavailable, "Configure only one CAPTCHA provider")
	}
	endpoint, secret, field := recaptchaVerifyURL, s.RecaptchaSecretKey, "g-recaptcha-response"
	if s.TurnstileSecretKey != "" {
		endpoint, secret, field = turnstileVerifyURL, s.TurnstileSecretKey, "cf-turnstile-response"
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16*1024)
	contentType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil {
		return reject(http.StatusBadRequest, "Invalid CAPTCHA request content type")
	}
	var token string
	switch contentType {
	case "application/json":
		var body struct {
			Token string `json:"captchaToken"`
		}
		if err := json.UnmarshalRead(r.Body, &body); err != nil {
			return reject(http.StatusBadRequest, "Invalid CAPTCHA request body")
		}
		token = body.Token
	case "application/x-www-form-urlencoded":
		if err := r.ParseForm(); err != nil {
			return reject(http.StatusBadRequest, "Invalid CAPTCHA request body")
		}
		values := r.PostForm[field]
		if len(values) != 1 {
			return reject(http.StatusBadRequest, "Expected a single CAPTCHA token")
		}
		token = values[0]
	default:
		return reject(http.StatusBadRequest, "Unsupported CAPTCHA request content type")
	}
	if strings.TrimSpace(token) == "" || len(token) > 8192 || (endpoint == turnstileVerifyURL && len(token) > 2048) {
		return reject(http.StatusBadRequest, "Missing or invalid CAPTCHA token")
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	form := url.Values{"secret": {secret}, "response": {token}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return reject(http.StatusServiceUnavailable, "CAPTCHA verification unavailable")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	client := s.captchaClient
	if client == nil {
		client = defaultCaptchaClient
	}
	response, err := client.Do(req)
	if err != nil {
		return reject(http.StatusServiceUnavailable, "CAPTCHA verification unavailable")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return reject(http.StatusServiceUnavailable, "CAPTCHA verification unavailable")
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 64*1024+1))
	var result struct {
		Success bool     `json:"success"`
		Score   *float64 `json:"score"`
	}
	if err != nil || len(data) > 64*1024 || json.Unmarshal(data, &result) != nil {
		return reject(http.StatusServiceUnavailable, "CAPTCHA verification unavailable")
	}
	// v3 requires a score/action policy; this integration supports reCAPTCHA v2.
	if !result.Success || (endpoint == recaptchaVerifyURL && result.Score != nil) {
		return reject(http.StatusForbidden, "CAPTCHA verification failed")
	}
	return true
}
