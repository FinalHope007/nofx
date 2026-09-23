package security

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

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

	args := []string{
		"-sS", "-L", "--max-time", strconv.FormatFloat(c.timeout.Seconds(), 'f', 3, 64),
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
	args = append(args, req.URL.String())

	out, err := exec.CommandContext(ctx, "curl", args...).Output()
	if err != nil {
		return nil, fmt.Errorf("curl subprocess: %w", err)
	}

	return parseCurlResponse(string(out), req)
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
