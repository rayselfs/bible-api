package logger

import (
	"log"
)

// Global logger instance
var (
	// Default logger for the application
	App *Logger

	// Standard logger for compatibility
	Standard = log.New(log.Writer(), "", log.LstdFlags)
)

// Init initializes the global logger
func Init() {
	App = New()
}

// GetAppLogger returns the application logger
func GetAppLogger() *Logger {
	if App == nil {
		Init()
	}
	return App
}

