package server

import (
	"encoding/json/v2"
	"net/http"
)

type Server struct {
	Addr string
}

func handleEnter(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.MarshalWrite(w, map[string]string{"message": "Entered!"})
}

func handlePosition(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	json.MarshalWrite(w, map[string]string{"message": "Position #1"})
}

func (s *Server) Start() error {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /enter", handleEnter)
	mux.HandleFunc("GET /position", handlePosition)
	return http.ListenAndServe(s.Addr, mux)
}

func InitServer() Server {
	return Server{
		Addr: "0.0.0.0:4242",
	}
}
