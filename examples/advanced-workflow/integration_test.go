package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

const runTimeout = 30 * time.Second

func TestIntegration(t *testing.T) {
	// 1. Determine paths
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	rootDir, err := filepath.Abs(filepath.Join(cwd, "../.."))
	if err != nil {
		t.Fatal(err)
	}

	// 2. Build vai binary
	binPath := filepath.Join(t.TempDir(), "vai")
	if runtime.GOOS == "windows" {
		binPath += ".exe"
	}

	buildCmd := exec.Command("go", "build", "-o", binPath, ".")
	buildCmd.Dir = rootDir
	if out, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to build vai: %v\n%s", err, out)
	}

	// 3. Setup sandbox
	tmpDir := t.TempDir()
	copyDir(t, cwd, tmpDir)

	// Initialize go module
	initCmd := exec.Command("go", "mod", "init", "advanced-workflow")
	initCmd.Dir = tmpDir
	_ = initCmd.Run()

	// 4. Run vai
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd := exec.CommandContext(ctx, binPath, "--debug")
	cmd.Dir = tmpDir
	cmd.Env = append(os.Environ(), "TERM=dumb")

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = cmd.Stdout

	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	// Channels
	changeDetectedCh := make(chan struct{})
	startedCh := make(chan struct{})

	// Output scanner
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.Contains(line, "File watcher started") {
				select {
				case startedCh <- struct{}{}:
				default:
				}
			}
			if strings.Contains(line, "Change detected") {
				changeDetectedCh <- struct{}{}
			}
		}
	}()

	// Wait for startup
	select {
	case <-startedCh:
		t.Log("Vai started")
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for vai start")
	}

	// Wait a bit
	time.Sleep(2 * time.Second)

	// Modify file
	f, err := os.OpenFile(filepath.Join(tmpDir, "main.go"), os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Fprintln(f, "\n// touch 1")
	f.Close()
	t.Log("Modified main.go (1st)")

	// Wait for detection
	select {
	case <-changeDetectedCh:
		t.Log("1st change detected")
	case <-time.After(runTimeout):
		t.Fatal("Timeout waiting for 1st change detection")
	}

	// Wait a bit
	time.Sleep(2 * time.Second)

	// Modify again
	f, err = os.OpenFile(filepath.Join(tmpDir, "main.go"), os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Fprintln(f, "\n// touch 2")
	f.Close()
	t.Log("Modified main.go (2nd)")

	// Wait for detection
	select {
	case <-changeDetectedCh:
		t.Log("2nd change detected")
	case <-time.After(runTimeout):
		t.Fatal("Timeout waiting for 2nd change detection")
	}

	// Cleanup
	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		t.Logf("Failed to signal interrupt: %v", err)
	}
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		cancel()
		<-done
	}
}

func copyDir(t *testing.T, src, dst string) {
	err := filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		dstPath := filepath.Join(dst, relPath)
		if info.IsDir() {
			return os.MkdirAll(dstPath, info.Mode())
		}
		srcFile, err := os.Open(path)
		if err != nil {
			return err
		}
		defer srcFile.Close()
		dstFile, err := os.Create(dstPath)
		if err != nil {
			return err
		}
		defer dstFile.Close()
		_, err = io.Copy(dstFile, srcFile)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}
