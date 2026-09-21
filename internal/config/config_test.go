package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	d := t.TempDir()
	c := Default()
	c.Nodes["login02"] = 9
	c.SEU.DefaultNode = "login02"
	if e := Save(d, c); e != nil {
		t.Fatal(e)
	}
	got, e := Load(d)
	if e != nil || got.Nodes[got.SEU.DefaultNode] != 9 {
		t.Fatal(got, e)
	}
	b, _ := os.ReadFile(filepath.Join(d, "config.json"))
	if strings.Contains(string(b), "Bearer") {
		t.Fatal("secret in config")
	}
}
func TestRejectInsecureConfig(t *testing.T) {
	for _, modify := range []func(*Config){func(c *Config) { c.SSH.Listen = "0.0.0.0" }, func(c *Config) { c.SSH.Alias = "seusc\nProxyCommand bad" }, func(c *Config) { c.SEU.BaseURL = "http://sc.seu.edu.cn" }, func(c *Config) { c.SEU.DefaultNode = "missing" }, func(c *Config) { c.Nodes["bad\nname"] = 7 }} {
		c := Default()
		modify(&c)
		if c.Validate() == nil {
			t.Fatal("accepted invalid config")
		}
	}
}
func TestManagedBlock(t *testing.T) {
	original := "# personal\r\nHost example\r\n  HostName example.org\r\n"
	block := begin + "\nHost seusc\n" + end + "\n"
	got, e := ManagedBlock(original, block)
	if e != nil || !strings.HasSuffix(got, original) {
		t.Fatal(got, e)
	}
	twice, e := ManagedBlock(got, block)
	if e != nil || twice != got {
		t.Fatal("not idempotent", e)
	}
	for _, bad := range []string{begin, end, begin + end + begin + end, end + begin} {
		if _, e := ManagedBlock(bad, block); e == nil {
			t.Fatal("accepted malformed markers")
		}
	}
}
