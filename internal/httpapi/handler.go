package httpapi

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/felipe/rinha/internal/store"
	"github.com/jackc/pgx/v5"
)

type Handler struct {
	db           *store.DB
	maxBatchSize int
	metrics      *Metrics
}

//go:embed web/index.html
var dashboard []byte

func New(db *store.DB, maxBatchSize int) http.Handler {
	h := &Handler{db: db, maxBatchSize: maxBatchSize, metrics: NewMetrics()}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", h.dashboard)
	mux.HandleFunc("GET /healthz", h.health)
	mux.HandleFunc("GET /api/metrics", h.metricsAPI)
	mux.HandleFunc("POST /pessoas", h.createPerson)
	mux.HandleFunc("GET /pessoas/{id}", h.getPerson)
	mux.HandleFunc("GET /pessoas", h.searchPeople)
	mux.HandleFunc("POST /v1/records/bulk", h.bulk)
	mux.HandleFunc("GET /v1/records/", h.get)
	mux.HandleFunc("GET /v1/records", h.list)
	return logging(mux)
}

func (h *Handler) createPerson(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	created := false
	defer func() { h.metrics.ObserveInsert(created, time.Since(started)) }()

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var person store.Person
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&person); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "body must be valid JSON")
		return
	}
	if err := validatePerson(person); err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	createdPerson, err := h.db.CreatePerson(r.Context(), person)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			writeError(w, http.StatusUnprocessableEntity, "apelido already exists")
			return
		}
		writeError(w, http.StatusInternalServerError, "could not create person")
		return
	}
	created = true
	w.Header().Set("Location", "/pessoas/"+createdPerson.ID)
	writeJSON(w, http.StatusCreated, createdPerson)
}

func (h *Handler) getPerson(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !uuidPattern.MatchString(id) {
		writeError(w, http.StatusBadRequest, "id must be a valid UUID")
		return
	}
	person, err := h.db.GetPerson(r.Context(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "person not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not read person")
		return
	}
	writeJSON(w, http.StatusOK, person)
}

func (h *Handler) searchPeople(w http.ResponseWriter, r *http.Request) {
	term := strings.TrimSpace(r.URL.Query().Get("t"))
	if term == "" {
		writeError(w, http.StatusBadRequest, "query parameter t is required")
		return
	}
	people, err := h.db.SearchPeople(r.Context(), term)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not search people")
		return
	}
	writeJSON(w, http.StatusOK, people)
}

func (h *Handler) health(w http.ResponseWriter, r *http.Request) {
	if err := h.db.Ping(r.Context()); err != nil {
		writeError(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) dashboard(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(dashboard)
}

func (h *Handler) metricsAPI(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.metrics.Snapshot())
}

func (h *Handler) bulk(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	ok := false
	accepted := 0
	defer func() { h.metrics.Observe(accepted, ok, time.Since(started)) }()

	r.Body = http.MaxBytesReader(w, r.Body, 32<<20)
	var payload struct {
		Records []store.Record `json:"records"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "body must be valid JSON")
		return
	}
	if len(payload.Records) == 0 || len(payload.Records) > h.maxBatchSize {
		writeError(w, http.StatusBadRequest, "records must contain between 1 and max batch size items")
		return
	}
	seen := make(map[string]struct{}, len(payload.Records))
	for i, record := range payload.Records {
		record, err := validateRecord(record)
		if err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("record %d: %v", i+1, err))
			return
		}
		if _, exists := seen[record.Email]; exists {
			writeError(w, http.StatusBadRequest, "duplicate key in batch")
			return
		}
		seen[record.Email] = struct{}{}
		payload.Records[i] = record
	}
	if err := h.db.BulkUpsert(r.Context(), payload.Records); err != nil {
		writeError(w, http.StatusInternalServerError, "could not persist batch")
		return
	}
	ok = true
	accepted = len(payload.Records)
	writeJSON(w, http.StatusAccepted, map[string]int{"accepted": len(payload.Records)})
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	email := strings.TrimPrefix(r.URL.Path, "/v1/records/")
	if email == "" {
		writeError(w, http.StatusBadRequest, "email is required")
		return
	}
	record, err := h.db.Get(r.Context(), strings.ToLower(email))
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "record not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not read record")
		return
	}
	writeJSON(w, http.StatusOK, record)
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	limit := queryInt(r, "limit", 100)
	offset := queryInt(r, "offset", 0)
	if limit < 1 || limit > 1000 || offset < 0 {
		writeError(w, http.StatusBadRequest, "limit must be 1..1000 and offset must be non-negative")
		return
	}
	result, err := h.db.List(r.Context(), r.URL.Query().Get("prefix"), limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list records")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"records": result, "count": len(result)})
}

func queryInt(r *http.Request, name string, fallback int) int {
	if value := r.URL.Query().Get(name); value != "" {
		parsed, err := strconv.Atoi(value)
		if err == nil {
			return parsed
		}
	}
	return fallback
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil {
			defer r.Body.Close()
		}
		next.ServeHTTP(w, r)
	})
}
