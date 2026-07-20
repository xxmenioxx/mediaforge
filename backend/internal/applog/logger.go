package applog

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	maxLogSize = 10 * 1024 * 1024
	logBackups = 5
)

var (
	mu     sync.Mutex
	output io.Writer = os.Stdout
)

// Initialize mirrors the standard logger to stdout and a size-rotated
// backend.log. Docker can collect stdout while the mounted file survives a
// container restart.
func Initialize(logDir string) error {
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return err
	}
	w := &rotatingWriter{path: filepath.Join(logDir, "backend.log")}
	mu.Lock()
	output = io.MultiWriter(os.Stdout, w)
	log.SetOutput(output)
	log.SetFlags(log.Ldate | log.Ltime | log.LUTC | log.Lmicroseconds)
	mu.Unlock()
	Event("info", "backend", "logging_initialized", map[string]any{"path": w.path}, nil)
	return nil
}

func Event(level, component, event string, fields map[string]any, eventErr error) {
	record := map[string]any{
		"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
		"level":     level,
		"component": component,
		"event":     event,
	}
	for key, value := range fields {
		record[key] = value
	}
	if eventErr != nil {
		record["error"] = eventErr.Error()
	}
	payload, err := json.Marshal(record)
	if err != nil {
		payload = []byte(fmt.Sprintf(`{"level":"error","component":"logging","event":"marshal_failed","error":%q}`, err.Error()))
	}
	mu.Lock()
	_, _ = output.Write(append(payload, '\n'))
	mu.Unlock()
}

type rotatingWriter struct {
	path string
	mu   sync.Mutex
}

func (w *rotatingWriter) Write(payload []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if info, err := os.Stat(w.path); err == nil && info.Size()+int64(len(payload)) > maxLogSize {
		if err := w.rotate(); err != nil {
			return 0, err
		}
	}
	file, err := os.OpenFile(w.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return 0, err
	}
	defer file.Close()
	return file.Write(payload)
}

func (w *rotatingWriter) rotate() error {
	_ = os.Remove(fmt.Sprintf("%s.%d", w.path, logBackups))
	for index := logBackups - 1; index >= 1; index-- {
		oldPath := fmt.Sprintf("%s.%d", w.path, index)
		newPath := fmt.Sprintf("%s.%d", w.path, index+1)
		if err := os.Rename(oldPath, newPath); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	if err := os.Rename(w.path, w.path+".1"); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
