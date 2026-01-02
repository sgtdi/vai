package main

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestWatch_SetDefaults(t *testing.T) {
	t.Run("sets defaults for zero values", func(t *testing.T) {
		w := &Vai{}
		w.setDefaults()

		if w.Config.BufferSize != 4096 {
			t.Errorf("Expected BufferSize to be 4096, got %d", w.Config.BufferSize)
		}
		if w.Config.Severity != SeverityWarn.String() {
			t.Errorf("Expected Severity to be 'warn', got '%s'", w.Config.Severity)
		}
		if w.Config.Cooldown != 100*time.Millisecond {
			t.Errorf("Expected Cooldown to be 100ms, got %v", w.Config.Cooldown)
		}
	})

	t.Run("does not override existing values", func(t *testing.T) {
		w := &Vai{
			Config: Config{
				BufferSize: 512,
				Severity:   "debug",
				Cooldown:   50 * time.Millisecond,
			},
		}
		w.setDefaults()

		if w.Config.BufferSize != 512 {
			t.Errorf("Expected BufferSize to remain 512, got %d", w.Config.BufferSize)
		}
		if w.Config.Severity != "debug" {
			t.Errorf("Expected Severity to remain 'debug', got '%s'", w.Config.Severity)
		}
		if w.Config.Cooldown != 50*time.Millisecond {
			t.Errorf("Expected Cooldown to remain 50ms, got %v", w.Config.Cooldown)
		}
	})
}

func TestWatch_Save(t *testing.T) {
	w := &Vai{
		Config: Config{Severity: "info"},
		Jobs: map[string]Job{
			"test-job": {Cmd: "go", Params: []string{"test"}},
		},
	}

	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "vai.yml")

	err := w.save(filePath)
	if err != nil {
		t.Fatalf("save() returned an unexpected error: %v", err)
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read saved file: %v", err)
	}

	var loadedVai Vai
	if err := yaml.Unmarshal(data, &loadedVai); err != nil {
		t.Fatalf("Failed to unmarshal saved data: %v", err)
	}

	if !reflect.DeepEqual(w.Config, loadedVai.Config) {
		t.Errorf("Saved config does not match original. Got %+v, want %+v", loadedVai.Config, w.Config)
	}
	if !reflect.DeepEqual(w.Jobs, loadedVai.Jobs) {
		t.Errorf("Saved jobs do not match original. Got %+v, want %+v", loadedVai.Jobs, w.Jobs)
	}
}

func TestAggregateRegex(t *testing.T) {
	vai := &Vai{
		Jobs: map[string]Job{
			"job1": {
				Trigger: &Trigger{Regex: []string{`\.go$`, `!\.test\.go$`, `\.mod$`}},
			},
			"job2": {
				Trigger: &Trigger{Regex: []string{`\.html$`, `!\.test\.go$`}},
			},
			"job3": {},
			"job4": {
				Trigger: &Trigger{Regex: []string{`\.go$`}},
			},
		},
	}

	inc, exc := vai.aggregateRegex()

	sort.Strings(inc)
	sort.Strings(exc)

	expectedInc := []string{`\.go$`, `\.html$`, `\.mod$`}
	sort.Strings(expectedInc)
	expectedExc := []string{`\.test\.go$`}
	sort.Strings(expectedExc)

	if !reflect.DeepEqual(inc, expectedInc) {
		t.Errorf("Expected inclusion patterns %v, got %v", expectedInc, inc)
	}
	if !reflect.DeepEqual(exc, expectedExc) {
		t.Errorf("Expected exclusion patterns %v, got %v", expectedExc, exc)
	}
}

func TestMatchRegex(t *testing.T) {
	testCases := []struct {
		name     string
		path     string
		patterns []string
		expected bool
	}{
		{
			name:     "Matches inclusion pattern",
			path:     "main.go",
			patterns: []string{`\.go$`},
			expected: true,
		},
		{
			name:     "Does not match inclusion pattern",
			path:     "image.png",
			patterns: []string{`\.go$`},
			expected: false,
		},
		{
			name:     "Matches exclusion pattern",
			path:     "main_test.go",
			patterns: []string{`\.go$`, `!_test\.go$`},
			expected: false,
		},
		{
			name:     "Matches inclusion but not exclusion",
			path:     "main.go",
			patterns: []string{`\.go$`, `!_test\.go$`},
			expected: true,
		},
		{
			name:     "Empty patterns match everything",
			path:     "anything",
			patterns: []string{},
			expected: true,
		},
		{
			name:     "Matches multiple inclusions",
			path:     "go.mod",
			patterns: []string{`\.go$`, `go\.mod$`},
			expected: true,
		},
		{
			name:     "Exclusion overrides match",
			path:     "vendor/foo.go",
			patterns: []string{`\.go$`, `!vendor/`},
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := matchRegex(tc.path, tc.patterns)
			if result != tc.expected {
				t.Errorf("matchRegex(%q, %v) = %v, want %v", tc.path, tc.patterns, result, tc.expected)
			}
		})
	}
}

func TestFileExists(t *testing.T) {
	t.Run("returns true for existing file", func(t *testing.T) {
		tmpfile, err := os.CreateTemp("", "testfile")
		if err != nil {
			t.Fatal(err)
		}
		defer os.Remove(tmpfile.Name())

		if !fileExists(tmpfile.Name()) {
			t.Errorf("Expected fileExists to return true for %s", tmpfile.Name())
		}
	})

	t.Run("returns false for non-existent file", func(t *testing.T) {
		if fileExists("non_existent_file.xyz") {
			t.Error("Expected fileExists to return false for non-existent file")
		}
	})

	t.Run("returns false for directory", func(t *testing.T) {
		tmpdir := t.TempDir()
		if fileExists(tmpdir) {
			t.Error("Expected fileExists to return false for directory")
		}
	})
}

func TestParsePath(t *testing.T) {
	t.Run("returns provided path if not empty", func(t *testing.T) {
		path := parsePath("/custom/path")
		if path != "/custom/path" {
			t.Errorf("Expected '/custom/path', got '%s'", path)
		}
	})

	t.Run("returns cwd if path is empty", func(t *testing.T) {
		cwd, _ := os.Getwd()
		path := parsePath("")
		if path != cwd {
			t.Errorf("Expected cwd '%s', got '%s'", cwd, path)
		}
	})
}

func TestParseRegex(t *testing.T) {
	t.Run("parses comma-separated patterns", func(t *testing.T) {
		patterns := parseRegex("p1, p2 ,p3")
		expected := []string{"p1", "p2", "p3"}
		if !reflect.DeepEqual(patterns, expected) {
			t.Errorf("Expected %v, got %v", expected, patterns)
		}
	})

	t.Run("returns default patterns when empty", func(t *testing.T) {
		patterns := parseRegex("")
		expected := []string{".*\\.go$", "^go\\.mod$", "^go\\.sum$"}
		if !reflect.DeepEqual(patterns, expected) {
			t.Errorf("Expected default patterns, got %v", patterns)
		}
	})
}

func TestParseEnv(t *testing.T) {
	t.Run("parses comma-separated env vars", func(t *testing.T) {
		envMap := parseEnv("K1=V1, K2=V2 , K3=V3")
		expected := map[string]string{"K1": "V1", "K2": "V2", "K3": "V3"}
		if !reflect.DeepEqual(envMap, expected) {
			t.Errorf("Expected %v, got %v", expected, envMap)
		}
	})

	t.Run("handles invalid pairs gracefully", func(t *testing.T) {
		envMap := parseEnv("K1=V1,invalid,K3=V3")
		expected := map[string]string{"K1": "V1", "K3": "V3"}
		if !reflect.DeepEqual(envMap, expected) {
			t.Errorf("Expected %v, got %v", expected, envMap)
		}
	})
}

func TestParseFlags(t *testing.T) {
	t.Run("CmdFlags take precedence", func(t *testing.T) {
		cmdFlags := []string{"cmd1", "cmd2"}
		posArgs := []string{"pos1", "pos2"}
		result := parseFlags(cmdFlags, posArgs)

		if !reflect.DeepEqual(result, cmdFlags) {
			t.Errorf("Expected %v, got %v", cmdFlags, result)
		}
	})

	t.Run("PositionalArgs used if CmdFlags empty", func(t *testing.T) {
		posArgs := []string{"echo", "hello"}
		result := parseFlags(nil, posArgs)
		expected := []string{"echo hello"}

		if !reflect.DeepEqual(result, expected) {
			t.Errorf("Expected %v, got %v", expected, result)
		}
	})

	t.Run("Returns nil if both empty", func(t *testing.T) {
		// Initialize logger to prevent panic if parseFlags logs
		logger = newLogger(SeverityDebug)
		result := parseFlags(nil, nil)
		if result != nil {
			t.Errorf("Expected nil, got %v", result)
		}
	})
}

func TestClearCLI(t *testing.T) {
	t.Run("clears screen", func(t *testing.T) {
		if runtime.GOOS != "windows" {
			output := captureOutput(func() {
				clearCLI()
			})
			expected := "\033[H\033[2J"
			if output != expected {
				t.Errorf("Expected %q, got %q", expected, output)
			}
		} else {
			// On Windows, just run it to ensure no panic
			clearCLI()
		}
	})
}
