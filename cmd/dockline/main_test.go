//go:build linux

package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// A subprocess keeps real OS signals away from the test runner.
func TestSignalHelperProcess(t *testing.T) {
	if os.Getenv("DOCKLINE_SIGNAL_HELPER") != "1" {
		return
	}
	os.Args = []string{"dockline", "import", "--input", "-",
		"--output", os.Getenv("DOCKLINE_SIGNAL_OUTPUT")}
	main()
}

func TestSignalsCancelAndCleanUnpublishedOutput(t *testing.T) {
	for _, sig := range []os.Signal{os.Interrupt, syscall.SIGTERM} {
		t.Run(sig.String(), func(t *testing.T) {
			exe, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			dir := t.TempDir()
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, exe, "-test.run=^TestSignalHelperProcess$")
			cmd.Env = append(os.Environ(), "DOCKLINE_SIGNAL_HELPER=1",
				"DOCKLINE_SIGNAL_OUTPUT="+filepath.Join(dir, "snapshot"))
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			input, err := cmd.StdinPipe()
			if err != nil {
				t.Fatal(err)
			}
			defer input.Close()
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			defer func() {
				if cmd.ProcessState == nil {
					_ = cmd.Process.Kill()
					_ = cmd.Wait()
				}
			}()
			// Keep input flowing: cancellation is cooperative between records,
			// not a promise to interrupt an idle blocking reader.
			finished := make(chan struct{})
			go func() {
				defer close(finished)
				buf := bytes.Repeat([]byte("\n"), 4096)
				for {
					if _, err := input.Write(buf); err != nil {
						return
					}
				}
			}()

			ready := false
			for !ready {
				entries, err := os.ReadDir(dir)
				if err != nil {
					t.Fatal(err)
				}
				for _, entry := range entries {
					if strings.HasPrefix(entry.Name(), ".dockline-lock-") {
						ready = true
						break
					}
				}
				if ctx.Err() != nil {
					t.Fatal("child did not acquire the destination lock")
				}
				if !ready {
					time.Sleep(time.Millisecond)
				}
			}
			if err := cmd.Process.Signal(sig); err != nil {
				t.Fatal(err)
			}
			err = cmd.Wait()
			_ = input.Close()
			<-finished
			var exit *exec.ExitError
			if ctx.Err() != nil || !errors.As(err, &exit) || exit.ExitCode() != 130 {
				t.Fatalf("exit=%v stdout=%q stderr=%q", err, stdout.String(), stderr.String())
			}
			if stdout.Len() != 0 || !strings.Contains(stderr.String(), "context canceled") {
				t.Fatalf("unexpected output: stdout=%q stderr=%q", stdout.String(), stderr.String())
			}
			entries, err := os.ReadDir(dir)
			if err != nil || len(entries) != 0 {
				t.Fatalf("unpublished data or lock left behind: %v; err=%v", entries, err)
			}
		})
	}
}
