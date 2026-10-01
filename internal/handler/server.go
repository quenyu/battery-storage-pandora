package handler

import (
	"battery-storage-pandora/internal/model"
	"battery-storage-pandora/internal/service"
	"battery-storage-pandora/swagger"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Server struct {
	service *service.Service
}

func New(service *service.Service) http.Handler {
	server := &Server{service: service}
	mux := http.NewServeMux()
	ui := swagger.Handler()
	mux.Handle("GET /swagger", ui)
	mux.Handle("GET /swagger/", ui)
	mux.HandleFunc("GET /openapi.yaml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/yaml")
		_, _ = w.Write(swagger.OpenAPI)
	})
	server.routes(mux)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, model.NotFound())
	})

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := newID()
		w.Header().Set("X-Request-ID", requestID)
		r.Header.Set("X-Request-ID", requestID)
		started := time.Now()
		recorder := &statusWriter{ResponseWriter: w}
		mux.ServeHTTP(recorder, r)
		if recorder.status == 0 {
			recorder.status = http.StatusOK
		}
		slog.Info("request", "method", r.Method, "route", r.Pattern,
			"status", recorder.status, "duration_ms", time.Since(started).Milliseconds())
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(status int) {
	if status >= 100 && status < 200 {
		w.ResponseWriter.WriteHeader(status)
		return
	}
	if w.status != 0 {
		return
	}
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusWriter) Write(body []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(body)
}

func (w *statusWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func normalizePath(r *http.Request) error {
	for _, field := range []string{"employee_id", "credential_id", "battery_id", "operation_id"} {
		value := r.PathValue(field)
		if value == "" {
			continue
		}
		if !model.ValidUUID(value) {
			return model.Invalid("Неверный UUID в пути")
		}
		r.SetPathValue(field, strings.ToLower(value))
	}
	return nil
}

func noQuery(r *http.Request) error {
	if r.URL.RawQuery != "" || r.URL.ForceQuery {
		return model.Invalid("Query-параметры не поддерживаются")
	}
	return nil
}

func canonicalPath(r *http.Request) string {
	parts := strings.Split(r.URL.Path, "/")
	for i, part := range parts {
		if model.ValidUUID(part) {
			parts[i] = strings.ToLower(part)
		}
	}
	return strings.Join(parts, "/")
}

func newID() string {
	return uuid.NewString()
}
