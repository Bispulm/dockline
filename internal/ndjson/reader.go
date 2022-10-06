// Package ndjson implements bounded-memory physical-line framing.
package ndjson

import (
	"bufio"
	"errors"
	"io"
)

var ErrTooLarge = errors.New("record exceeds max record bytes")

type Line struct {
	Number int64
	Data   []byte
	Err    error // A framing error on this line; the next line remains readable.
}

type Reader struct {
	r      *bufio.Reader
	max    int
	number int64
}

func NewReader(r io.Reader, maxBytes int) *Reader {
	return &Reader{r: bufio.NewReaderSize(r, 4096), max: maxBytes}
}

// Next accepts LF, CRLF, and a final line without a newline. It drains an
// oversized line before returning ErrTooLarge, without retaining its payload.
func (r *Reader) Next() (Line, error) {
	var data []byte
	var size int64
	oversize := false
	for {
		part, err := r.r.ReadSlice('\n')
		size += int64(len(part))
		if !oversize {
			if size > int64(r.max)+2 {
				oversize = true
				data = nil
			} else {
				data = append(data, part...)
			}
		}
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		if err != nil && !errors.Is(err, io.EOF) {
			return Line{}, err
		}
		if size == 0 && errors.Is(err, io.EOF) {
			return Line{}, io.EOF
		}
		r.number++
		if !oversize {
			if len(data) > 0 && data[len(data)-1] == '\n' {
				data = data[:len(data)-1]
				if len(data) > 0 && data[len(data)-1] == '\r' {
					data = data[:len(data)-1]
				}
			}
			oversize = len(data) > r.max
		}
		if oversize {
			return Line{Number: r.number, Err: ErrTooLarge}, nil
		}
		return Line{Number: r.number, Data: data}, nil
	}
}
