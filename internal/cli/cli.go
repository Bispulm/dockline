// Package cli exposes a testable command boundary; only main calls os.Exit.
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/Bispulm/dockline/internal/engine"
	"github.com/Bispulm/dockline/internal/model"
	"github.com/Bispulm/dockline/internal/snapshot"
)

func Run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		fmt.Fprintln(stdout, "dockline import --input FILE|- --output DIR [--invalid fail|skip] [--duplicates fail|skip]\ndockline verify --snapshot DIR [--input FILE]\ndockline version")
		return 0
	}
	var err error
	switch args[0] {
	case "version":
		if len(args) != 1 {
			fmt.Fprintln(stderr, "version accepts no arguments")
			return 2
		}
		fmt.Fprintln(stdout, model.Version)
		return 0
	case "import":
		err = importCommand(ctx, args[1:], stdin, stdout, stderr)
	case "verify":
		err = verifyCommand(ctx, args[1:], stdout, stderr)
	default:
		fmt.Fprintln(stderr, "unknown command:", args[0])
		return 2
	}
	if errors.Is(err, flag.ErrHelp) {
		return 0
	}
	if err == nil {
		return 0
	}
	fmt.Fprintln(stderr, "dockline:", err)
	var u usageError
	if errors.As(err, &u) {
		return 2
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return 130
	}
	return 1
}

type usageError struct{ error }

func (e usageError) Unwrap() error { return e.error }

func importCommand(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) (result error) {
	fs := flag.NewFlagSet("import", flag.ContinueOnError)
	fs.SetOutput(stderr)
	input := fs.String("input", "", "NDJSON input path, or - for stdin")
	output := fs.String("output", "", "new snapshot directory; must not exist")
	invalid := fs.String("invalid", "fail", "invalid record policy: fail or skip")
	duplicates := fs.String("duplicates", "fail", "duplicate id policy: fail or skip (first wins)")
	max := fs.Int("max-record-bytes", model.DefaultMaxRecordBytes, "maximum bytes per line excluding LF/CRLF")
	timeout := fs.Duration("timeout", 0, "optional whole-command deadline, for example 30s")
	if err := fs.Parse(args); err != nil {
		return usageError{err}
	}
	if *input == "" || *output == "" || fs.NArg() != 0 || *timeout < 0 {
		return usageError{errors.New("import requires --input and --output, no positional arguments, and a nonnegative timeout")}
	}
	opts := model.Options{InvalidPolicy: *invalid, DuplicatePolicy: *duplicates, MaxRecordBytes: *max}
	if err := opts.Validate(); err != nil {
		return usageError{err}
	}
	if *timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, *timeout)
		defer cancel()
	}
	var r io.Reader = stdin
	if *input != "-" {
		f, err := os.Open(*input)
		if err != nil {
			return err
		}
		defer f.Close()
		r = f
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	w, err := snapshot.Begin(*output)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, w.Abort()) }()
	m, err := engine.Import(ctx, r, w.Records, w.Rejected, opts)
	if err != nil {
		return err
	}
	if err = w.Publish(ctx, m); err != nil {
		return err
	}
	if err = w.Abort(); err != nil {
		return fmt.Errorf("snapshot published but lock cleanup failed: %w", err)
	}
	if err = json.NewEncoder(stdout).Encode(m); err != nil {
		return fmt.Errorf("snapshot published but summary could not be written: %w", err)
	}
	return nil
}

func verifyCommand(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dir := fs.String("snapshot", "", "snapshot directory")
	input := fs.String("input", "", "optional original file to verify against source fingerprint")
	if err := fs.Parse(args); err != nil {
		return usageError{err}
	}
	if *dir == "" || fs.NArg() != 0 {
		return usageError{errors.New("verify requires --snapshot and no positional arguments")}
	}
	m, err := snapshot.Verify(ctx, *dir)
	if err != nil {
		return err
	}
	if *input != "" {
		f, err := os.Open(*input)
		if err != nil {
			return err
		}
		digest, size, readErr := engine.Digest(ctx, f)
		if err := errors.Join(readErr, f.Close()); err != nil {
			return err
		}
		if digest != m.Digests.Source || size != m.Stats.SourceBytes {
			return errors.New("source fingerprint or size mismatch")
		}
	}
	return json.NewEncoder(stdout).Encode(struct {
		OK            bool        `json:"ok"`
		SourceChecked bool        `json:"source_checked"`
		Stats         model.Stats `json:"stats"`
	}{true, *input != "", m.Stats})
}
