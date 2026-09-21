package browser

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExplicitBrowser(t *testing.T) {
	f := filepath.Join(t.TempDir(), "browser")
	if e := os.WriteFile(f, []byte("placeholder"), 0700); e != nil {
		t.Fatal(e)
	}
	if p, e := Find(f); e != nil || p != f {
		t.Fatal(p, e)
	}
	if _, e := Find(f + "missing"); e == nil {
		t.Fatal("missing browser accepted")
	}
}
