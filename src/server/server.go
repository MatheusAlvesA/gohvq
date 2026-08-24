package server

import (
	"MatheusAlvesA/gohvq/src/repository"
	"encoding/json/v2"
	"net/http"
	"strconv"
)

type Server struct {
	Addr string
	repo *repository.Repository
}

func handleEnter(s *Server, w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if s.repo == nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.MarshalWrite(w, map[string]string{"message": "Repository not set"})
		return
	}
	item, err := s.repo.CreateItem()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.MarshalWrite(w, map[string]string{"message": "Fail to create new item"})
		return
	}
	w.WriteHeader(http.StatusCreated)
	json.MarshalWrite(w, map[string]any{
		"key":      item.Key,
		"position": s.repo.GetCurrentQueueSize(),
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
	item, position := s.repo.GetAndPingItemByKey(searchKey)
	if item == nil {
		item = s.repo.GetFinished(searchKey)
		position = 0
	}
	if item == nil {
		w.WriteHeader(http.StatusNotFound)
		json.MarshalWrite(w, map[string]string{"message": "Item not found"})
		return
	}

	json.MarshalWrite(w, map[string]any{
		"key":        item.Key,
		"position":   position,
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
	var nItems uint = 1
	nParam, err := strconv.Atoi(r.URL.Query().Get("n"))
	if err == nil && nParam > 0 {
		nItems = uint(nParam)
	}

	items := s.repo.FinishItems(nItems)

	var resList []map[string]any
	for _, item := range items {
		resList = append(resList, map[string]any{
			"key":       item.Key,
			"createdAt": item.CreatedAt,
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

func handleAdminClearFinished(s *Server, w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if s.repo == nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.MarshalWrite(w, map[string]string{"message": "Repository not set"})
		return
	}
	s.repo.ClearFinished()

	w.WriteHeader(http.StatusOK)
	json.MarshalWrite(w, map[string]string{"status": "ok"})
}

func handleAdminClearQueue(s *Server, w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if s.repo == nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.MarshalWrite(w, map[string]string{"message": "Repository not set"})
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
	json.MarshalWrite(w, map[string]any{"key": item.Key, "createdAt": item.CreatedAt})
}

func (s *Server) Start() error {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /enter", func(w http.ResponseWriter, r *http.Request) {
		handleEnter(s, w, r)
	})
	mux.HandleFunc("GET /position", func(w http.ResponseWriter, r *http.Request) {
		handlePosition(s, w, r)
	})
	mux.HandleFunc("GET /admin/finishItems", func(w http.ResponseWriter, r *http.Request) {
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
	return http.ListenAndServe(s.Addr, mux)
}

func (s *Server) SetRepository(repo *repository.Repository) {
	s.repo = repo
}

func InitServer() *Server {
	return &Server{
		Addr: "0.0.0.0:4242",
	}
}
