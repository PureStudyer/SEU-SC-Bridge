package update

import "testing"

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.0.0", "1.0.1", -1},
		{"1.2.0", "1.1.9", 1},
		{"2.0.0", "2.0.0", 0},
		{"1.10.0", "1.9.99", 1},
	}
	for _, tc := range cases {
		got, ok := compareVersions(tc.a, tc.b)
		if !ok || got != tc.want {
			t.Errorf("compareVersions(%q, %q) = %d, %v; want %d, true", tc.a, tc.b, got, ok, tc.want)
		}
	}
	if _, ok := compareVersions("v1.0", "1.0.0"); ok {
		t.Fatal("accepted incomplete version")
	}
}

func TestChecksumFor(t *testing.T) {
	manifest := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa  app.exe\n" +
		"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb  notes.txt\n"
	got, ok := checksumFor(manifest, "app.exe")
	if !ok || got[0] != 'a' {
		t.Fatalf("checksumFor returned %q, %v", got, ok)
	}
	if _, ok := checksumFor(manifest, "missing.exe"); ok {
		t.Fatal("accepted a missing checksum")
	}
	if _, ok := checksumFor("bad app.exe\n", "app.exe"); ok {
		t.Fatal("accepted a malformed checksum")
	}
}

func TestAssetForWindowsArchitectures(t *testing.T) {
	r := release{TagName: "v1.0.1"}
	r.Assets = append(r.Assets,
		struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
		}{Name: "SEU-SC-Bridge-Setup-x64.exe"},
		struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
		}{Name: "SEU-SC-Bridge-Setup-arm64.exe"},
	)
	if got := assetFor(r, "windows", "amd64"); got == nil || got.Name != "SEU-SC-Bridge-Setup-x64.exe" {
		t.Fatalf("amd64 asset = %#v", got)
	}
	if got := assetFor(r, "windows", "arm64"); got == nil || got.Name != "SEU-SC-Bridge-Setup-arm64.exe" {
		t.Fatalf("arm64 asset = %#v", got)
	}
	if got := assetFor(r, "linux", "amd64"); got != nil {
		t.Fatalf("unexpected Linux installer: %#v", got)
	}
}

func TestValidateReleaseURL(t *testing.T) {
	r := release{TagName: "v1.0.1"}
	if err := validateReleaseURL(r, "https://github.com/PureStudyer/SEU-SC-Bridge/releases/download/v1.0.1/SEU-SC-Bridge-Setup-x64.exe"); err != nil {
		t.Fatal(err)
	}
	for _, u := range []string{
		"http://github.com/PureStudyer/SEU-SC-Bridge/releases/download/v1.0.1/file.exe",
		"https://evil.example/PureStudyer/SEU-SC-Bridge/releases/download/v1.0.1/file.exe",
		"https://github.com/other/repo/releases/download/v1.0.1/file.exe",
	} {
		if err := validateReleaseURL(r, u); err == nil {
			t.Errorf("accepted untrusted Release URL %q", u)
		}
	}
}
