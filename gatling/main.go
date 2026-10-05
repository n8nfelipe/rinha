package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

//go:embed web/index.html
var indexHTML []byte

type report struct {
	Name    string    `json:"name"`
	Updated time.Time `json:"updated"`
	URL     string    `json:"url"`
}

type runState struct {
	Running    bool      `json:"running"`
	Total      int       `json:"total"`
	Users      int       `json:"users"`
	StartedAt  time.Time `json:"started_at,omitempty"`
	FinishedAt time.Time `json:"finished_at,omitempty"`
	ExitCode   *int      `json:"exit_code,omitempty"`
	Error      string    `json:"error,omitempty"`
	Log        []string  `json:"log"`
	Reports    []report  `json:"reports"`
}

type runner struct {
	mu        sync.Mutex
	state     runState
	cancel    context.CancelFunc
	workspace string
	baseURL   string
}

func main() {
	r := &runner{
		workspace: env("WORKSPACE", "/workspace"),
		baseURL:   env("BASE_URL", "http://api:8080"),
		state:     runState{Log: []string{}, Reports: []report{}},
	}
	r.refreshReports()

	mux := http.NewServeMux()
	mux.HandleFunc("/", r.index)
	mux.HandleFunc("/api/status", r.status)
	mux.HandleFunc("/api/run", r.start)
	mux.HandleFunc("/api/stop", r.stop)
	mux.Handle("/reports/", http.StripPrefix("/reports/", http.FileServer(http.Dir(filepath.Join(r.workspace, "target", "gatling")))))

	addr := env("HTTP_ADDR", ":8090")
	server := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	fmt.Printf("gatling web interface listening on %s\n", addr)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func (r *runner) index(w http.ResponseWriter, req *http.Request) {
	if req.URL.Path != "/" {
		http.NotFound(w, req)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(indexHTML)
}

func (r *runner) status(w http.ResponseWriter, _ *http.Request) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.state.Reports = r.reports()
	writeJSON(w, http.StatusOK, r.state)
}

func (r *runner) start(w http.ResponseWriter, req *http.Request) {
	var input struct {
		Total int `json:"total"`
		Users int `json:"users"`
	}
	if err := json.NewDecoder(io.LimitReader(req.Body, 1<<20)).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "body must be valid JSON")
		return
	}
	if input.Total < 1 || input.Total > 500000 || input.Users < 1 || input.Users > 1000 {
		writeError(w, http.StatusBadRequest, "total deve estar entre 1 e 500000; usuários entre 1 e 1000")
		return
	}
	if input.Total%input.Users != 0 {
		writeError(w, http.StatusBadRequest, "total deve ser divisível pelo número de usuários")
		return
	}

	r.mu.Lock()
	if r.state.Running {
		r.mu.Unlock()
		writeError(w, http.StatusConflict, "já existe uma carga em execução")
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	r.cancel = cancel
	r.state = runState{Running: true, Total: input.Total, Users: input.Users, StartedAt: time.Now(), Log: []string{"Preparando simulação..."}}
	r.mu.Unlock()

	go r.run(ctx, input.Total, input.Users)
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "started"})
}

func (r *runner) stop(w http.ResponseWriter, _ *http.Request) {
	r.mu.Lock()
	if r.cancel == nil || !r.state.Running {
		r.mu.Unlock()
		writeError(w, http.StatusConflict, "não há carga em execução")
		return
	}
	r.cancel()
	r.mu.Unlock()
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "stopping"})
}

func (r *runner) run(ctx context.Context, total, users int) {
	cmd := exec.CommandContext(ctx, "mvn", "-U", "gatling:test", "-DbaseUrl="+r.baseURL, "-Dtotal="+strconv.Itoa(total), "-Dusers="+strconv.Itoa(users))
	cmd.Dir = r.workspace
	cmd.Stdout = &logWriter{appendLine: r.appendLog}
	cmd.Stderr = &logWriter{appendLine: r.appendLog}
	r.appendLog(fmt.Sprintf("Executando %d requisições com %d usuários...", total, users))
	err := cmd.Run()

	r.mu.Lock()
	defer r.mu.Unlock()
	code := 0
	if err != nil {
		code = 1
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			code = exitErr.ExitCode()
		}
		if ctx.Err() != nil {
			r.state.Error = "execução interrompida"
		} else {
			r.state.Error = err.Error()
		}
	}
	r.state.Running = false
	r.state.FinishedAt = time.Now()
	r.state.ExitCode = &code
	r.state.Reports = r.reports()
	r.cancel = nil
	if err == nil {
		r.state.Log = append(r.state.Log, "Simulação concluída com sucesso.")
	} else {
		r.state.Log = append(r.state.Log, "Simulação encerrada com erro.")
	}
}

type logWriter struct{ appendLine func(string) }

func (w *logWriter) Write(p []byte) (int, error) {
	for _, line := range strings.Split(strings.TrimRight(string(p), "\n"), "\n") {
		if strings.TrimSpace(line) != "" {
			w.appendLine(line)
		}
	}
	return len(p), nil
}

func (r *runner) appendLog(line string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	line = strings.TrimSpace(line)
	r.state.Log = append(r.state.Log, line)
	if len(r.state.Log) > 100 {
		r.state.Log = r.state.Log[len(r.state.Log)-100:]
	}
}

func (r *runner) refreshReports() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.state.Reports = r.reports()
}

func (r *runner) reports() []report {
	root := filepath.Join(r.workspace, "target", "gatling")
	entries, err := os.ReadDir(root)
	if err != nil {
		return []report{}
	}
	result := make([]report, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		result = append(result, report{Name: entry.Name(), Updated: info.ModTime(), URL: "/reports/" + entry.Name() + "/index.html"})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Updated.After(result[j].Updated) })
	return result
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func env(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
