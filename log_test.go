package main

import (
	"strings"
	"testing"

	"github.com/sgtdi/fswatcher"
)

func TestLogging(t *testing.T) {
	isQuiet = false

	testCases := []struct {
		name            string
		severity        string
		message         string
		expectedInLog   string
		expectedColor   string
		unexpectedColor []string
	}{
		{
			name:            "Log with SeverityInfo",
			severity:        SeverityInfo,
			message:         "info message",
			expectedInLog:   "info message",
			expectedColor:   "",
			unexpectedColor: []string{ColorGreen, ColorYellow, ColorRed},
		},
		{
			name:          "Log with SeveritySuccess",
			severity:      SeveritySuccess,
			message:       "success message",
			expectedInLog: "success message",
			expectedColor: ColorGreen,
		},
		{
			name:          "Log with SeverityWarn",
			severity:      SeverityWarn,
			message:       "warn message",
			expectedInLog: "warn message",
			expectedColor: ColorYellow,
		},
		{
			name:          "Log with SeverityError",
			severity:      SeverityError,
			message:       "error message",
			expectedInLog: "error message",
			expectedColor: ColorRed,
		},
		{
			name:            "Log with Default Severity",
			severity:        "some-unknown-severity",
			message:         "default message",
			expectedInLog:   "default message",
			expectedColor:   "",
			unexpectedColor: []string{ColorGreen, ColorYellow, ColorRed},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			output := captureOutput(func() {
				Log(tc.severity, tc.message)
			})

			if !strings.Contains(output, tc.expectedInLog) {
				t.Errorf("Expected output to contain '%s', but got '%s'", tc.expectedInLog, output)
			}

			if tc.expectedColor != "" && !strings.Contains(output, tc.expectedColor) {
				t.Errorf("Expected output to contain color '%s', but it didn't. Got: '%s'", tc.expectedColor, output)
			}

			for _, color := range tc.unexpectedColor {
				if strings.Contains(output, color) {
					t.Errorf("Output contained unexpected color '%s'. Got: '%s'", color, output)
				}
			}
		})
	}

	t.Run("Logf for formatted messages", func(t *testing.T) {
		output := captureOutput(func() {
			Logf(SeveritySuccess, "formatted %s %d", "message", 123)
		})

		expected := "formatted message 123"
		if !strings.Contains(output, expected) {
			t.Errorf("Expected output to contain '%s', but got '%s'", expected, output)
		}
	})

	t.Run("Logging in quiet mode", func(t *testing.T) {
		isQuiet = true
		defer func() { isQuiet = false }()

		output := captureOutput(func() {
			Log(SeverityInfo, "this should not appear")
			Logf(SeverityError, "this should also not appear")
		})

		if output != "" {
			t.Errorf("Expected no output in quiet mode, but got '%s'", output)
		}
	})
}

func TestLogLevelString(t *testing.T) {
	testCases := []struct {
		name     string
		level    string
		expected fswatcher.LogSeverity
	}{
		{"Debug", "debug", fswatcher.SeverityDebug},
		{"Info", "info", fswatcher.SeverityInfo},
		{"Error", "error", fswatcher.SeverityError},
		{"Warn", "warn", fswatcher.SeverityWarn},
		{"DefaultToWarn", "invalid", fswatcher.SeverityWarn},
		{"CaseInsensitive", "DEBUG", fswatcher.SeverityDebug},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := logLevelString(tc.level)
			if result != tc.expected {
				t.Errorf("Expected logLevelString('%s') to be %v, but got %v", tc.level, tc.expected, result)
			}
		})
	}
}
