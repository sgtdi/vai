package main

import (
	"fmt"
	"strings"
	"time"
)

const (
	ColorReset  = "\033[0m"
	ColorRed    = "\033[31m"
	ColorGreen  = "\033[32m"
	ColorYellow = "\033[33m"
	ColorCyan   = "\033[36m"
	ColorPurple = "\033[35m"
	ColorWhite  = "\033[97m"
	ColorGray   = "\033[90m"
)

// colorize returns a string with the specified color
func colorize(color string, values ...any) string {
	if len(values) == 0 {
		return ""
	}

	return fmt.Sprintf(
		"%s%s%s",
		color,
		fmt.Sprint(values...),
		ColorReset,
	)
}

// red returns a string with red color
func red(v ...any) string {
	return colorize(ColorRed, v...)
}

// green returns a string with green color
func green(v ...any) string {
	return colorize(ColorGreen, v...)
}

// yellow returns a string with yellow color
func yellow(v ...any) string {
	return colorize(ColorYellow, v...)
}

// cyan returns a string with cyan color
func cyan(v ...any) string {
	return colorize(ColorCyan, v...)
}

// purple returns a string with purple color
func purple(v ...any) string {
	return colorize(ColorPurple, v...)
}

// white returns a string with white color
func white(v ...any) string {
	return colorize(ColorWhite, v...)
}

// gray returns a string with gray color
func gray(v ...any) string {
	return colorize(ColorGray, v...)
}

// Severity represents the severity level of a log message
type Severity int

const (
	SeverityDebug Severity = iota
	SeverityInfo
	SeverityWarn
	SeverityError
)

// parseSeverity converts a string to a Severity level
func parseSeverity(s string) Severity {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return SeverityDebug
	case "info":
		return SeverityInfo
	case "warn", "warning":
		return SeverityWarn
	case "error":
		return SeverityError
	default:
		return SeverityWarn
	}
}

// String returns the string representation of the severity level
func (s Severity) String() string {
	switch s {
	case SeverityDebug:
		return "debug"
	case SeverityInfo:
		return "info"
	case SeverityWarn:
		return "warn"
	case SeverityError:
		return "error"
	default:
		return "warn"
	}
}

// Operation type
type Op int

const (
	OpSuccess Op = iota
	OpError
	OpInfo
	OpWarn
	OpTrigger
)

// Association between ops and colors
var opColor = map[Op]string{
	OpSuccess: ColorGreen,
	OpError:   ColorRed,
	OpInfo:    ColorCyan,
	OpWarn:    ColorYellow,
	OpTrigger: ColorPurple,
}

// Logger represents a logger instance with the specified log level
type Logger struct {
	level Severity
}

// newLogger creates a new logger instance with the specified log level
func newLogger(level Severity) *Logger {
	return &Logger{level: level}
}

// log logs a message with the specified level, operation, format, and arguments
func (l *Logger) log(level Severity, op Op, format string, args ...any) {
	// Severity filter
	if level < l.level {
		return
	}

	// Timestamp (HH:MM:SS)
	timestamp := time.Now().Format("15:04:05")

	// Color based on op
	color := opColor[op]

	// Format message
	message := fmt.Sprintf(format, args...)

	// Final output
	fmt.Printf(
		"%s[%s]%s %s\n",
		color,
		timestamp,
		ColorReset,
		message,
	)
}

// debug logs a debug message with the specified operation, format, and arguments
func (l *Logger) debug(op Op, format string, args ...any) {
	l.log(SeverityDebug, op, format, args...)
}

// info logs an info message with the specified operation, format, and arguments
func (l *Logger) info(op Op, format string, args ...any) {
	l.log(SeverityInfo, op, format, args...)
}

// warn logs a warn message with the specified operation, format, and arguments
func (l *Logger) warn(op Op, format string, args ...any) {
	l.log(SeverityWarn, op, format, args...)
}

// error logs an error message with the specified operation, format, and arguments
func (l *Logger) error(op Op, format string, args ...any) {
	l.log(SeverityError, op, format, args...)
}
