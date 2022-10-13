// Package snapshot publishes and verifies complete local snapshots.
package snapshot

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Bispulm/dockline/internal/model"
)

var ErrExists = errors.New("output already exists")
var ErrBusy = errors.New("output is locked; another import or an interrupted run owns it")

// Writer is single-owner. Concurrent imports use separate Writers, serialized
// by an atomic mkdir lock for the same destination. No automatic stale-lock
// stealing is attempted in the baseline.
type Writer struct {
	Records  *os.File
	Rejected *os.File
	output   string
	parent   string
	stage    string
	lock     string
	closed   bool
}

func Begin(output string) (*Writer, error) {
	path, err := filepath.Abs(output)
	if err != nil {
		return nil, err
	}
	if _, err := os.Lstat(path); err == nil {
		return nil, ErrExists
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	parent := filepath.Dir(path)
	if err := os.MkdirAll(parent, 0755); err != nil {
		return nil, err
	}
	sum := sha256.Sum256([]byte(filepath.Base(path)))
	lock := filepath.Join(parent, ".dockline-lock-"+hex.EncodeToString(sum[:]))
	if err := os.Mkdir(lock, 0700); err != nil {
		if errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf("%w: %s", ErrBusy, lock)
		}
		return nil, err
	}
	w := &Writer{output: path, parent: parent, lock: lock}
	success := false
	defer func() {
		if !success {
			_ = w.Abort()
		}
	}()
	if _, err := os.Lstat(path); err == nil {
		return nil, ErrExists
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	w.stage, err = os.MkdirTemp(parent, ".dockline-stage-")
	if err != nil {
		return nil, err
	}
	w.Records, err = os.OpenFile(filepath.Join(w.stage, "records.ndjson"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return nil, err
	}
	w.Rejected, err = os.OpenFile(filepath.Join(w.stage, "rejected.ndjson"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return nil, err
	}
	success = true
	return w, nil
}

func syncClose(f *os.File) error {
	return errors.Join(f.Sync(), f.Close())
}

func syncDir(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	return syncClose(f)
}

// Publish syncs files and the staging directory, renames within the same
// parent, then syncs the parent. Supported contract: Linux local filesystems
// with directory fsync and atomic same-filesystem rename. If the final parent
// sync fails, output may already exist; the caller must verify before retrying.
// Filesystem mutation by non-cooperating programs is outside this contract.
func (w *Writer) Publish(ctx context.Context, manifest model.Manifest) error {
	if w.closed {
		return errors.New("snapshot writer is closed")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	w.closed = true
	err := errors.Join(syncClose(w.Records), syncClose(w.Rejected))
	w.Records, w.Rejected = nil, nil
	if err != nil {
		return fmt.Errorf("sync data: %w", err)
	}
	f, err := os.OpenFile(filepath.Join(w.stage, "manifest.json"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err = enc.Encode(manifest); err != nil {
		_ = f.Close()
		return err
	}
	if err = syncClose(f); err != nil {
		return err
	}
	if err = syncDir(w.stage); err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	if _, err := os.Lstat(w.output); err == nil {
		return ErrExists
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err = os.Rename(w.stage, w.output); err != nil {
		return err
	}
	w.stage = "" // Abort must never remove a published snapshot.
	w.closed = true
	if err = syncDir(w.parent); err != nil {
		return fmt.Errorf("snapshot was renamed but durability is uncertain; verify %s before retrying: %w", w.output, err)
	}
	return nil
}

// Abort is also required after Publish to release the owned lock. It never
// removes a published snapshot and is safe to call repeatedly.
func (w *Writer) Abort() error {
	var errs []error
	if w.Records != nil {
		errs = append(errs, w.Records.Close())
		w.Records = nil
	}
	if w.Rejected != nil {
		errs = append(errs, w.Rejected.Close())
		w.Rejected = nil
	}
	if w.stage != "" {
		errs = append(errs, os.RemoveAll(w.stage))
		w.stage = ""
	}
	if w.lock != "" {
		err := os.Remove(w.lock)
		if !errors.Is(err, os.ErrNotExist) {
			errs = append(errs, err)
		}
		w.lock = ""
	}
	w.closed = true
	return errors.Join(errs...)
}
