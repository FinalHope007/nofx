package security

import (
	"net/http"
	"os"
	"strings"
	"time"
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
		return SafeHTTPClient(timeout), nil
	}
}

func isCloudflareChallenge(resp *http.Response) bool {
	if resp == nil || resp.StatusCode != http.StatusForbidden {
		return false
	}
	return strings.EqualFold(resp.Header.Get("Server"), "cloudflare")
}
