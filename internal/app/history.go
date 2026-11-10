package app

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type runFiles struct {
	dir   string
	files map[string]*os.File
	names map[string]string
}

func newRunFiles(project string, now time.Time) (*runFiles, error) {
	canonical, err := filepath.Abs(project)
	if err != nil {
		return nil, fmt.Errorf("create run history: %w", err)
	}
	canonical, err = filepath.EvalSymlinks(canonical)
	if err != nil {
		return nil, fmt.Errorf("create run history: %w", err)
	}
	base := os.Getenv("XDG_DATA_HOME")
	if !filepath.IsAbs(base) {
		home, e := os.UserHomeDir()
		if e != nil {
			return nil, fmt.Errorf("locate run history: %w", e)
		}
		base = filepath.Join(home, ".local", "share")
	}
	hash := sha256.Sum256([]byte(canonical))
	parent := filepath.Join(base, "acton", "runs", hex.EncodeToString(hash[:]))
	//nolint:gosec // G703: base is the configured data directory; the project component is a SHA256 digest.
	if err = os.MkdirAll(parent, 0o700); err != nil {
		return nil, fmt.Errorf("create run history: %w", err)
	}
	// MkdirTemp gives each run a distinct timestamp directory even at the same instant.
	dir, err := os.MkdirTemp(parent, now.UTC().Format("20060102T150405.000000000Z")+"-")
	if err != nil {
		return nil, fmt.Errorf("create run history: %w", err)
	}
	return &runFiles{dir: dir, files: map[string]*os.File{}, names: map[string]string{}}, nil
}

func (h *runFiles) write(line string) error {
	var event struct {
		Message *string         `json:"msg"`
		Job     string          `json:"job"`
		JobID   string          `json:"jobID"`
		Matrix  json.RawMessage `json:"matrix"`
		Masked  bool            `json:"msg_masked"`
	}
	key, name := "runner", "runner"
	// Global logger records and --insecure-secrets output have no masking guarantee.
	msg := "Unmasked diagnostic omitted from saved history"
	if json.Unmarshal([]byte(line), &event) == nil && event.Message != nil && event.Masked {
		msg = *event.Message
		if event.Job != "" || event.JobID != "" {
			identity, _ := json.Marshal([]any{event.Job, event.JobID, event.Matrix})
			sum := sha256.Sum256(identity)
			key = hex.EncodeToString(sum[:])
			workflow, job, _ := strings.Cut(event.Job, "/")
			if event.JobID != "" {
				job = event.JobID
			}
			name = safeFilePart(workflow) + "-" + safeFilePart(job)
			// Matrix variants and colliding display names must not share a file.
			matrix := strings.TrimSpace(string(event.Matrix))
			if matrix != "" && matrix != "null" && matrix != "{}" {
				name += "-" + key[:16]
			}
			if previous, exists := h.names[name]; exists && previous != key {
				name += "-" + key[:16]
			}
		}
	}
	f := h.files[key]
	if f == nil {
		var err error
		f, err = os.OpenFile(filepath.Join(h.dir, name+".jsonl"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			return fmt.Errorf("write run history: %w", err)
		}
		h.files[key] = f
		h.names[name] = key
	}
	data, err := json.Marshal(struct {
		Message string `json:"msg"`
	}{msg})
	if err != nil {
		return fmt.Errorf("write run history: %w", err)
	}
	if _, err := fmt.Fprintln(f, string(data)); err != nil {
		return fmt.Errorf("write run history: %w", err)
	}
	return nil
}

func safeFilePart(s string) string {
	// Restrict path components and bound their length. Hashes preserve distinct identities.
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
		if b.Len() >= 48 {
			break
		}
	}
	if b.Len() == 0 {
		return "unknown"
	}
	return b.String()
}

func (h *runFiles) close() error {
	var first error
	for _, f := range h.files {
		if err := f.Close(); err != nil && first == nil {
			first = err
		}
	}
	h.files = nil
	return first
}

func (s *Session) writeRunFile(line string) {
	if s.runFiles == nil {
		return
	}
	if err := s.runFiles.write(line); err != nil {
		s.app.preferenceErr = "Cannot save run output: " + err.Error()
		s.closeRunFiles()
	}
}
func (s *Session) feedRun(line string) { s.run.Feed(line); s.writeRunFile(line) }
func (s *Session) closeRunFiles() {
	if s.runFiles != nil {
		if err := s.runFiles.close(); err != nil {
			s.app.preferenceErr = "Cannot close run output: " + err.Error()
		}
		s.runFiles = nil
	}
}
