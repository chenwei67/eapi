package eapi

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

// LogLevel represents the severity level of a log message
type LogLevel int

const (
	// LogLevelSilent suppresses all log output
	LogLevelSilent LogLevel = iota
	// LogLevelError shows only error messages
	LogLevelError
	// LogLevelWarn shows error and warning messages
	LogLevelWarn
	// LogLevelInfo shows error, warning, and info messages
	LogLevelInfo
	// LogLevelDebug shows all messages including debug information
	LogLevelDebug
)

// String returns the string representation of the log level
func (l LogLevel) String() string {
	switch l {
	case LogLevelSilent:
		return "silent"
	case LogLevelError:
		return "error"
	case LogLevelWarn:
		return "warn"
	case LogLevelInfo:
		return "info"
	case LogLevelDebug:
		return "debug"
	default:
		return "unknown"
	}
}

// ParseLogLevel parses a string into a LogLevel
func ParseLogLevel(level string) LogLevel {
	switch strings.ToLower(level) {
	case "silent":
		return LogLevelSilent
	case "error":
		return LogLevelError
	case "warn", "warning":
		return LogLevelWarn
	case "info":
		return LogLevelInfo
	case "debug":
		return LogLevelDebug
	default:
		return LogLevelInfo // default to info level
	}
}

// Logger provides structured logging with different levels
type Logger struct {
	level      LogLevel
	output     io.Writer
	errorOut   io.Writer
	colorized  bool
	timestamp  bool
	strictMode bool
}

// NewLogger creates a new logger with the specified level
func NewLogger(level LogLevel) *Logger {
	return &Logger{
		level:     level,
		output:    os.Stdout,
		errorOut:  os.Stderr,
		colorized: true,
		timestamp: false,
	}
}

// SetLevel sets the logging level
func (l *Logger) SetLevel(level LogLevel) {
	l.level = level
}

// SetOutput sets the output writer for info and debug messages
func (l *Logger) SetOutput(w io.Writer) {
	l.output = w
}

// SetErrorOutput sets the output writer for error and warning messages
func (l *Logger) SetErrorOutput(w io.Writer) {
	l.errorOut = w
}

// SetColorized enables or disables colored output
func (l *Logger) SetColorized(colorized bool) {
	l.colorized = colorized
}

// SetTimestamp enables or disables timestamp in log messages
func (l *Logger) SetTimestamp(timestamp bool) {
	l.timestamp = timestamp
}

// SetStrictMode enables or disables strict mode
func (l *Logger) SetStrictMode(strict bool) {
	l.strictMode = strict
}

// formatMessage formats a log message with optional timestamp and color
func (l *Logger) formatMessage(level, color, message string) string {
	var parts []string
	
	if l.timestamp {
		parts = append(parts, time.Now().Format("2006-01-02 15:04:05"))
	}
	
	if l.colorized && color != "" {
		parts = append(parts, fmt.Sprintf("%s[%s]\033[0m", color, level))
	} else {
		parts = append(parts, fmt.Sprintf("[%s]", level))
	}
	
	parts = append(parts, message)
	return strings.Join(parts, " ")
}

// Error logs an error message
func (l *Logger) Error(format string, args ...interface{}) {
	if l.level < LogLevelError {
		return
	}
	
	message := fmt.Sprintf(format, args...)
	color := "\033[31m" // red
	if l.strictMode {
		color = "\033[31m" // red for strict mode
	}
	
	formatted := l.formatMessage("ERROR", color, message)
	fmt.Fprintln(l.errorOut, formatted)
}

// Warn logs a warning message
func (l *Logger) Warn(format string, args ...interface{}) {
	if l.level < LogLevelWarn {
		return
	}
	
	message := fmt.Sprintf(format, args...)
	color := "\033[33m" // yellow
	
	formatted := l.formatMessage("WARN", color, message)
	fmt.Fprintln(l.errorOut, formatted)
}

// Info logs an info message
func (l *Logger) Info(format string, args ...interface{}) {
	if l.level < LogLevelInfo {
		return
	}
	
	message := fmt.Sprintf(format, args...)
	color := "\033[36m" // cyan
	
	formatted := l.formatMessage("INFO", color, message)
	fmt.Fprintln(l.output, formatted)
}

// Debug logs a debug message
func (l *Logger) Debug(format string, args ...interface{}) {
	if l.level < LogLevelDebug {
		return
	}
	
	message := fmt.Sprintf(format, args...)
	color := "\033[37m" // white
	
	formatted := l.formatMessage("DEBUG", color, message)
	fmt.Fprintln(l.output, formatted)
}

// StrictError logs an error in strict mode (red) or normal mode (stderr)
func (l *Logger) StrictError(format string, args ...interface{}) {
	if l.strictMode {
		l.Error(format, args...)
	} else {
		message := fmt.Sprintf(format, args...)
		fmt.Fprintln(l.errorOut, message)
	}
}

// StrictWarn logs a warning in strict mode (yellow) or normal mode (stderr)
func (l *Logger) StrictWarn(format string, args ...interface{}) {
	if l.strictMode {
		l.Warn(format, args...)
	} else {
		message := fmt.Sprintf(format, args...)
		fmt.Fprintln(l.errorOut, message)
	}
}

// Global logger instance
var globalLogger = NewLogger(LogLevelInfo)

// SetGlobalLogLevel sets the global logger level
func SetGlobalLogLevel(level LogLevel) {
	globalLogger.SetLevel(level)
}

// SetGlobalLogColorized sets the global logger colorization
func SetGlobalLogColorized(colorized bool) {
	globalLogger.SetColorized(colorized)
}

// SetGlobalLogTimestamp sets the global logger timestamp
func SetGlobalLogTimestamp(timestamp bool) {
	globalLogger.SetTimestamp(timestamp)
}

// SetGlobalLogStrictMode sets the global logger strict mode
func SetGlobalLogStrictMode(strict bool) {
	globalLogger.SetStrictMode(strict)
}

// GetGlobalLogger returns the global logger instance
func GetGlobalLogger() *Logger {
	return globalLogger
}

// Global logging functions
func LogError(format string, args ...interface{}) {
	globalLogger.Error(format, args...)
}

func LogWarn(format string, args ...interface{}) {
	globalLogger.Warn(format, args...)
}

func LogInfo(format string, args ...interface{}) {
	globalLogger.Info(format, args...)
}

func LogDebug(format string, args ...interface{}) {
	globalLogger.Debug(format, args...)
}

func LogStrictError(format string, args ...interface{}) {
	globalLogger.StrictError(format, args...)
}

func LogStrictWarn(format string, args ...interface{}) {
	globalLogger.StrictWarn(format, args...)
}