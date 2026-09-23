package security

import (
	"errors"
	"io"
	"net/http"
	"net/url"
	"testing"
	"time"

	fhttp "github.com/bogdanfinn/fhttp"
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

func TestFallbackDoerDemotesOnChallenge(t *testing.T) {
	primaryCalls, secondaryCalls := 0, 0
	challenge := &http.Response{StatusCode: http.StatusForbidden, Header: http.Header{"Server": []string{"cloudflare"}}}
	ok := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: http.NoBody}

	fd := &fallbackDoer{
		doers: []HTTPDoer{
			doerFunc(func(*http.Request) (*http.Response, error) { primaryCalls++; return challenge, nil }),
			doerFunc(func(*http.Request) (*http.Response, error) { secondaryCalls++; return ok, nil }),
		},
	}
	req, _ := http.NewRequest(http.MethodGet, "https://vergex.trade/x", nil)

	if _, err := fd.Do(req); err != nil {
		t.Fatalf("first Do: %v", err)
	}
	if primaryCalls != 1 || secondaryCalls != 1 {
		t.Fatalf("expected demotion to secondary on first call, primary=%d secondary=%d", primaryCalls, secondaryCalls)
	}
	if _, err := fd.Do(req); err != nil {
		t.Fatalf("second Do: %v", err)
	}
	if primaryCalls != 1 || secondaryCalls != 2 {
		t.Fatalf("expected sticky secondary, primary=%d secondary=%d", primaryCalls, secondaryCalls)
	}
}

func TestFallbackDoerSkipsMultipleBlockedProfiles(t *testing.T) {
	var calls [4]int
	challenge := func() *http.Response {
		return &http.Response{StatusCode: http.StatusForbidden, Header: http.Header{"Server": []string{"cloudflare"}}}
	}
	ok := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: http.NoBody}
	mk := func(i int) HTTPDoer {
		return doerFunc(func(*http.Request) (*http.Response, error) {
			calls[i]++
			return challenge(), nil
		})
	}
	fd := &fallbackDoer{
		doers: []HTTPDoer{mk(0), mk(1), doerFunc(func(*http.Request) (*http.Response, error) {
			calls[2]++
			return ok, nil
		}), mk(3)},
	}
	req, _ := http.NewRequest(http.MethodGet, "https://vergex.trade/x", nil)

	for i := 0; i < 3; i++ {
		if _, err := fd.Do(req); err != nil {
			t.Fatalf("Do %d: %v", i, err)
		}
	}
	if calls[0] != 1 || calls[1] != 1 || calls[2] != 3 || calls[3] != 0 {
		t.Fatalf("expected sticky good transport after two blocked profiles, got %v", calls)
	}
}

func TestFallbackDoerExhaustedReturnsLastResponse(t *testing.T) {
	challenge := &http.Response{StatusCode: http.StatusForbidden, Header: http.Header{"Server": []string{"cloudflare"}}}
	var lastCalls int
	fd := &fallbackDoer{
		doers: []HTTPDoer{
			doerFunc(func(*http.Request) (*http.Response, error) { return challenge, nil }),
			doerFunc(func(*http.Request) (*http.Response, error) { lastCalls++; return challenge, nil }),
		},
	}
	req, _ := http.NewRequest(http.MethodGet, "https://vergex.trade/x", nil)
	resp, err := fd.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	if resp.StatusCode != http.StatusForbidden || lastCalls != 2 {
		t.Fatalf("expected final challenge from last doer, status=%d lastCalls=%d", resp.StatusCode, lastCalls)
	}
}

type doerFunc func(*http.Request) (*http.Response, error)

func (f doerFunc) Do(r *http.Request) (*http.Response, error) { return f(r) }

func TestTLSAdapterBlocksPrivateIP(t *testing.T) {
	t.Setenv("ALLOW_LOCAL_CUSTOM_API", "")
	req, _ := http.NewRequest(http.MethodGet, "http://127.0.0.1/", nil)
	a := &tlsAdapter{client: nil}
	_, err := a.Do(req)
	if err == nil {
		t.Fatalf("expected SSRF block, got nil error")
	}
	var ssrf *SSRFError
	if !errors.As(err, &ssrf) {
		t.Fatalf("expected *SSRFError, got %T: %v", err, err)
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

func TestCurlDoerRedirectToPrivateIPRejected(t *testing.T) {
	t.Setenv("ALLOW_LOCAL_CUSTOM_API", "")
	out := "HTTP/1.1 302 Found\r\n" +
		"Location: http://127.0.0.1/\r\n" +
		"Content-Length: 0\r\n" +
		"\r\n"

	resp, err := parseCurlResponse(out, nil)
	if err != nil {
		t.Fatalf("parseCurlResponse: %v", err)
	}
	_, isRedirect, err := nextCurlRedirect(resp, "https://vergex.trade/x")
	if err == nil {
		t.Fatalf("expected SSRF rejection of redirect to 127.0.0.1")
	}
	var ssrf *SSRFError
	if !errors.As(err, &ssrf) {
		t.Fatalf("expected *SSRFError, got %T: %v", err, err)
	}
	if isRedirect {
		t.Fatalf("must not report redirect as followable when SSRF-blocked")
	}
}

func TestCurlDoerRedirectResolvesAndValidates(t *testing.T) {
	t.Setenv("ALLOW_LOCAL_CUSTOM_API", "")
	out := "HTTP/1.1 301 Moved Permanently\r\n" +
		"Location: /final\r\n" +
		"Content-Length: 0\r\n" +
		"\r\n"

	resp, err := parseCurlResponse(out, nil)
	if err != nil {
		t.Fatalf("parseCurlResponse: %v", err)
	}
	next, isRedirect, err := nextCurlRedirect(resp, "http://93.184.216.34/x")
	if err != nil {
		t.Fatalf("nextCurlRedirect: %v", err)
	}
	if !isRedirect {
		t.Fatalf("expected redirect")
	}
	if next != "http://93.184.216.34/final" {
		t.Fatalf("unexpected resolved URL %q", next)
	}
}

func TestFingerprintRedirectCheckBlocksPrivateIP(t *testing.T) {
	t.Setenv("ALLOW_LOCAL_CUSTOM_API", "")
	u, err := url.Parse("http://127.0.0.1/")
	if err != nil {
		t.Fatalf("url.Parse: %v", err)
	}
	req := &fhttp.Request{Method: fhttp.MethodGet, URL: u}
	err = fingerprintRedirectCheck(req, nil)
	if err == nil {
		t.Fatalf("expected SSRF rejection")
	}
	var ssrf *SSRFError
	if !errors.As(err, &ssrf) {
		t.Fatalf("expected *SSRFError, got %T: %v", err, err)
	}
}

func TestFingerprintRedirectCheckCapsHops(t *testing.T) {
	t.Setenv("ALLOW_LOCAL_CUSTOM_API", "")
	u, err := url.Parse("http://93.184.216.34/x")
	if err != nil {
		t.Fatalf("url.Parse: %v", err)
	}
	req := &fhttp.Request{Method: fhttp.MethodGet, URL: u}
	via := make([]*fhttp.Request, maxCurlRedirects)
	if err := fingerprintRedirectCheck(req, via); err == nil {
		t.Fatalf("expected hop-limit error at %d redirects", maxCurlRedirects)
	}
}
