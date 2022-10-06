package ndjson

import (
	"errors"
	"io"
	"strings"
	"testing"
)

func TestPhysicalLinesAndBoundaries(t *testing.T) {
	r := NewReader(strings.NewReader("abc\r\n\n1234\nxy"), 3)
	for i, want := range []string{"abc", "", "too-long", "xy"} {
		line, err := r.Next()
		if err != nil || line.Number != int64(i+1) {
			t.Fatalf("line %d: %+v %v", i, line, err)
		}
		if want == "too-long" {
			if !errors.Is(line.Err, ErrTooLarge) {
				t.Fatal("missing line limit error")
			}
		} else if line.Err != nil || string(line.Data) != want {
			t.Fatalf("got %+v, want %q", line, want)
		}
	}
	if _, err := r.Next(); err != io.EOF {
		t.Fatalf("expected EOF, got %v", err)
	}
}

func TestOversizedLineDrainsWithoutLosingNext(t *testing.T) {
	r := NewReader(strings.NewReader(strings.Repeat("x", 200000)+"\nokay\n"), 4)
	first, err := r.Next()
	if err != nil || first.Err != ErrTooLarge || len(first.Data) != 0 {
		t.Fatalf("unexpected oversized line: %+v %v", first, err)
	}
	second, err := r.Next()
	if err != nil || second.Err != nil || string(second.Data) != "okay" || second.Number != 2 {
		t.Fatalf("lost following line: %+v %v", second, err)
	}
}

type brokenReader struct{}

func (brokenReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

func TestIOErrorIsFatal(t *testing.T) {
	if _, err := NewReader(brokenReader{}, 10).Next(); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("lost I/O error: %v", err)
	}
}

func TestEmptyInputAndExactLimits(t *testing.T) {
	for _, suffix := range []string{"", "\n", "\r\n"} {
		r := NewReader(strings.NewReader("abc"+suffix), 3)
		line, err := r.Next()
		if err != nil || line.Err != nil || string(line.Data) != "abc" {
			t.Fatalf("suffix %q: %+v %v", suffix, line, err)
		}
	}
	if _, err := NewReader(strings.NewReader(""), 1).Next(); err != io.EOF {
		t.Fatalf("empty input: %v", err)
	}
}
