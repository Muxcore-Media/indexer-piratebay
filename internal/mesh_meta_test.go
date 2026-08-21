package internal

import "testing"

func TestParseInfoHashFromDownloadURL(t *testing.T) {
	ih := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	got := parseInfoHashFromDownloadURL("magnet:?xt=urn:btih:" + ih + "&dn=x")
	if got != ih {
		t.Fatalf("got %q", got)
	}
	if parseInfoHashFromDownloadURL("http://example/download") != "" {
		t.Fatal("expected empty for http")
	}
}
