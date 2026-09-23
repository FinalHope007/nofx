package security

import (
	"io"
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

func TestCurlDoerParseRedirectChain(t *testing.T) {
	out := "HTTP/1.1 301 Moved Permanently\r\n" +
		"Server: cloudflare\r\n" +
		"Location: https://vergex.trade/final\r\n" +
		"Content-Length: 0\r\n" +
		"\r\n" +
		"HTTP/1.1 403 Forbidden\r\n" +
		"Server: cloudflare\r\n" +
		"Content-Type: text/html\r\n" +
		"\r\n" +
		"<html>challenge</html>"

	resp, err := parseCurlResponse(out, nil)
	if err != nil {
		t.Fatalf("parseCurlResponse: %v", err)
	}
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", resp.StatusCode)
	}
	if got := resp.Header.Get("Server"); got != "cloudflare" {
		t.Fatalf("expected Server cloudflare, got %q", got)
	}
	if !isCloudflareChallenge(resp) {
		t.Fatalf("expected isCloudflareChallenge true")
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "<html>challenge</html>" {
		t.Fatalf("unexpected body %q", string(body))
	}
}

func TestCurlDoerParseLFOnly(t *testing.T) {
	out := "HTTP/1.1 200 OK\n" +
		"Content-Type: text/plain\n" +
		"\n" +
		"hello"

	resp, err := parseCurlResponse(out, nil)
	if err != nil {
		t.Fatalf("parseCurlResponse: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Type"); got != "text/plain" {
		t.Fatalf("unexpected Content-Type %q", got)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "hello" {
		t.Fatalf("unexpected body %q", string(body))
	}
}
