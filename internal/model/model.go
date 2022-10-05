// Package model defines the versioned import contract and snapshot format.
package model

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

const Version = "0.1.0"
const Format = "dockline.snapshot.v1"
const DefaultMaxRecordBytes = 1 << 20
const MaxRecordBytes = 64 << 20

type Options struct {
	InvalidPolicy   string `json:"invalid_policy"`
	DuplicatePolicy string `json:"duplicate_policy"`
	MaxRecordBytes  int    `json:"max_record_bytes"`
}

func DefaultOptions() Options {
	return Options{"fail", "fail", DefaultMaxRecordBytes}
}

func (o Options) Validate() error {
	if o.InvalidPolicy != "fail" && o.InvalidPolicy != "skip" {
		return errors.New("invalid policy must be fail or skip")
	}
	if o.DuplicatePolicy != "fail" && o.DuplicatePolicy != "skip" {
		return errors.New("duplicate policy must be fail or skip")
	}
	if o.MaxRecordBytes < 1 || o.MaxRecordBytes > MaxRecordBytes {
		return fmt.Errorf("max record bytes must be between 1 and %d", MaxRecordBytes)
	}
	return nil
}

type Stats struct {
	LinesRead        int64 `json:"lines_read"`
	BlankLines       int64 `json:"blank_lines"`
	RecordsWritten   int64 `json:"records_written"`
	DuplicateRecords int64 `json:"duplicate_records"`
	RejectedRecords  int64 `json:"rejected_records"`
	SourceBytes      int64 `json:"source_bytes"`
	RecordsBytes     int64 `json:"records_bytes"`
	RejectedBytes    int64 `json:"rejected_bytes"`
}

type Digests struct {
	Source   string `json:"source_sha256"`
	Records  string `json:"records_sha256"`
	Rejected string `json:"rejected_sha256"`
}

type Manifest struct {
	Format      string  `json:"format"`
	ToolVersion string  `json:"tool_version"`
	Options     Options `json:"options"`
	Stats       Stats   `json:"stats"`
	Digests     Digests `json:"digests"`
}

type Rejection struct {
	Line   int64  `json:"line"`
	Reason string `json:"reason"`
}

type Record struct {
	ID   string
	JSON []byte
}

// ParseRecord preserves number literals and field order. Top-level keys must
// be unique; id is an opaque, nonblank UTF-8 string of at most 256 bytes.
func ParseRecord(raw []byte) (Record, error) {
	if !utf8.Valid(raw) {
		return Record{}, errors.New("record is not valid UTF-8")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	t, err := d.Token()
	if err != nil || t != json.Delim('{') {
		return Record{}, errors.New("record must be a JSON object")
	}
	seen := make(map[string]struct{})
	var id string
	for d.More() {
		t, err = d.Token()
		if err != nil {
			return Record{}, errors.New("invalid JSON object")
		}
		key, ok := t.(string)
		if !ok {
			return Record{}, errors.New("invalid JSON object key")
		}
		if _, ok := seen[key]; ok {
			return Record{}, errors.New("duplicate top-level JSON key")
		}
		seen[key] = struct{}{}
		var value json.RawMessage
		if err := d.Decode(&value); err != nil {
			return Record{}, errors.New("invalid JSON value")
		}
		if key == "id" {
			if len(value) == 0 || value[0] != '"' || json.Unmarshal(value, &id) != nil {
				return Record{}, errors.New("id must be a string")
			}
		}
	}
	if _, err = d.Token(); err != nil {
		return Record{}, errors.New("unterminated JSON object")
	}
	var extra any
	if err = d.Decode(&extra); err != io.EOF {
		return Record{}, errors.New("trailing data after JSON object")
	}
	if strings.TrimSpace(id) == "" || len(id) > 256 {
		return Record{}, errors.New("id must be nonblank and at most 256 bytes")
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, raw); err != nil {
		return Record{}, errors.New("invalid JSON record")
	}
	return Record{ID: id, JSON: compact.Bytes()}, nil
}
