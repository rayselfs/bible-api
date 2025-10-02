package logger

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"
)

// JSONMarshal marshals data to JSON string
func JSONMarshal(v interface{}) ([]byte, error) {
	return json.Marshal(v)
}

// LogLevel represents the logging level
type LogLevel string

const (
	DEBUG LogLevel = "DEBUG"
	INFO  LogLevel = "INFO"
	WARN  LogLevel = "WARN"
	ERROR LogLevel = "ERROR"
)

// LogEntry represents a structured log entry
type LogEntry struct {
	Timestamp string   `json:"timestamp"`
	Level     LogLevel `json:"level"`
	Message   string   `json:"message"`
}

// Logger provides structured logging functionality
type Logger struct {
	*log.Logger
}

// New creates a new structured logger
func New() *Logger {
	return &Logger{
		Logger: log.New(os.Stdout, "", 0),
	}
}

// formatMessage formats the log entry as JSON
func (l *Logger) formatMessage(level LogLevel, message string) string {
	entry := LogEntry{
		Timestamp: time.Now().Format("2006-01-02 15:04:05"),
		Level:     level,
		Message:   message,
	}

	jsonData, err := json.Marshal(entry)
	if err != nil {
		return fmt.Sprintf(`{"timestamp": "%s", "level": "ERROR", "message": "Failed to format log entry: %v"}`,
			time.Now().Format("2006-01-02 15:04:05"), err)
	}

	return string(jsonData)
}

// Debug logs a debug message
func (l *Logger) Debug(message string) {
	l.Print(l.formatMessage(DEBUG, message))
}

// Info logs an info message
func (l *Logger) Info(message string) {
	l.Print(l.formatMessage(INFO, message))
}

// Warn logs a warning message
func (l *Logger) Warn(message string) {
	l.Print(l.formatMessage(WARN, message))
}

// Error logs an error message
func (l *Logger) Error(message string) {
	l.Print(l.formatMessage(ERROR, message))
}

// Fatal logs a fatal error message and exits
func (l *Logger) Fatal(message string) {
	l.Print(l.formatMessage(ERROR, message))
	os.Exit(1)
}

// Fatalf logs a formatted fatal error message and exits
func (l *Logger) Fatalf(format string, args ...interface{}) {
	message := fmt.Sprintf(format, args...)
	l.Fatal(message)
}

// Infof logs a formatted info message
func (l *Logger) Infof(format string, args ...interface{}) {
	message := fmt.Sprintf(format, args...)
	l.Info(message)
}

// Errorf logs a formatted error message
func (l *Logger) Errorf(format string, args ...interface{}) {
	message := fmt.Sprintf(format, args...)
	l.Error(message)
}

// Warnf logs a formatted warning message
func (l *Logger) Warnf(format string, args ...interface{}) {
	message := fmt.Sprintf(format, args...)
	l.Warn(message)
}
