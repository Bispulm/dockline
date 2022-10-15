package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func run(t *testing.T, ctx context.Context, args []string, input string, want int) string {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := Run(ctx, args, strings.NewReader(input), &stdout, &stderr)
	if code != want {
		t.Fatalf("%v exit %d, want %d; stdout=%s stderr=%s", args, code, want, stdout.String(), stderr.String())
	}
	return stdout.String()
}

func TestImportAndVerifyCLI(t *testing.T) {
	dir := t.TempDir()
	output := filepath.Join(dir, "dataset")
	input := "{\"id\":\"a\"}\n{\"id\":\"b\"}\n"
	ctx := context.Background()
	summary := run(t, ctx, []string{"import", "--input", "-", "--output", output}, input, 0)
	if !json.Valid([]byte(summary)) {
		t.Fatal("summary is not JSON")
	}
	source := filepath.Join(dir, "source.ndjson")
	if err := os.WriteFile(source, []byte(input), 0600); err != nil {
		t.Fatal(err)
	}
	verified := run(t, ctx, []string{"verify", "--snapshot", output, "--input", source}, "", 0)
	if !strings.Contains(verified, `"source_checked":true`) {
		t.Fatal(verified)
	}
	run(t, ctx, []string{"import", "--input", "-", "--output", output}, input, 1)
	if err := os.WriteFile(source, []byte("different"), 0600); err != nil {
		t.Fatal(err)
	}
	run(t, ctx, []string{"verify", "--snapshot", output, "--input", source}, "", 1)
}

func TestFailureLeavesNoPublishedOutput(t *testing.T) {
	dir := t.TempDir()
	output := filepath.Join(dir, "dataset")
	run(t, context.Background(), []string{"import", "--input", "-", "--output", output}, "{\"id\":\"a\"}\nbad\n", 1)
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 0 {
		t.Fatalf("leftover data: %v %v", entries, err)
	}
}

func TestUsageHelpAndCancellation(t *testing.T) {
	ctx := context.Background()
	for _, args := range [][]string{{"import"}, {"verify"}, {"import", "--bad"}, {"unknown"}, {"version", "extra"}} {
		run(t, ctx, args, "", 2)
	}
	for _, args := range [][]string{nil, {"--help"}, {"import", "--help"}, {"verify", "--help"}, {"version"}} {
		run(t, ctx, args, "", 0)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	run(t, cancelled, []string{"import", "--input", "-", "--output", filepath.Join(t.TempDir(), "out")}, "", 130)
}
