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

func (c *curlDoer) Do(req *http.Request) (*http.Response, error) {
	if err := ValidateURL(req.URL.String()); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(req.Context(), c.timeout)
	defer cancel()

	args := []string{
		"-sS", "-L", "--max-time", strconv.Itoa(int(c.timeout.Seconds())),
		"-D", "-", "-o", "-",
		"-A", req.Header.Get("User-Agent"),
		"-H", "Accept: " + req.Header.Get("Accept"),
		"-H", "Accept-Language: " + req.Header.Get("Accept-Language"),
		"-H", "Referer: " + req.Header.Get("Referer"),
	}
	if ck := req.Header.Get("Cookie"); ck != "" {
		args = append(args, "-H", "Cookie: "+ck)
	}
	args = append(args, req.URL.String())

	out, err := exec.CommandContext(ctx, "curl", args...).Output()
	if err != nil {
		return nil, fmt.Errorf("curl subprocess: %w", err)
	}

	headerEnd := bytes.Index(out, []byte("\r\n\r\n"))
	if headerEnd < 0 {
		headerEnd = bytes.Index(out, []byte("\n\n"))
		if headerEnd < 0 {
			return nil, fmt.Errorf("curl subprocess: malformed response")
		}
	}
	head := string(out[:headerEnd])
	body := out[headerEnd:]
	body = bytes.TrimPrefix(body, []byte("\r\n\r\n"))
	body = bytes.TrimPrefix(body, []byte("\n\n"))

	statusLine := strings.SplitN(head, "\n", 2)[0]
	parts := strings.Fields(statusLine)
	if len(parts) < 2 {
		return nil, fmt.Errorf("curl subprocess: bad status line %q", statusLine)
	}
	code, _ := strconv.Atoi(parts[1])

	resp := &http.Response{
		StatusCode: code,
		Status:     strings.TrimSpace(statusLine),
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewReader(body)),
		Request:    req,
	}
	if strings.HasPrefix(strings.ToLower(head), "http/") {
		resp.Proto = "HTTP/1.1"
	}
	return resp, nil
}
