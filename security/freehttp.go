package security

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	fhttp "github.com/bogdanfinn/fhttp"
	tlsclient "github.com/bogdanfinn/tls-client"
	"github.com/bogdanfinn/tls-client/profiles"
)

type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

type FreeHTTPMode string

const (
	FreeHTTPFingerprint FreeHTTPMode = "fingerprint"
	FreeHTTPCurl        FreeHTTPMode = "curl"
	FreeHTTPStdlib      FreeHTTPMode = "stdlib"
)

const chromeUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/153.0.0.0 Safari/537.36"

func ResolveFreeHTTPMode() FreeHTTPMode {
	switch strings.TrimSpace(strings.ToLower(os.Getenv("VERGEX_HTTP_MODE"))) {
	case "curl":
		return FreeHTTPCurl
	case "stdlib":
		return FreeHTTPStdlib
	default:
		return FreeHTTPFingerprint
	}
}

func SetBrowserHeaders(req *http.Request) {
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", chromeUserAgent)
	}
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Referer", "https://vergex.trade/")
	req.Header.Set("sec-ch-ua", `"Google Chrome";v="153", "Not_A Brand";v="8", "Chromium";v="153"`)
	req.Header.Set("sec-ch-ua-mobile", "?0")
	req.Header.Set("sec-ch-ua-platform", `"Windows"`)
	req.Header.Set("sec-fetch-dest", "document")
	req.Header.Set("sec-fetch-mode", "navigate")
	req.Header.Set("sec-fetch-site", "none")
	req.Header.Set("upgrade-insecure-requests", "1")
	if cc := strings.TrimSpace(os.Getenv("VERGEX_CF_CLEARANCE")); cc != "" {
		req.Header.Set("Cookie", "cf_clearance="+cc+"; cookieConsent=true")
	}
}

func NewFreeHTTPClient(timeout time.Duration) (HTTPDoer, error) {
	switch ResolveFreeHTTPMode() {
	case FreeHTTPCurl:
		return &curlDoer{timeout: timeout}, nil
	case FreeHTTPStdlib:
		return SafeHTTPClient(timeout), nil
	default:
		return newFingerprintClient(timeout)
	}
}

// fingerprintProfiles is the ordered set of TLS fingerprints tried before
// falling back to curl. Cloudflare's managed-challenge tuning blocks some
// profiles and allows others, and which ones are blocked shifts over time, so
// a single pinned profile is fragile.
var fingerprintProfiles = []profiles.ClientProfile{
	profiles.Chrome_133,
	profiles.Firefox_120,
	profiles.Safari_16_0,
}

func newFingerprintProfileClient(timeout time.Duration, profile profiles.ClientProfile) (HTTPDoer, error) {
	jar := tlsclient.NewCookieJar()
	opts := []tlsclient.HttpClientOption{
		tlsclient.WithTimeoutSeconds(int(timeout.Seconds())),
		tlsclient.WithClientProfile(profile),
		tlsclient.WithCookieJar(jar),
		tlsclient.WithCustomRedirectFunc(fingerprintRedirectCheck),
	}
	tc, err := tlsclient.NewHttpClient(tlsclient.NewNoopLogger(), opts...)
	if err != nil {
		return nil, fmt.Errorf("tls-client init: %w", err)
	}
	return &tlsAdapter{client: tc}, nil
}

func newFingerprintClient(timeout time.Duration) (HTTPDoer, error) {
	doers := make([]HTTPDoer, 0, len(fingerprintProfiles)+1)
	for _, profile := range fingerprintProfiles {
		d, err := newFingerprintProfileClient(timeout, profile)
		if err != nil {
			return nil, err
		}
		doers = append(doers, d)
	}
	doers = append(doers, &curlDoer{timeout: timeout})
	return &fallbackDoer{doers: doers}, nil
}

// fingerprintRedirectCheck validates every redirect target the tls-client
// follows (requests are fhttp types). Mirrors SafeHTTPClient's CheckRedirect.
func fingerprintRedirectCheck(req *fhttp.Request, via []*fhttp.Request) error {
	if len(via) >= maxCurlRedirects {
		return fmt.Errorf("stopped after %d redirects", maxCurlRedirects)
	}
	return ValidateURL(req.URL.String())
}

type tlsAdapter struct {
	client tlsclient.HttpClient
}

func (a *tlsAdapter) Do(req *http.Request) (*http.Response, error) {
	if err := ValidateURL(req.URL.String()); err != nil {
		return nil, err
	}
	freq, err := toFHTTPRequest(req)
	if err != nil {
		return nil, err
	}
	fresp, err := a.client.Do(freq)
	if err != nil {
		return nil, err
	}
	return toNetHTTPResponse(fresp, req), nil
}

func toFHTTPRequest(req *http.Request) (*fhttp.Request, error) {
	freq, err := fhttp.NewRequestWithContext(req.Context(), req.Method, req.URL.String(), nil)
	if err != nil {
		return nil, err
	}
	if req.Body != nil {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		req.Body = io.NopCloser(bytes.NewReader(body))
		freq.Body = io.NopCloser(bytes.NewReader(body))
	}
	freq.Header = fhttp.Header(req.Header.Clone())
	freq.Host = req.Host
	return freq, nil
}

func toNetHTTPResponse(fresp *fhttp.Response, req *http.Request) *http.Response {
	header := make(http.Header, len(fresp.Header))
	for k, v := range fresp.Header {
		header[k] = append([]string(nil), v...)
	}
	return &http.Response{
		Status:        fresp.Status,
		StatusCode:    fresp.StatusCode,
		Proto:         fresp.Proto,
		ProtoMajor:    fresp.ProtoMajor,
		ProtoMinor:    fresp.ProtoMinor,
		Header:        header,
		Body:          fresp.Body,
		ContentLength: fresp.ContentLength,
		Request:       req,
	}
}

// fallbackDoer tries a sequence of transports in order and sticks with the
// first one that does not return a Cloudflare challenge. The index is advanced
// past any transport whose response is a Cloudflare challenge, so subsequent
// requests skip the blocked fingerprint(s) for the process lifetime.
type fallbackDoer struct {
	doers []HTTPDoer
	idx   int
	mu    sync.Mutex
}

func (f *fallbackDoer) Do(req *http.Request) (*http.Response, error) {
	f.mu.Lock()
	idx := f.idx
	f.mu.Unlock()

	for ; idx < len(f.doers); idx++ {
		resp, err := f.doers[idx].Do(req)
		if err != nil {
			return nil, err
		}
		if !isCloudflareChallenge(resp) {
			return resp, nil
		}
		// Challenge: drain this response and demote to the next transport.
		if resp.Body != nil {
			_, _ = io.ReadAll(resp.Body)
			resp.Body.Close()
		}
		f.mu.Lock()
		if f.idx < idx+1 {
			f.idx = idx + 1
		}
		f.mu.Unlock()
	}
	// All transports exhausted their non-challenge attempts; use the last one
	// (curl) so the caller still receives a concrete response/error.
	f.mu.Lock()
	last := f.idx - 1
	f.mu.Unlock()
	if last < 0 {
		last = 0
	}
	return f.doers[last].Do(req)
}

func isCloudflareChallenge(resp *http.Response) bool {
	if resp == nil || resp.StatusCode != http.StatusForbidden {
		return false
	}
	return strings.EqualFold(resp.Header.Get("Server"), "cloudflare")
}
