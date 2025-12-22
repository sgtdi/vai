package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/sgtdi/fswatcher"
)

// isQuiet is a global flag to disable all logging
var isQuiet bool

// Severity levels to control the color of the output
const (
	SeverityInfo    = "info"
	SeveritySuccess = "success"
	SeverityWarn    = "warn"
	SeverityError   = "error"
)

// ANSI Color Codes for terminal output
const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorCyan   = "\033[36m"
	colorPurple = "\033[35m"
	colorWhite  = "\033[97m"
)

// logImpl handles formatting and printing
func logImpl(severity, message string) {
	// Get HH:MM:SS
	timestamp := time.Now().Format("15:04:05")

	// Determine the color
	var color string
	switch severity {
	case SeveritySuccess:
		color = colorGreen
	case SeverityWarn:
		color = colorYellow
	case SeverityError:
		color = colorRed
		message = fmt.Sprintf("%s%s%s", colorRed, message, colorReset)
	default:
		color = colorCyan
	}

	// Print the formatted string: [hh:mm:ss] - Message
	fmt.Printf("[%s%s%s] %s\n",
		color,
		timestamp,
		colorReset,
		message,
	)
}

// Log prints a formatted log mesage
func Log(severity, message string) {
	if isQuiet {
		return
	}
	logImpl(severity, message)
}

// Logf support formatted strings
func Logf(severity, format string, a ...any) {
	message := fmt.Sprintf(format, a...)
	Log(severity, message)
}

// logLevelString converts a string to a fswatcher.LogLevel
func logLevelString(level string) fswatcher.LogSeverity {
	switch strings.ToLower(level) {
	case "debug":
		return fswatcher.SeverityDebug
	case "info":
		return fswatcher.SeverityInfo
	case "error":
		return fswatcher.SeverityError
	default:
		return fswatcher.SeverityWarn
	}
}
