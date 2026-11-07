package logger

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5/pgconn"
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
	Timestamp  string   `json:"timestamp"`
	Level      LogLevel `json:"level"`
	Message    string   `json:"message"`
	Source     string   `json:"source,omitempty"`
	Duration   string   `json:"duration,omitempty"`
	Phase      string   `json:"phase,omitempty"`
	Outcome    string   `json:"outcome,omitempty"`
	ErrorClass string   `json:"error_class,omitempty"`
	SQLState   string   `json:"sqlstate,omitempty"`
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
		message := "database diagnostic"
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
		message := "database diagnostic"
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
		message := "database diagnostic"
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
	// Never call fc: it renders SQL and interpolated private arguments.

	entry := GormLogEntry{
		Timestamp: time.Now().Format("2006-01-02 15:04:05"),
		Source:    "gorm",
		Duration:  fmt.Sprintf("%.3fms", float64(elapsed.Nanoseconds())/1e6),
		Phase:     "query",
		Outcome:   "success",
	}

	switch {
	case err != nil && l.logLevel >= logger.Error:
		entry.Level = ERROR
		entry.Message = "database query failed"
		entry.Outcome = "failure"
		entry.ErrorClass = "database"
		switch {
		case errors.Is(err, context.DeadlineExceeded):
			entry.ErrorClass = "timeout"
		case errors.Is(err, context.Canceled):
			entry.ErrorClass = "canceled"
		default:
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && len(pgErr.Code) == 5 {
				valid := true
				for _, c := range pgErr.Code {
					if !(c >= '0' && c <= '9' || c >= 'A' && c <= 'Z') {
						valid = false
					}
				}
				if valid {
					entry.SQLState = pgErr.Code
				}
			}
		}
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
