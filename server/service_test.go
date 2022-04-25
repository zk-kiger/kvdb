package server

import (
	"net/url"
	"strings"
	"testing"
)

func TestService_ParseURL(t *testing.T) {
	url, err := url.Parse("http://localhost:8080/key/foo?level=default")
	if err != nil {
		t.Fatalf("failed to parse url, err: %v", err)
	}
	if strings.Compare(url.Host, "localhost:8080") != 0 {
		t.Fatalf("expected url host not: %s", url.Host)
	}
	if strings.Compare(url.Path, "/key/foo") != 0 {
		t.Fatalf("expected url path not: %s", url.Path)
	}
	if strings.Compare(url.RawQuery, "level=default") != 0 {
		t.Fatalf("expected raw query not: %s", url.RawQuery)
	}
}
