// Package engine processes a single immutable input stream. It does not own
// publication or recovery of the output snapshot.
package engine

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"io"

	"github.com/Bispulm/dockline/internal/model"
	"github.com/Bispulm/dockline/internal/ndjson"
)

type measuredWriter struct {
	w io.Writer
	h hash.Hash
	n int64
}

func (w *measuredWriter) Write(p []byte) (int, error) {
	n, err := w.w.Write(p)
	if n > 0 {
		_, _ = w.h.Write(p[:n])
		w.n += int64(n)
	}
	if n != len(p) && err == nil {
		err = io.ErrShortWrite
	}
	return n, err
}

type measuredReader struct {
	r io.Reader
	h hash.Hash
	n int64
}

func (r *measuredReader) Read(p []byte) (int, error) {
	n, err := r.r.Read(p)
	if n > 0 {
		_, _ = r.h.Write(p[:n])
		r.n += int64(n)
	}
	return n, err
}

// Import checks context between records. It cannot interrupt a blocked Read
// on an arbitrary caller-owned io.Reader. Output is provisional until the
// caller publishes it. Duplicate policy skip retains the first valid record.
func Import(ctx context.Context, input io.Reader, records, rejected io.Writer, opts model.Options) (model.Manifest, error) {
	if err := opts.Validate(); err != nil {
		return model.Manifest{}, err
	}
	m := model.Manifest{Format: model.Format, ToolVersion: model.Version, Options: opts}
	source := &measuredReader{r: input, h: sha256.New()}
	out := &measuredWriter{w: records, h: sha256.New()}
	bad := &measuredWriter{w: rejected, h: sha256.New()}
	lines := ndjson.NewReader(source, opts.MaxRecordBytes)
	seen := make(map[string]struct{})
	for {
		if err := ctx.Err(); err != nil {
			return m, err
		}
		line, err := lines.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return m, fmt.Errorf("read input: %w", err)
		}
		m.Stats.LinesRead++
		if line.Err == nil && len(bytes.TrimSpace(line.Data)) == 0 {
			m.Stats.BlankLines++
			continue
		}
		var rec model.Record
		err = line.Err
		if err == nil {
			rec, err = model.ParseRecord(line.Data)
		}
		if err != nil {
			if opts.InvalidPolicy == "fail" {
				return m, fmt.Errorf("line %d: %w", line.Number, err)
			}
			if err = json.NewEncoder(bad).Encode(model.Rejection{Line: line.Number, Reason: err.Error()}); err != nil {
				return m, fmt.Errorf("write rejection: %w", err)
			}
			m.Stats.RejectedRecords++
			continue
		}
		if _, exists := seen[rec.ID]; exists {
			if opts.DuplicatePolicy == "fail" {
				return m, fmt.Errorf("line %d: duplicate id", line.Number)
			}
			m.Stats.DuplicateRecords++
			continue
		}
		seen[rec.ID] = struct{}{}
		if _, err = out.Write(append(rec.JSON, '\n')); err != nil {
			return m, fmt.Errorf("write record: %w", err)
		}
		m.Stats.RecordsWritten++
	}
	if err := ctx.Err(); err != nil {
		return m, err
	}
	m.Stats.SourceBytes = source.n
	m.Stats.RecordsBytes = out.n
	m.Stats.RejectedBytes = bad.n
	m.Digests = model.Digests{Source: hex.EncodeToString(source.h.Sum(nil)), Records: hex.EncodeToString(out.h.Sum(nil)), Rejected: hex.EncodeToString(bad.h.Sum(nil))}
	return m, nil
}

// Digest computes a complete byte-level source fingerprint.
func Digest(ctx context.Context, r io.Reader) (string, int64, error) {
	h := sha256.New()
	buf := make([]byte, 32*1024)
	var size int64
	for {
		if err := ctx.Err(); err != nil {
			return "", size, err
		}
		n, err := r.Read(buf)
		if n > 0 {
			_, _ = h.Write(buf[:n])
			size += int64(n)
		}
		if err == io.EOF {
			return hex.EncodeToString(h.Sum(nil)), size, nil
		}
		if err != nil {
			return "", size, err
		}
	}
}
