package snapshot

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/Bispulm/dockline/internal/engine"
	"github.com/Bispulm/dockline/internal/model"
)

func makeSnapshot(t *testing.T, output string) model.Manifest {
	t.Helper()
	w, err := Begin(output)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Abort()
	m, err := engine.Import(context.Background(), strings.NewReader("{\"id\":\"a\"}\nbad\n{\"id\":\"b\"}\n"), w.Records, w.Rejected, model.Options{InvalidPolicy: "skip", DuplicatePolicy: "fail", MaxRecordBytes: 1024})
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Publish(context.Background(), m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestPublicationAndVerification(t *testing.T) {
	dir := t.TempDir()
	output := filepath.Join(dir, "snapshot")
	want := makeSnapshot(t, output)
	got, err := Verify(context.Background(), output)
	if err != nil || got != want {
		t.Fatalf("got %+v %v", got, err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 || entries[0].Name() != "snapshot" {
		t.Fatalf("unexpected leftovers: %v %v", entries, err)
	}
	if _, err = Begin(output); !errors.Is(err, ErrExists) {
		t.Fatalf("existing output not protected: %v", err)
	}
}

func TestAbortAndCancelledPublication(t *testing.T) {
	for _, cancelBeforePublish := range []bool{false, true} {
		dir := t.TempDir()
		output := filepath.Join(dir, "snapshot")
		w, err := Begin(output)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(output); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("uncommitted output is visible")
		}
		if cancelBeforePublish {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			if err := w.Publish(ctx, model.Manifest{}); !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
		}
		if err := w.Abort(); err != nil {
			t.Fatal(err)
		}
		if err := w.Abort(); err != nil {
			t.Fatal(err)
		}
		entries, _ := os.ReadDir(dir)
		if len(entries) != 0 {
			t.Fatalf("aborted import left files: %v", entries)
		}
	}
}

func TestConcurrentDestinationReservation(t *testing.T) {
	output := filepath.Join(t.TempDir(), "snapshot")
	const attempts = 12
	start := make(chan struct{})
	winners := make(chan *Writer, attempts)
	failures := make(chan error, attempts)
	var wg sync.WaitGroup
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			w, err := Begin(output)
			if err != nil {
				failures <- err
			} else {
				winners <- w
			}
		}()
	}
	close(start)
	wg.Wait()
	close(winners)
	close(failures)
	if len(winners) != 1 {
		t.Fatalf("expected one owner, got %d", len(winners))
	}
	for w := range winners {
		if err := w.Abort(); err != nil {
			t.Fatal(err)
		}
	}
	for err := range failures {
		if !errors.Is(err, ErrBusy) {
			t.Fatal(err)
		}
	}
}

func TestCorruptionAndManifestMismatch(t *testing.T) {
	for _, target := range []string{"records.ndjson", "rejected.ndjson", "counts", "format"} {
		t.Run(target, func(t *testing.T) {
			output := filepath.Join(t.TempDir(), "snapshot")
			m := makeSnapshot(t, output)
			if target == "counts" || target == "format" {
				if target == "counts" {
					m.Stats.RecordsWritten++
				} else {
					m.Format = "unknown"
				}
				data, _ := json.Marshal(m)
				if err := os.WriteFile(filepath.Join(output, "manifest.json"), data, 0600); err != nil {
					t.Fatal(err)
				}
			} else if err := os.WriteFile(filepath.Join(output, target), []byte("corrupt\n"), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := Verify(context.Background(), output); err == nil {
				t.Fatal("corruption was accepted")
			}
		})
	}
}

func TestPublicationIOErrorDoesNotRemoveExistingData(t *testing.T) {
	dir := t.TempDir()
	output := filepath.Join(dir, "snapshot")
	w, err := Begin(output)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Records.Close(); err != nil {
		t.Fatal(err)
	}
	if err := w.Publish(context.Background(), model.Manifest{}); err == nil {
		t.Fatal("closed output should fail")
	}
	if err := w.Publish(context.Background(), model.Manifest{}); err == nil {
		t.Fatal("repeated publication should fail safely")
	}
	_ = w.Abort()
	if _, err := os.Stat(output); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed output exists: %v", err)
	}
}
