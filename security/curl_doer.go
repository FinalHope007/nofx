package security

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

const maxCurlRedirects = 10

type curlDoer struct {
	timeout time.Duration
}

var curlForwardedHeaders = []string{
	"sec-ch-ua",
	"sec-ch-ua-mobile",
	"sec-ch-ua-platform",
	"sec-fetch-dest",
	"sec-fetch-mode",
	"sec-fetch-site",
	"upgrade-insecure-requests",
}

func (c *curlDoer) Do(req *http.Request) (*http.Response, error) {
	if err := ValidateURL(req.URL.String()); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(req.Context(), c.timeout)
	defer cancel()

	currentURL := req.URL.String()
	for hop := 0; ; hop++ {
		out, err := c.execOnce(ctx, req, currentURL)
		if err != nil {
			return nil, err
		}
		resp, err := parseCurlResponse(string(out), req)
		if err != nil {
			return nil, err
		}
		next, isRedirect, err := nextCurlRedirect(resp, currentURL)
		if err != nil {
			resp.Body.Close()
			return nil, err
		}
		if !isRedirect {
			return resp, nil
		}
		if hop >= maxCurlRedirects {
			resp.Body.Close()
			return nil, fmt.Errorf("curl subprocess: stopped after %d redirects", maxCurlRedirects)
		}
		resp.Body.Close()
		currentURL = next
	}
}

// nextCurlRedirect resolves and validates the redirect target of a 3xx
// response. Returns the next URL to request, or isRedirect=false when the
// response is final (non-redirect status or missing Location).
func nextCurlRedirect(resp *http.Response, currentURL string) (string, bool, error) {
	if !isRedirectStatus(resp.StatusCode) {
		return "", false, nil
	}
	loc := resp.Header.Get("Location")
	if loc == "" {
		return "", false, nil
	}
	base, err := url.Parse(currentURL)
	if err != nil {
		return "", false, fmt.Errorf("curl subprocess: bad current URL %q: %w", currentURL, err)
	}
	ref, err := url.Parse(loc)
	if err != nil {
		return "", false, fmt.Errorf("curl subprocess: bad redirect location %q: %w", loc, err)
	}
	resolved := base.ResolveReference(ref)
	if err := ValidateURL(resolved.String()); err != nil {
		return "", false, err
	}
	return resolved.String(), true, nil
}

func isRedirectStatus(code int) bool {
	switch code {
	case http.StatusMovedPermanently, http.StatusFound, http.StatusSeeOther,
		http.StatusTemporaryRedirect, http.StatusPermanentRedirect:
		return true
	}
	return false
}

func (c *curlDoer) execOnce(ctx context.Context, req *http.Request, rawURL string) ([]byte, error) {
	args := []string{
		"-sS", "--max-time", strconv.FormatFloat(c.timeout.Seconds(), 'f', 3, 64),
		"-D", "-", "-o", "-",
		"-A", req.Header.Get("User-Agent"),
		"-H", "Accept: " + req.Header.Get("Accept"),
		"-H", "Accept-Language: " + req.Header.Get("Accept-Language"),
		"-H", "Referer: " + req.Header.Get("Referer"),
	}
	for _, name := range curlForwardedHeaders {
		if v := req.Header.Get(name); v != "" {
			args = append(args, "-H", name+": "+v)
		}
	}
	if ck := req.Header.Get("Cookie"); ck != "" {
		args = append(args, "-H", "Cookie: "+ck)
	}
	args = append(args, rawURL)

	out, err := exec.CommandContext(ctx, "curl", args...).Output()
	if err != nil {
		return nil, fmt.Errorf("curl subprocess: %w", err)
	}
	return out, nil
}

func parseCurlResponse(raw string, req *http.Request) (*http.Response, error) {
	lastStatus := strings.LastIndex(strings.ToUpper(raw), "HTTP/")
	if lastStatus < 0 {
		return nil, fmt.Errorf("curl subprocess: malformed response")
	}
	rest := raw[lastStatus:]

	sep := "\r\n\r\n"
	headerEnd := strings.Index(rest, sep)
	if headerEnd < 0 {
		sep = "\n\n"
		headerEnd = strings.Index(rest, sep)
		if headerEnd < 0 {
			return nil, fmt.Errorf("curl subprocess: malformed response")
		}
	}

	head := rest[:headerEnd]
	body := rest[headerEnd+len(sep):]

	lines := strings.Split(head, "\n")
	statusLine := strings.TrimSpace(lines[0])
	parts := strings.Fields(statusLine)
	if len(parts) < 2 {
		return nil, fmt.Errorf("curl subprocess: bad status line %q", statusLine)
	}
	code, err := strconv.Atoi(parts[1])
	if err != nil {
		return nil, fmt.Errorf("curl subprocess: bad status code %q", parts[1])
	}

	resp := &http.Response{
		StatusCode: code,
		Status:     statusLine,
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewReader([]byte(body))),
		Request:    req,
	}
	for _, line := range lines[1:] {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}
		name, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		resp.Header.Add(http.CanonicalHeaderKey(strings.TrimSpace(name)), strings.TrimSpace(value))
	}
	if strings.HasPrefix(strings.ToLower(statusLine), "http/") {
		resp.Proto = "HTTP/1.1"
	}
	return resp, nil
}
