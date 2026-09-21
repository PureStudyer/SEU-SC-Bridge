package bridge

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

type tinyReader struct {
	r io.Reader
	n int
}

func (r tinyReader) Read(p []byte) (int, error) {
	if len(p) > r.n {
		p = p[:r.n]
	}
	return r.r.Read(p)
}
func TestExecSplitMarkers(t *testing.T) {
	begin, end := []byte("\x1eBEGIN\x1f"), []byte("\x1eEND:")
	for split := 1; split <= 25; split++ {
		var out bytes.Buffer
		input := "banner and echoed script\r\n" + string(begin) + "中文\r\n\x1b[31mred\x1b[0m\n" + string(end) + "23\x1fignored"
		code, e := ReadExec(&out, tinyReader{strings.NewReader(input), split}, begin, end)
		if e != nil || code != 23 || out.String() != "中文\r\n\x1b[31mred\x1b[0m\n" {
			t.Fatalf("split=%d code=%d out=%q error=%v", split, code, out.String(), e)
		}
	}
}
func TestExecMissingMarker(t *testing.T) {
	var out bytes.Buffer
	if code, e := ReadExec(&out, strings.NewReader("beginhello"), []byte("begin"), []byte("end:")); e == nil || code != 255 {
		t.Fatal(code, e)
	}
}
