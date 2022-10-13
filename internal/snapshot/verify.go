package snapshot

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/Bispulm/dockline/internal/model"
	"github.com/Bispulm/dockline/internal/ndjson"
)

// Verify checks snapshot checksums, syntax, unique IDs, and count invariants.
// Digests detect accidental corruption, not maliciously rewritten manifests.
func Verify(ctx context.Context, dir string) (model.Manifest, error) {
	var m model.Manifest
	f, err := os.Open(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return m, err
	}
	data, readErr := io.ReadAll(io.LimitReader(f, (64<<10)+1))
	closeErr := f.Close()
	if err = errors.Join(readErr, closeErr); err != nil {
		return m, err
	}
	if len(data) > 64<<10 {
		return m, errors.New("manifest is too large")
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err = d.Decode(&m); err != nil {
		return m, fmt.Errorf("decode manifest: %w", err)
	}
	var extra any
	if err = d.Decode(&extra); err != io.EOF {
		return m, errors.New("trailing data in manifest")
	}
	if m.Format != model.Format || m.ToolVersion == "" {
		return m, errors.New("unsupported or incomplete manifest")
	}
	if err = m.Options.Validate(); err != nil {
		return m, err
	}
	s := m.Stats
	for _, n := range []int64{s.LinesRead, s.BlankLines, s.RecordsWritten, s.DuplicateRecords, s.RejectedRecords, s.SourceBytes, s.RecordsBytes, s.RejectedBytes} {
		if n < 0 {
			return m, errors.New("negative manifest count")
		}
	}
	// Subtract rather than sum untrusted counts, avoiding overflow.
	remaining := s.LinesRead
	for _, n := range []int64{s.BlankLines, s.RecordsWritten, s.DuplicateRecords, s.RejectedRecords} {
		if n > remaining {
			return m, errors.New("inconsistent line counts")
		}
		remaining -= n
	}
	if remaining != 0 || (m.Options.InvalidPolicy == "fail" && s.RejectedRecords != 0) || (m.Options.DuplicatePolicy == "fail" && s.DuplicateRecords != 0) {
		return m, errors.New("inconsistent manifest policy or counts")
	}
	for _, digest := range []string{m.Digests.Source, m.Digests.Records, m.Digests.Rejected} {
		raw, err := hex.DecodeString(digest)
		if err != nil || len(raw) != sha256.Size {
			return m, errors.New("invalid SHA-256 digest")
		}
	}
	seen := make(map[string]struct{})
	checkRecord := func(line ndjson.Line) error {
		r, err := model.ParseRecord(line.Data)
		if err != nil {
			return err
		}
		if _, exists := seen[r.ID]; exists {
			return errors.New("duplicate id in snapshot")
		}
		seen[r.ID] = struct{}{}
		return nil
	}
	if err = verifyFile(ctx, filepath.Join(dir, "records.ndjson"), m.Options.MaxRecordBytes, s.RecordsWritten, s.RecordsBytes, m.Digests.Records, checkRecord); err != nil {
		return m, err
	}
	var previous int64
	checkRejection := func(line ndjson.Line) error {
		var r model.Rejection
		d := json.NewDecoder(bytes.NewReader(line.Data))
		d.DisallowUnknownFields()
		if err := d.Decode(&r); err != nil {
			return err
		}
		var extra any
		if err := d.Decode(&extra); err != io.EOF {
			return errors.New("trailing rejection data")
		}
		if r.Line <= previous || r.Line > s.LinesRead || r.Reason == "" {
			return errors.New("invalid rejection line number or reason")
		}
		previous = r.Line
		return nil
	}
	if err = verifyFile(ctx, filepath.Join(dir, "rejected.ndjson"), 4096, s.RejectedRecords, s.RejectedBytes, m.Digests.Rejected, checkRejection); err != nil {
		return m, err
	}
	return m, nil
}

type countingReader struct {
	r io.Reader
	n int64
}

func (r *countingReader) Read(p []byte) (int, error) {
	n, err := r.r.Read(p)
	r.n += int64(n)
	return n, err
}

func verifyFile(ctx context.Context, path string, maxBytes int, count, size int64, digest string, check func(ndjson.Line) error) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	r := &countingReader{r: io.TeeReader(f, h)}
	lines := ndjson.NewReader(r, maxBytes)
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		line, err := lines.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if line.Err != nil {
			return line.Err
		}
		if err = check(line); err != nil {
			return fmt.Errorf("%s line %d: %w", filepath.Base(path), line.Number, err)
		}
		total++
	}
	if total != count || r.n != size || hex.EncodeToString(h.Sum(nil)) != digest {
		return fmt.Errorf("%s checksum, byte count, or record count mismatch", filepath.Base(path))
	}
	return nil
}
