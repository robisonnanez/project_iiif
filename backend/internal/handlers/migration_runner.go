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
	Scope struct {
		ProjectKey string `json:"project_key"`
		TenantKey  string `json:"tenant_key"`
	} `json:"scope"`
}

type migrationStatus struct {
	Running         bool                `json:"running"`
	StartedAt       time.Time           `json:"started_at,omitempty"`
	FinishedAt      time.Time           `json:"finished_at,omitempty"`
	ExitCode        int                 `json:"exit_code"`
	Message         string              `json:"message,omitempty"`
	Logs            []string            `json:"logs"`
	JobID           string              `json:"job_id,omitempty"`
	Source          string              `json:"source,omitempty"`
	Metrics         map[string]int      `json:"metrics,omitempty"`
	ProgressPercent int                 `json:"progress_percent,omitempty"`
	CurrentDocument string              `json:"current_document,omitempty"`
	Items           []migrationDocState `json:"items,omitempty"`
}

type migrationDocState struct {
	DocumentID string `json:"document_id"`
	PDFName    string `json:"pdf_name"`
	ImagesDone int    `json:"images_done"`
	ImagesTotal int   `json:"images_total"`
	Status     string `json:"status"`
	Message    string `json:"message,omitempty"`
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
			Metrics:  map[string]int{},
			Items:    []migrationDocState{},
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
	r.status.Metrics = map[string]int{}
	r.status.ProgressPercent = 0
	r.status.CurrentDocument = ""
	r.status.Items = []migrationDocState{}
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
	s.Items = append([]migrationDocState(nil), r.status.Items...)
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
	if strings.TrimSpace(req.Scope.ProjectKey) != "" {
		env = append(env, "MIGRATION_SCOPE_PROJECT="+req.Scope.ProjectKey)
	}
	if strings.TrimSpace(req.Scope.TenantKey) != "" {
		env = append(env, "MIGRATION_SCOPE_TENANT="+req.Scope.TenantKey)
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
	if exitCode == 0 && migrationItemsHaveErrors(r.status.Items) {
		exitCode = 1
		message = "Migracion finalizada con errores"
	}
	r.status.ExitCode = exitCode
	r.status.Message = message
	if r.status.ExitCode == 0 {
		r.status.ProgressPercent = 100
	}
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
	if strings.HasPrefix(line, "PROGRESS_DOC ") {
		r.updateDocProgress(strings.TrimPrefix(line, "PROGRESS_DOC "))
	}
	if strings.HasPrefix(line, "METRIC ") {
		payload := strings.TrimPrefix(line, "METRIC ")
		parts := strings.SplitN(payload, "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			if v, err := strconv.Atoi(value); err == nil {
				r.status.Metrics[key] = v
			}
		}
	}
	r.recomputeProgress()
}

func (r *migrationRunner) updateDocProgress(payload string) {
	parts := strings.SplitN(payload, "|", 6)
	if len(parts) < 5 {
		return
	}
	documentID := strings.TrimSpace(parts[0])
	pdfName := strings.TrimSpace(parts[1])
	imagesDone, _ := strconv.Atoi(strings.TrimSpace(parts[2]))
	imagesTotal, _ := strconv.Atoi(strings.TrimSpace(parts[3]))
	status := strings.TrimSpace(parts[4])
	message := ""
	if len(parts) == 6 {
		message = strings.TrimSpace(parts[5])
	}
	r.status.CurrentDocument = pdfName
	for i := range r.status.Items {
		if r.status.Items[i].DocumentID == documentID {
			r.status.Items[i].PDFName = pdfName
			r.status.Items[i].ImagesDone = imagesDone
			r.status.Items[i].ImagesTotal = imagesTotal
			r.status.Items[i].Status = status
			r.status.Items[i].Message = message
			return
		}
	}
	r.status.Items = append(r.status.Items, migrationDocState{
		DocumentID: documentID,
		PDFName:    pdfName,
		ImagesDone: imagesDone,
		ImagesTotal: imagesTotal,
		Status:     status,
		Message:    message,
	})
}

func (r *migrationRunner) recomputeProgress() {
	totalDocs := r.status.Metrics["docs_total"]
	currentDoc := r.status.Metrics["current_doc"]
	if totalDocs > 0 {
		percent := int(float64(currentDoc) / float64(totalDocs) * 100)
		if percent < 0 {
			percent = 0
		}
		if percent > 100 {
			percent = 100
		}
		r.status.ProgressPercent = percent
	}
}

func (r *migrationRunner) finishWithError(message string, exitCode int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.status.Running = false
	r.status.FinishedAt = time.Now()
	r.status.ExitCode = exitCode
	r.status.Message = message
	r.status.ProgressPercent = 0
	r.status.Logs = append(r.status.Logs, "ERROR "+message)
}

func migrationItemsHaveErrors(items []migrationDocState) bool {
	for _, item := range items {
		if strings.EqualFold(strings.TrimSpace(item.Status), "error") {
			return true
		}
	}
	return false
}
