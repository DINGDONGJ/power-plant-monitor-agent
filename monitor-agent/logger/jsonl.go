package logger

import (
	"encoding/json"
	"os"
	"sync"
)

// JSONLLogger JSONL 格式日志写入器
type JSONLLogger struct {
	mu   sync.Mutex
	file *os.File
}

func NewJSONLLogger(path string) (*JSONLLogger, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}
	return &JSONLLogger{file: f}, nil
}

func (l *JSONLLogger) Write(v any) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = l.file.Write(append(data, '\n'))
	return err
}

func (l *JSONLLogger) Close() error {
	return l.file.Close()
}
