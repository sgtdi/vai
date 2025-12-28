package main

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestWatch_SetDefaults(t *testing.T) {
	t.Run("sets defaults for zero values", func(t *testing.T) {
		w := &Vai{}
		w.SetDefaults()

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
		w.SetDefaults()

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
		Config: Config{Path: "/tmp", Severity: "info"},
		Jobs: map[string]Job{
			"test-job": {Cmd: "go", Params: []string{"test"}},
		},
	}

	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "vai.yml")

	err := w.Save(filePath)
	if err != nil {
		t.Fatalf("Save() returned an unexpected error: %v", err)
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
				Trigger: &Trigger{Regex: []string{"\\.go$", "!\\.test\\.go$", "\\.mod$"}},
			},
			"job2": {
				Trigger: &Trigger{Regex: []string{"\\.html$", "!\\.test\\.go$"}},
			},
			"job3": {},
			"job4": {
				Trigger: &Trigger{Regex: []string{"\\.go$"}},
			},
		},
	}

	inc, exc := aggregateRegex(vai)

	sort.Strings(inc)
	sort.Strings(exc)

	expectedInc := []string{"\\.go$", "\\.html$", "\\.mod$"}
	sort.Strings(expectedInc)
	expectedExc := []string{"\\.test\\.go$"}
	sort.Strings(expectedExc)

	if !reflect.DeepEqual(inc, expectedInc) {
		t.Errorf("Expected inclusion patterns %v, got %v", expectedInc, inc)
	}
	if !reflect.DeepEqual(exc, expectedExc) {
		t.Errorf("Expected exclusion patterns %v, got %v", expectedExc, exc)
	}
}
