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
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	rootDir, err := filepath.Abs(filepath.Join(cwd, "../.."))
	if err != nil {
		t.Fatal(err)
	}

	binPath := filepath.Join(t.TempDir(), "vai")
	if runtime.GOOS == "windows" {
		binPath += ".exe"
	}

	buildCmd := exec.Command("go", "build", "-o", binPath, ".")
	buildCmd.Dir = rootDir
	if out, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to build vai: %v\n%s", err, out)
	}

	tmpDir, err := os.MkdirTemp("", "vai-integration-*")
	if err != nil {
		t.Fatal(err)
	}

	if runtime.GOOS != "windows" {
		t.Cleanup(func() {
			_ = os.RemoveAll(tmpDir)
		})
	}

	copyDir(t, cwd, tmpDir)

	initCmd := exec.Command("go", "mod", "init", "advanced-workflow")
	initCmd.Dir = tmpDir
	_ = initCmd.Run()

	ctx, cancel := context.WithCancel(context.Background())
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

	changeDetectedCh := make(chan struct{})
	startedCh := make(chan struct{})
	scannerDone := make(chan struct{})

	go func() {
		defer close(scannerDone)
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
				select {
				case changeDetectedCh <- struct{}{}:
				default:
				}
			}
		}
	}()

	select {
	case <-startedCh:
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for vai start")
	}

	time.Sleep(2 * time.Second)

	f, err := os.OpenFile(filepath.Join(tmpDir, "main.go"), os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Fprintln(f, "\n// touch 1")
	f.Close()

	select {
	case <-changeDetectedCh:
	case <-time.After(runTimeout):
		t.Fatal("Timeout waiting for 1st change detection")
	}

	time.Sleep(2 * time.Second)

	f, err = os.OpenFile(filepath.Join(tmpDir, "main.go"), os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Fprintln(f, "\n// touch 2")
	f.Close()

	select {
	case <-changeDetectedCh:
	case <-time.After(runTimeout):
		t.Fatal("Timeout waiting for 2nd change detection")
	}

	cancel()

	if runtime.GOOS == "windows" {
		_ = cmd.Process.Kill()
	} else {
		_ = cmd.Process.Signal(os.Interrupt)
	}

	_ = cmd.Wait()
	_ = stdout.Close()
	<-scannerDone

	if runtime.GOOS == "windows" {
		time.Sleep(750 * time.Millisecond)
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
