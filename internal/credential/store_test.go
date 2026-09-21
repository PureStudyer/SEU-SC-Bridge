package credential

import (
	"fmt"
	"os"
	"runtime"
	"testing"
	"time"
)

func TestEmptyDelete(t *testing.T) {
	if e := (System{}).Delete(""); e != nil {
		t.Fatal(e)
	}
}
func TestNativeCredentialRoundTrip(t *testing.T) {
	if os.Getenv("SEUSC_CREDENTIAL_TEST") != "1" {
		t.Skip("opt-in native credential-store integration test")
	}
	if runtime.GOOS != "windows" && runtime.GOOS != "darwin" {
		t.Skip("native desktop platforms only")
	}
	name := fmt.Sprintf("seusc-test-%d", time.Now().UnixNano())
	s := System{}
	defer s.Delete(name)
	if e := s.Set(name, "synthetic-non-user-test-secret"); e != nil {
		t.Fatal(e)
	}
	value, e := s.Get(name)
	if e != nil || value != "synthetic-non-user-test-secret" {
		t.Fatal("credential roundtrip failed")
	}
	if e = s.Delete(name); e != nil {
		t.Fatal(e)
	}
	value, e = s.Get(name)
	if e != nil || value != "" {
		t.Fatal("credential deletion failed")
	}
}
