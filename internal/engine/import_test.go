package engine

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/Bispulm/dockline/internal/model"
)

func fingerprint(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func TestImportMixedInput(t *testing.T) {
	input := "{\"id\":\"a\",\"n\":9007199254740993}\r\n\nnot-json\n{\"id\":\"a\",\"n\":0}\n{ \"id\" : \"b\" }"
	var out, rejected bytes.Buffer
	opts := model.Options{InvalidPolicy: "skip", DuplicatePolicy: "skip", MaxRecordBytes: 1024}
	m, err := Import(context.Background(), strings.NewReader(input), &out, &rejected, opts)
	if err != nil {
		t.Fatal(err)
	}
	want := "{\"id\":\"a\",\"n\":9007199254740993}\n{\"id\":\"b\"}\n"
	if out.String() != want {
		t.Fatalf("output differs: %s", out.String())
	}
	s := m.Stats
	if s.LinesRead != 5 || s.BlankLines != 1 || s.RecordsWritten != 2 || s.DuplicateRecords != 1 || s.RejectedRecords != 1 {
		t.Fatalf("unexpected stats: %+v", s)
	}
	if s.SourceBytes != int64(len(input)) || s.RecordsBytes != int64(len(want)) || s.RejectedBytes != int64(rejected.Len()) {
		t.Fatalf("unexpected byte counts: %+v", s)
	}
	if m.Digests.Source != fingerprint(input) || m.Digests.Records != fingerprint(want) || m.Digests.Rejected != fingerprint(rejected.String()) {
		t.Fatal("fingerprint mismatch")
	}
	if rejected.String() != "{\"line\":3,\"reason\":\"record must be a JSON object\"}\n" {
		t.Fatalf("unexpected rejection: %s", rejected.String())
	}
}

func TestFailurePolicies(t *testing.T) {
	for _, input := range []string{"{\"id\":\"a\"}\nbroken\n", "{\"id\":\"a\"}\n{\"id\":\"a\"}\n"} {
		if _, err := Import(context.Background(), strings.NewReader(input), io.Discard, io.Discard, model.DefaultOptions()); err == nil {
			t.Fatalf("expected failure for %q", input)
		}
	}
}

func TestOversizedRecordCanBeSkipped(t *testing.T) {
	opts := model.Options{InvalidPolicy: "skip", DuplicatePolicy: "fail", MaxRecordBytes: 16}
	var out bytes.Buffer
	m, err := Import(context.Background(), strings.NewReader(strings.Repeat("x", 100000)+"\n{\"id\":\"good\"}\n"), &out, io.Discard, opts)
	if err != nil || m.Stats.RejectedRecords != 1 || m.Stats.RecordsWritten != 1 || out.String() != "{\"id\":\"good\"}\n" {
		t.Fatalf("recovery failed: %+v %v %s", m, err, out.String())
	}
}

type shortWriter struct{}

func (shortWriter) Write(p []byte) (int, error) { return len(p) / 2, nil }

func TestWriteFailureIsNotSuccess(t *testing.T) {
	_, err := Import(context.Background(), strings.NewReader("{\"id\":\"a\"}\n"), shortWriter{}, io.Discard, model.DefaultOptions())
	if !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("got %v", err)
	}
	opts := model.DefaultOptions()
	opts.InvalidPolicy = "skip"
	_, err = Import(context.Background(), strings.NewReader("bad\n"), io.Discard, shortWriter{}, opts)
	if !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("rejection write: %v", err)
	}
}

type cancellingWriter struct{ cancel context.CancelFunc }

func (w cancellingWriter) Write(p []byte) (int, error) {
	w.cancel()
	return len(p), nil
}

func TestCancellationDuringImport(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, err := Import(ctx, strings.NewReader("{\"id\":\"a\"}\n{\"id\":\"b\"}\n"), cancellingWriter{cancel}, io.Discard, model.DefaultOptions())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v", err)
	}
}

func TestInvalidOptionsDoNotWrite(t *testing.T) {
	var out bytes.Buffer
	if _, err := Import(context.Background(), strings.NewReader("{\"id\":\"a\"}"), &out, io.Discard, model.Options{}); err == nil || out.Len() != 0 {
		t.Fatal("invalid options must fail before writing")
	}
}

func TestDigest(t *testing.T) {
	h, size, err := Digest(context.Background(), strings.NewReader("abc"))
	if err != nil || size != 3 || h != "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad" {
		t.Fatalf("%s %d %v", h, size, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err = Digest(ctx, strings.NewReader("abc")); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func BenchmarkImport(b *testing.B) {
	var input strings.Builder
	for i := 0; i < 10000; i++ {
		fmtID := []byte{byte('a' + i/676), byte('a' + (i/26)%26), byte('a' + i%26)}
		input.WriteString("{\"id\":\"" + string(fmtID) + "\",\"payload\":\"example\"}\n")
	}
	data := input.String()
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Import(context.Background(), strings.NewReader(data), io.Discard, io.Discard, model.DefaultOptions()); err != nil {
			b.Fatal(err)
		}
	}
}
