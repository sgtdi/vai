package main

import (
	"strings"
	"testing"
)

func TestLogger_Log(t *testing.T) {
	testCases := []struct {
		name          string
		loggerLevel   Severity
		logLevel      Severity
		op            Op
		message       string
		shouldLog     bool
		expectedColor string
	}{
		{
			name:          "Info log with Info logger",
			loggerLevel:   SeverityInfo,
			logLevel:      SeverityInfo,
			op:            OpInfo,
			message:       "info message",
			shouldLog:     true,
			expectedColor: ColorCyan,
		},
		{
			name:          "Debug log with Info logger (should filter)",
			loggerLevel:   SeverityInfo,
			logLevel:      SeverityDebug,
			op:            OpSuccess,
			message:       "debug message",
			shouldLog:     false,
			expectedColor: "",
		},
		{
			name:          "Warn log with Debug logger",
			loggerLevel:   SeverityDebug,
			logLevel:      SeverityWarn,
			op:            OpWarn,
			message:       "warn message",
			shouldLog:     true,
			expectedColor: ColorYellow,
		},
		{
			name:          "Error log with Error logger",
			loggerLevel:   SeverityError,
			logLevel:      SeverityError,
			op:            OpError,
			message:       "error message",
			shouldLog:     true,
			expectedColor: ColorRed,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			l := newLogger(tc.loggerLevel)
			output := captureOutput(func() {
				l.log(tc.logLevel, tc.op, "%s", tc.message)
			})

			if tc.shouldLog {
				if !strings.Contains(output, tc.message) {
					t.Errorf("Expected output to contain '%s', but got '%s'", tc.message, output)
				}
				if !strings.Contains(output, tc.expectedColor) {
					t.Errorf("Expected output to contain color code '%s', but got '%s'", tc.expectedColor, output)
				}
			} else {
				if output != "" {
					t.Errorf("Expected no output, but got '%s'", output)
				}
			}
		})
	}
}

func TestLogger_HelperMethods(t *testing.T) {
	l := newLogger(SeverityDebug)

	t.Run("Debug", func(t *testing.T) {
		output := captureOutput(func() {
			l.debug(OpSuccess, "debug msg")
		})
		if !strings.Contains(output, "debug msg") {
			t.Error("Expected debug message to be logged")
		}
	})

	t.Run("Info", func(t *testing.T) {
		output := captureOutput(func() {
			l.info(OpInfo, "info msg")
		})
		if !strings.Contains(output, "info msg") {
			t.Error("Expected info message to be logged")
		}
	})

	t.Run("Warn", func(t *testing.T) {
		output := captureOutput(func() {
			l.warn(OpWarn, "warn msg")
		})
		if !strings.Contains(output, "warn msg") {
			t.Error("Expected warn message to be logged")
		}
	})

	t.Run("Error", func(t *testing.T) {
		output := captureOutput(func() {
			l.error(OpError, "error msg")
		})
		if !strings.Contains(output, "error msg") {
			t.Error("Expected error message to be logged")
		}
	})
}

func TestParseSeverity(t *testing.T) {
	testCases := []struct {
		input    string
		expected Severity
	}{
		{"debug", SeverityDebug},
		{"DEBUG", SeverityDebug},
		{"info", SeverityInfo},
		{"warn", SeverityWarn},
		{"warning", SeverityWarn},
		{"error", SeverityError},
		{"invalid", SeverityWarn},
	}

	for _, tc := range testCases {
		got := parseSeverity(tc.input)
		if got != tc.expected {
			t.Errorf("parseSeverity(%q) = %v, want %v", tc.input, got, tc.expected)
		}
	}
}
