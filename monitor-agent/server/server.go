package server

import (
	"encoding/json"
	"net/http"
	"strconv"

	"monitor-agent/monitor"
)

type Server struct {
	monitor *monitor.Monitor
	mux     *http.ServeMux
}

func New(m *monitor.Monitor) *Server {
	s := &Server{monitor: m, mux: http.NewServeMux()}
	s.mux.HandleFunc("/health", s.handleHealth)
	s.mux.HandleFunc("/status", s.handleStatus)
	s.mux.HandleFunc("/metrics/recent", s.handleMetricsRecent)
	s.mux.HandleFunc("/events/recent", s.handleEventsRecent)
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	status := "ok"
	if !s.monitor.IsRunning() {
		status = "degraded"
	}
	json.NewEncoder(w).Encode(map[string]string{"status": status})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s.monitor.GetStatus())
}

func (s *Server) handleMetricsRecent(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	n := parseIntParam(r, "n", 10)
	json.NewEncoder(w).Encode(s.monitor.GetRecentMetrics(n))
}

func (s *Server) handleEventsRecent(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	n := parseIntParam(r, "n", 10)
	json.NewEncoder(w).Encode(s.monitor.GetRecentEvents(n))
}

func parseIntParam(r *http.Request, key string, def int) int {
	v := r.URL.Query().Get(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return def
	}
	return n
}
