package logger

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"gorm.io/gorm/logger"
)

// GormLogger is a custom GORM logger that outputs JSON format
type GormLogger struct {
	appLogger *Logger
	logLevel  logger.LogLevel
}

// GormLogEntry represents a GORM log entry
type GormLogEntry struct {
	Timestamp string   `json:"timestamp"`
	Level     LogLevel `json:"level"`
	Message   string   `json:"message"`
	Source    string   `json:"source,omitempty"`
	Duration  string   `json:"duration,omitempty"`
	Rows      int64    `json:"rows,omitempty"`
	SQL       string   `json:"sql,omitempty"`
}

// NewGormLogger creates a new GORM logger with JSON output
func NewGormLogger(appLogger *Logger, logLevel logger.LogLevel) logger.Interface {
	return &GormLogger{
		appLogger: appLogger,
		logLevel:  logLevel,
	}
}

// LogMode sets log mode
func (l *GormLogger) LogMode(level logger.LogLevel) logger.Interface {
	newLogger := *l
	newLogger.logLevel = level
	return &newLogger
}

// Info logs info messages
func (l *GormLogger) Info(ctx context.Context, msg string, data ...interface{}) {
	if l.logLevel >= logger.Info {
		message := fmt.Sprintf(msg, data...)
		entry := GormLogEntry{
			Timestamp: time.Now().Format("2006-01-02 15:04:05"),
			Level:     INFO,
			Message:   message,
			Source:    "gorm",
		}
		l.printJSON(entry)
	}
}

// Warn logs warning messages
func (l *GormLogger) Warn(ctx context.Context, msg string, data ...interface{}) {
	if l.logLevel >= logger.Warn {
		message := fmt.Sprintf(msg, data...)
		entry := GormLogEntry{
			Timestamp: time.Now().Format("2006-01-02 15:04:05"),
			Level:     WARN,
			Message:   message,
			Source:    "gorm",
		}
		l.printJSON(entry)
	}
}

// Error logs error messages
func (l *GormLogger) Error(ctx context.Context, msg string, data ...interface{}) {
	if l.logLevel >= logger.Error {
		message := fmt.Sprintf(msg, data...)
		entry := GormLogEntry{
			Timestamp: time.Now().Format("2006-01-02 15:04:05"),
			Level:     ERROR,
			Message:   message,
			Source:    "gorm",
		}
		l.printJSON(entry)
	}
}

// Trace logs SQL queries
func (l *GormLogger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	if l.logLevel <= logger.Silent {
		return
	}

	elapsed := time.Since(begin)
	sql, rows := fc()

	entry := GormLogEntry{
		Timestamp: time.Now().Format("2006-01-02 15:04:05"),
		Source:    "gorm",
		Duration:  fmt.Sprintf("%.3fms", float64(elapsed.Nanoseconds())/1e6),
		Rows:      rows,
		SQL:       sql,
	}

	switch {
	case err != nil && l.logLevel >= logger.Error:
		entry.Level = ERROR
		entry.Message = fmt.Sprintf("SQL Error: %v", err)
	case elapsed > 200*time.Millisecond && l.logLevel >= logger.Warn:
		entry.Level = WARN
		entry.Message = "Slow SQL query detected"
	case l.logLevel >= logger.Info:
		entry.Level = INFO
		entry.Message = "SQL query executed"
	default:
		return
	}

	l.printJSON(entry)
}

// printJSON prints the log entry as JSON
func (l *GormLogger) printJSON(entry GormLogEntry) {
	jsonData, err := json.Marshal(entry)
	if err != nil {
		l.appLogger.Error(fmt.Sprintf("Failed to marshal GORM log entry: %v", err))
		return
	}
	l.appLogger.Print(string(jsonData))
}

