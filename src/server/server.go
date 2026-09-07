package server

import (
	"MatheusAlvesA/gohvq/src/log"
	"MatheusAlvesA/gohvq/src/repository"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"encoding/json/v2"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Server struct {
	TLSCertFile       string
	TLSKeyFile        string
	Server            *http.Server
	repo              *repository.Repository
	log               *log.LogService
	AccessTk          string
	ClientIPHeader    string
	CORSAllowedOrigin string
}

func (s *Server) CheckAdminToken(token string) bool {
	if len(s.AccessTk) < 10 {
		return false
	}
	// Cria o hash de tamanho fixo (32 bytes) para ambos os tokens
	hashA := sha256.Sum256([]byte(s.AccessTk))
	hashB := sha256.Sum256([]byte(token))

	// Como o tamanho agora é sempre idêntico, a comparação é segura contra timing attacks
	return subtle.ConstantTimeCompare(hashA[:], hashB[:]) == 1
}

func (s *Server) SetAdminAcessToken(token string) bool {
	if len(token) < 10 {
		s.Log(log.Error, "Invalid new admin access token, have to be at least 10 chars")
		return false
	}
	s.AccessTk = token
	return true
}

// clientIP uses only the explicitly configured source.
func (s *Server) clientIP(r *http.Request) (netip.Addr, error) {
	if s.ClientIPHeader != "" {
		values := r.Header.Values(s.ClientIPHeader)
		if len(values) == 0 {
			return netip.Addr{}, fmt.Errorf("Required client IP header %q is missing", s.ClientIPHeader)
		}
		if len(values) != 1 {
			return netip.Addr{}, fmt.Errorf("Client IP header %q must contain a single IP address", s.ClientIPHeader)
		}
		addr, err := netip.ParseAddr(strings.TrimSpace(values[0]))
		if err != nil {
			return netip.Addr{}, fmt.Errorf("Client IP header %q must contain a valid IPv4 or IPv6 address", s.ClientIPHeader)
		}
		return addr, nil
	}
	addr, err := netip.ParseAddrPort(r.RemoteAddr)
	return addr.Addr(), err
}

func (s *Server) requireClientIPHeader(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.ClientIPHeader != "" {
			if _, err := s.clientIP(r); err != nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				json.MarshalWrite(w, map[string]string{"message": err.Error()})
				s.Log(log.Warning, fmt.Sprintf("Rejected request: method=%q path=%q remote=%q reason=%q", r.Method, r.URL.Path, r.RemoteAddr, err.Error()))
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func handleEnter(s *Server, w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if s.repo == nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.MarshalWrite(w, map[string]string{"message": "Repository not set"})
		return
	}
	addr, err := s.clientIP(r)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.MarshalWrite(w, map[string]string{"message": "Invalid client address"})
		return
	}
	item, err := s.repo.CreateItem(addr.WithZone("").Unmap().String())
	if errors.Is(err, repository.ErrIPLimitReached) {
		w.WriteHeader(http.StatusTooManyRequests)
		json.MarshalWrite(w, map[string]string{"message": "Queue entry limit reached for IP"})
		return
	}
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.MarshalWrite(w, map[string]string{"message": "Fail to create new item"})
		return
	}
	w.WriteHeader(http.StatusCreated)
	json.MarshalWrite(w, map[string]any{
		"key":      item.Key,
		"position": item.Position,
	})
}

func handlePosition(s *Server, w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	searchKey := r.URL.Query().Get("key")
	if s.repo == nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.MarshalWrite(w, map[string]string{"message": "Repository not set"})
		return
	}
	if !repository.IsValidKey(searchKey) {
		w.WriteHeader(http.StatusBadRequest)
		json.MarshalWrite(w, map[string]string{"message": "Invalid key"})
		return
	}
	item := s.repo.GetAndPingItemByKey(searchKey)
	if item == nil {
		item = s.repo.GetFinished(searchKey)
	}
	if item == nil {
		w.WriteHeader(http.StatusNotFound)
		json.MarshalWrite(w, map[string]string{"message": "Item not found"})
		return
	}

	json.MarshalWrite(w, map[string]any{
		"key":        item.Key,
		"position":   item.Position,
		"finishedAt": item.FinishedAt,
	})
}

func handleAdminFinish(s *Server, w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if s.repo == nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.MarshalWrite(w, map[string]string{"message": "Repository not set"})
		return
	}
	headerTk := r.Header.Get("authorization")
	if !s.CheckAdminToken(headerTk) {
		w.WriteHeader(http.StatusForbidden)
		json.MarshalWrite(w, map[string]string{"message": "Access Denied"})
		return
	}
	var nItems uint = 1
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.MarshalWrite(w, map[string]string{"message": "Invalid query parameters"})
		return
	}
	if values, present := query["n"]; present {
		nParam, err := strconv.Atoi(values[0])
		if len(values) != 1 || err != nil || nParam <= 0 {
			w.WriteHeader(http.StatusBadRequest)
			json.MarshalWrite(w, map[string]string{"message": "n must be a positive integer"})
			return
		}
		nItems = uint(nParam)
	}

	items := s.repo.FinishItems(nItems)

	var resList []map[string]any
	for _, item := range items {
		resList = append(resList, map[string]any{
			"key":       item.Key,
			"createdAt": item.CreatedAt,
			"ip":        item.IP,
		})
	}

	w.WriteHeader(http.StatusOK)
	json.MarshalWrite(w, resList)
}

func handleAdminDeleteFinished(s *Server, w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if s.repo == nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.MarshalWrite(w, map[string]string{"message": "Repository not set"})
		return
	}
	headerTk := r.Header.Get("authorization")
	if !s.CheckAdminToken(headerTk) {
		w.WriteHeader(http.StatusForbidden)
		json.MarshalWrite(w, map[string]string{"message": "Access Denied"})
		return
	}

	key := r.PathValue("key")
	if !repository.IsValidKey(key) {
		w.WriteHeader(http.StatusBadRequest)
		json.MarshalWrite(w, map[string]string{"message": "Invalid key"})
		return
	}

	s.repo.DeleteFinished(key)

	w.WriteHeader(http.StatusOK)
	json.MarshalWrite(w, map[string]any{"key": key})
}

func handleAdminClearFinished(s *Server, w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if s.repo == nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.MarshalWrite(w, map[string]string{"message": "Repository not set"})
		return
	}
	headerTk := r.Header.Get("authorization")
	if !s.CheckAdminToken(headerTk) {
		w.WriteHeader(http.StatusForbidden)
		json.MarshalWrite(w, map[string]string{"message": "Access Denied"})
		return
	}
	s.repo.ClearFinished()

	w.WriteHeader(http.StatusOK)
	json.MarshalWrite(w, map[string]string{"status": "ok"})
}

func handleAdminClearQueue(s *Server, w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if s.repo == nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.MarshalWrite(w, map[string]string{"message": "Repository not set"})
		return
	}
	headerTk := r.Header.Get("authorization")
	if !s.CheckAdminToken(headerTk) {
		w.WriteHeader(http.StatusForbidden)
		json.MarshalWrite(w, map[string]string{"message": "Access Denied"})
		return
	}
	s.repo.ClearQueue()

	w.WriteHeader(http.StatusOK)
	json.MarshalWrite(w, map[string]string{"status": "ok"})
}

func handleAdminGetFinished(s *Server, w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if s.repo == nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.MarshalWrite(w, map[string]string{"message": "Repository not set"})
		return
	}
	headerTk := r.Header.Get("authorization")
	if !s.CheckAdminToken(headerTk) {
		w.WriteHeader(http.StatusForbidden)
		json.MarshalWrite(w, map[string]string{"message": "Access Denied"})
		return
	}

	key := r.PathValue("key")
	if !repository.IsValidKey(key) {
		w.WriteHeader(http.StatusBadRequest)
		json.MarshalWrite(w, map[string]string{"message": "Invalid key"})
		return
	}

	item := s.repo.GetFinished(key)
	if item == nil {
		w.WriteHeader(http.StatusNotFound)
		json.MarshalWrite(w, map[string]string{"message": "Item not found or not finished"})
		return
	}

	w.WriteHeader(http.StatusOK)
	json.MarshalWrite(w, map[string]any{"key": item.Key, "createdAt": item.CreatedAt, "ip": item.IP})
}

func (s *Server) Log(logType string, message string) {
	if s.log == nil {
		return
	}
	s.log.PrintLn(logType, "SERVER", message)
}

func (s *Server) Start() error {
	if (s.TLSCertFile == "") != (s.TLSKeyFile == "") {
		return errors.New("tlsCertFile and tlsKeyFile must both be configured")
	}
	scheme := "http"
	if s.TLSCertFile != "" {
		certificate, err := tls.LoadX509KeyPair(s.TLSCertFile, s.TLSKeyFile)
		if err != nil {
			return fmt.Errorf("load TLS certificate and key: %w", err)
		}
		s.Server.TLSConfig = &tls.Config{Certificates: []tls.Certificate{certificate}}
		scheme = "https"
	}
	listener, err := net.Listen("tcp", s.Server.Addr)
	if err != nil {
		return err
	}

	if s.AccessTk == "" {
		tk, err := repository.GenerateRandomKey()
		if err != nil {
			s.Log(log.Error, "Fail to generate secure initial admin access token")
		} else {
			s.AccessTk = tk
			s.Log(log.Warning, "Initial admin access token: "+s.AccessTk)
		}
	}

	go func() {
		s.Log(log.Info, "Starting on "+scheme+"://"+listener.Addr().String())
		var err error
		if scheme == "https" {
			err = s.Server.ServeTLS(listener, "", "")
		} else {
			err = s.Server.Serve(listener)
		}
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.Log(log.Error, err.Error())
		}
	}()
	return nil
}
func (s *Server) Stop() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := s.Server.Shutdown(ctx); err != nil {
		s.Log(log.Error, err.Error())
	}
}

func (s *Server) SetRepository(repo *repository.Repository) {
	s.repo = repo
}
func (s *Server) SetLogService(logService *log.LogService) {
	s.log = logService
}

// allowCORS permits credentialed access from the configured origin on every route.
// Handlers still enforce administrator authorization.
func (s *Server) allowCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.CORSAllowedOrigin != "" && s.CORSAllowedOrigin != "*" {
			w.Header().Set("Access-Control-Allow-Origin", s.CORSAllowedOrigin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
		}
		if r.Method == http.MethodOptions {
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func InitServer() *Server {
	mux := http.NewServeMux()
	s := &Server{
		Server: &http.Server{
			Addr:    "0.0.0.0:4242",
			Handler: mux,
		},
	}

	s.Server.Handler = s.allowCORS(mux)

	mux.Handle("POST /enter", s.requireClientIPHeader(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handleEnter(s, w, r)
	})))
	mux.Handle("GET /position", s.requireClientIPHeader(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlePosition(s, w, r)
	})))
	mux.HandleFunc("POST /admin/finishItems", func(w http.ResponseWriter, r *http.Request) {
		handleAdminFinish(s, w, r)
	})
	mux.HandleFunc("GET /admin/finishedItem/{key}", func(w http.ResponseWriter, r *http.Request) {
		handleAdminGetFinished(s, w, r)
	})
	mux.HandleFunc("DELETE /admin/finishedItem/{key}", func(w http.ResponseWriter, r *http.Request) {
		handleAdminDeleteFinished(s, w, r)
	})
	mux.HandleFunc("DELETE /admin/clearFinished", func(w http.ResponseWriter, r *http.Request) {
		handleAdminClearFinished(s, w, r)
	})
	mux.HandleFunc("DELETE /admin/clearQueue", func(w http.ResponseWriter, r *http.Request) {
		handleAdminClearQueue(s, w, r)
	})

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Header().Set("Content-Type", "application/json")
		json.MarshalWrite(w, map[string]string{"error": "Invalid route for Go Human Virtual Queue"})
		s.Log(log.Warning, "Invalid route hit: "+r.URL.Path)
	})

	return s
}
