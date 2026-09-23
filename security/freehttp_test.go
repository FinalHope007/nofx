package security

import (
	"net/http"
	"testing"
	"time"
)

func TestResolveFreeHTTPModeDefault(t *testing.T) {
	t.Setenv("VERGEX_HTTP_MODE", "")
	if got := ResolveFreeHTTPMode(); got != FreeHTTPFingerprint {
		t.Fatalf("expected fingerprint, got %q", got)
	}
}

func TestResolveFreeHTTPModeOverride(t *testing.T) {
	t.Setenv("VERGEX_HTTP_MODE", "curl")
	if got := ResolveFreeHTTPMode(); got != FreeHTTPCurl {
		t.Fatalf("expected curl, got %q", got)
	}
}

func TestSetBrowserHeaders(t *testing.T) {
	t.Setenv("VERGEX_CF_CLEARANCE", "abc123")
	req, _ := http.NewRequest(http.MethodGet, "https://vergex.trade/x", nil)
	SetBrowserHeaders(req)
	if got := req.Header.Get("User-Agent"); got == "" || got[:7] != "Mozilla" {
		t.Fatalf("unexpected UA %q", got)
	}
	if got := req.Header.Get("Accept-Language"); got == "" {
		t.Fatalf("missing Accept-Language")
	}
	if got := req.Header.Get("Referer"); got != "https://vergex.trade/" {
		t.Fatalf("unexpected Referer %q", got)
	}
	if got := req.Header.Get("Cookie"); got != "cf_clearance=abc123; cookieConsent=true" {
		t.Fatalf("unexpected Cookie %q", got)
	}
}

func TestNewFreeHTTPClientCurlMode(t *testing.T) {
	t.Setenv("VERGEX_HTTP_MODE", "curl")
	d, err := NewFreeHTTPClient(5 * time.Second)
	if err != nil {
		t.Fatalf("NewFreeHTTPClient: %v", err)
	}
	if _, ok := d.(*curlDoer); !ok {
		t.Fatalf("expected *curlDoer, got %T", d)
	}
}
