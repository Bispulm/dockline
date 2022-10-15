package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Bispulm/dockline/internal/snapshot"
)

type unavailableOutput struct{}

func (unavailableOutput) Write([]byte) (int, error) {
	return 0, errors.New("output unavailable")
}

func TestHelpAndVersionReportOutputFailures(t *testing.T) {
	for _, args := range [][]string{nil, {"help"}, {"--help"}, {"-h"}, {"version"}} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			var stderr bytes.Buffer
			code := Run(context.Background(), args, strings.NewReader(""), unavailableOutput{}, &stderr)
			if code != 1 || !strings.Contains(stderr.String(), "output unavailable") {
				t.Fatalf("exit=%d stderr=%q", code, stderr.String())
			}
		})
	}
}

func TestPublishedSnapshotSurvivesSummaryWriteFailure(t *testing.T) {
	dir := t.TempDir()
	output := filepath.Join(dir, "snapshot")
	var stderr bytes.Buffer
	code := Run(context.Background(),
		[]string{"import", "--input", "-", "--output", output},
		strings.NewReader("{\"id\":\"one\"}\n"), unavailableOutput{}, &stderr)
	if code != 1 || !strings.Contains(stderr.String(), "snapshot published") {
		t.Fatalf("exit=%d stderr=%q", code, stderr.String())
	}
	m, err := snapshot.Verify(context.Background(), output)
	if err != nil || m.Stats.RecordsWritten != 1 {
		t.Fatalf("published snapshot changed: manifest=%+v err=%v", m, err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 || entries[0].Name() != "snapshot" {
		t.Fatalf("unexpected staging or lock leftovers: entries=%v err=%v", entries, err)
	}
}
