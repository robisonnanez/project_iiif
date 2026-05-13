package handlers

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

type migrationStartRequest struct {
	Source struct {
		Type  string `json:"type"`
		Local struct {
			Path string `json:"path"`
		} `json:"local"`
		SSH struct {
			Host       string `json:"host"`
			Port       int    `json:"port"`
			User       string `json:"user"`
			Path       string `json:"path"`
			PrivateKey string `json:"private_key"`
		} `json:"ssh"`
	} `json:"source"`
}

type migrationStatus struct {
	Running    bool      `json:"running"`
	StartedAt  time.Time `json:"started_at,omitempty"`
	FinishedAt time.Time `json:"finished_at,omitempty"`
	ExitCode   int       `json:"exit_code"`
	Message    string    `json:"message,omitempty"`
	Logs       []string  `json:"logs"`
	JobID      string    `json:"job_id,omitempty"`
	Source     string    `json:"source,omitempty"`
}

type migrationRunner struct {
	mu          sync.Mutex
	status      migrationStatus
	maxLogLines int
}

func newMigrationRunner(maxLogLines int) *migrationRunner {
	if maxLogLines <= 0 {
		maxLogLines = 500
	}
	return &migrationRunner{
		maxLogLines: maxLogLines,
		status: migrationStatus{
			ExitCode: -1,
			Logs:     []string{},
		},
	}
}

func (r *migrationRunner) Start(req migrationStartRequest) error {
	r.mu.Lock()
	if r.status.Running {
		r.mu.Unlock()
		return fmt.Errorf("la migracion ya esta en ejecucion")
	}
	sourceSummary := normalizeSourceSummary(req)
	jobID := strconv.FormatInt(time.Now().UnixNano(), 10)
	r.status.Running = true
	r.status.StartedAt = time.Now()
	r.status.FinishedAt = time.Time{}
	r.status.ExitCode = -1
	r.status.Message = "Migracion iniciada"
	r.status.Logs = []string{fmt.Sprintf("INFO inicio de migracion %s", sourceSummary)}
	r.status.JobID = jobID
	r.status.Source = sourceSummary
	r.mu.Unlock()

	go r.run(req)
	return nil
}

func (r *migrationRunner) Status() migrationStatus {
	r.mu.Lock()
	defer r.mu.Unlock()
	copyLogs := append([]string(nil), r.status.Logs...)
	s := r.status
	s.Logs = copyLogs
	return s
}

func (r *migrationRunner) run(req migrationStartRequest) {
	execPath, _ := os.Executable()
	binPath := filepath.Join(filepath.Dir(execPath), "migrate-local-to-mysql")
	if _, err := os.Stat(binPath); err != nil {
		binPath = "./migrate-local-to-mysql"
	}

	cmd := exec.Command(binPath)
	env := os.Environ()
	env = append(env, "MIGRATION_SOURCE_TYPE="+strings.ToLower(strings.TrimSpace(req.Source.Type)))
	if strings.EqualFold(req.Source.Type, "local") {
		env = append(env, "MIGRATION_SOURCE_LOCAL_PATH="+req.Source.Local.Path)
	}
	if strings.EqualFold(req.Source.Type, "ssh") {
		port := req.Source.SSH.Port
		if port <= 0 {
			port = 22
		}
		env = append(env, "MIGRATION_SOURCE_SSH_HOST="+req.Source.SSH.Host)
		env = append(env, "MIGRATION_SOURCE_SSH_PORT="+strconv.Itoa(port))
		env = append(env, "MIGRATION_SOURCE_SSH_USER="+req.Source.SSH.User)
		env = append(env, "MIGRATION_SOURCE_SSH_PATH="+req.Source.SSH.Path)
		env = append(env, "MIGRATION_SOURCE_SSH_PRIVATE_KEY="+req.Source.SSH.PrivateKey)
	}
	cmd.Env = env
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	if err := cmd.Start(); err != nil {
		r.finishWithError(fmt.Sprintf("no se pudo iniciar migracion: %v", err), -1)
		return
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		r.collectLogs(stdout)
	}()
	go func() {
		defer wg.Done()
		r.collectLogs(stderr)
	}()

	waitErr := cmd.Wait()
	wg.Wait()

	exitCode := 0
	message := "Migracion finalizada correctamente"
	if waitErr != nil {
		message = waitErr.Error()
		if exitErr, ok := waitErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
		}
	}

	r.mu.Lock()
	r.status.Running = false
	r.status.FinishedAt = time.Now()
	r.status.ExitCode = exitCode
	r.status.Message = message
	r.mu.Unlock()
}

func normalizeSourceSummary(req migrationStartRequest) string {
	if strings.EqualFold(req.Source.Type, "ssh") {
		port := req.Source.SSH.Port
		if port <= 0 {
			port = 22
		}
		return fmt.Sprintf("ssh:%s@%s:%d path=%s", req.Source.SSH.User, req.Source.SSH.Host, port, req.Source.SSH.Path)
	}
	return fmt.Sprintf("local path=%s", req.Source.Local.Path)
}

func (r *migrationRunner) collectLogs(pipe io.Reader) {
	scanner := bufio.NewScanner(pipe)
	for scanner.Scan() {
		r.appendLog(scanner.Text())
	}
}

func (r *migrationRunner) appendLog(line string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.status.Logs) > r.maxLogLines {
		r.status.Logs = r.status.Logs[len(r.status.Logs)-r.maxLogLines:]
	}
	r.status.Logs = append(r.status.Logs, line)
}

func (r *migrationRunner) finishWithError(message string, exitCode int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.status.Running = false
	r.status.FinishedAt = time.Now()
	r.status.ExitCode = exitCode
	r.status.Message = message
	r.status.Logs = append(r.status.Logs, "ERROR "+message)
}
