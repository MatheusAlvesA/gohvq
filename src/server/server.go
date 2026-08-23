package server

import (
	"MatheusAlvesA/gohvq/src/repository"
	"encoding/json/v2"
	"net/http"
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
	json.MarshalWrite(w, map[string]string{"key": item.Key})
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
		w.WriteHeader(http.StatusNotFound)
		json.MarshalWrite(w, map[string]string{"message": "Invalid key"})
		return
	}
	item, position := s.repo.GetAndPingItemByKey(searchKey)
	if item == nil {
		w.WriteHeader(http.StatusNotFound)
		json.MarshalWrite(w, map[string]string{"message": "Item not found"})
		return
	}

	json.MarshalWrite(w, map[string]any{"key": item.Key, "position": position})
}

func (s *Server) Start() error {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /enter", func(w http.ResponseWriter, r *http.Request) {
		handleEnter(s, w, r)
	})
	mux.HandleFunc("GET /position", func(w http.ResponseWriter, r *http.Request) {
		handlePosition(s, w, r)
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
