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

func newFingerprintClient(timeout time.Duration) (HTTPDoer, error) {
	jar := tlsclient.NewCookieJar()
	opts := []tlsclient.HttpClientOption{
		tlsclient.WithTimeoutSeconds(int(timeout.Seconds())),
		tlsclient.WithClientProfile(profiles.Chrome_133),
		tlsclient.WithCookieJar(jar),
	}
	tc, err := tlsclient.NewHttpClient(tlsclient.NewNoopLogger(), opts...)
	if err != nil {
		return nil, fmt.Errorf("tls-client init: %w", err)
	}
	return &fallbackDoer{
		primary:   &tlsAdapter{client: tc},
		secondary: &curlDoer{timeout: timeout},
	}, nil
}

type tlsAdapter struct {
	client tlsclient.HttpClient
}

func (a *tlsAdapter) Do(req *http.Request) (*http.Response, error) {
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

type fallbackDoer struct {
	primary   HTTPDoer
	secondary HTTPDoer
	demoted   bool
	mu        sync.Mutex
}

func (f *fallbackDoer) Do(req *http.Request) (*http.Response, error) {
	f.mu.Lock()
	demoted := f.demoted
	f.mu.Unlock()

	if !demoted {
		resp, err := f.primary.Do(req)
		if err == nil && isCloudflareChallenge(resp) {
			f.mu.Lock()
			f.demoted = true
			f.mu.Unlock()
			var body []byte
			if resp.Body != nil {
				body, _ = io.ReadAll(resp.Body)
				resp.Body.Close()
			}
			retry := req.Clone(req.Context())
			retry.Body = io.NopCloser(bytes.NewReader(body))
			return f.secondary.Do(retry)
		}
		return resp, err
	}
	return f.secondary.Do(req)
}

func isCloudflareChallenge(resp *http.Response) bool {
	if resp == nil || resp.StatusCode != http.StatusForbidden {
		return false
	}
	return strings.EqualFold(resp.Header.Get("Server"), "cloudflare")
}
