# Vergex.trade Cloudflare Resilience & Position Management on Failure — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make all free vergex.trade calls survive Cloudflare's managed challenge, serve a last-good candidate pool for up to 2 hours during a fetch outage with an explicit warning, and ensure the LLM still manages open positions whenever the candidate pool is unavailable.

**Architecture:** Three independent layers. (1) A Cloudflare-resilient HTTP transport in `security/` using a Chrome TLS/HTTP2 fingerprint with automatic curl-subprocess fallback. (2) A TTL cache with 15-minute fresh window and 2-hour max stale embedded in `nofxos.FreeTrendingClient`, populated only on successful fetches. (3) A trader-loop change that no longer aborts on empty candidates when positions are open, so the LLM is always called to manage positions.

**Tech Stack:** Go 1.25, `github.com/bogdanfinn/tls-client` (Chrome fingerprint), `os/exec` curl fallback, existing `sync.Mutex` TTL-cache pattern from `provider/nofxos/ai500_cache.go`, React/TS frontend consuming the existing `execution_log` field.

## Global Constraints

- Go module is `nofx`; Go version `1.25.11` (from `go.mod`).
- Cache fresh window: `15 * time.Minute`. Cache max stale: `2 * time.Hour`. Exact values.
- Cache is populated **only from a successful fetch**. A successful fetch that legitimately omits a coin does NOT trigger stale-serve.
- Transport mode default `fingerprint`; demote to `curl` on the first 403 Cloudflare challenge. Env `VERGEX_HTTP_MODE` ∈ {`fingerprint`,`curl`,`stdlib`} pins the mode.
- Browser headers: Chrome `User-Agent`, `Accept`, `Accept-Language: en-US,en;q=0.9`, `Referer: https://vergex.trade/`, `sec-fetch-dest/mode/site`.
- Optional cookie env: `VERGEX_CF_CLEARANCE` → `Cookie: cf_clearance=<v>; cookieConsent=true`. Never commit its value.
- SSRF protection from `security.SafeHTTPClient` (`security/url_validator.go:177`) MUST be preserved in all transports.
- Do NOT add comments unless they explain non-obvious intent, matching existing style.
- Existing getter signatures (`GetAI500()`, `GetOITop(int)`, etc.) MUST keep working for callers not yet migrated; add stale-aware variants instead of breaking them.

---

### Task 1: Resilient transport package — browser headers + doer abstraction

**Files:**
- Create: `security/freehttp.go`
- Test: `security/freehttp_test.go`

**Interfaces:**
- Consumes: `security.ValidateURL(string) error`, `security.SafeHTTPClient(time.Duration) *http.Client` (both in `security/url_validator.go`).
- Produces:
  - `type HTTPDoer interface { Do(req *http.Request) (*http.Response, error) }`
  - `type FreeHTTPMode string` with consts `FreeHTTPFingerprint`, `FreeHTTPCurl`, `FreeHTTPStdlib` (values `"fingerprint"`, `"curl"`, `"stdlib"`).
  - `func ResolveFreeHTTPMode() FreeHTTPMode` — reads `VERGEX_HTTP_MODE`, defaults `FreeHTTPFingerprint`.
  - `func SetBrowserHeaders(req *http.Request)` — sets UA/Accept/Accept-Language/Referer/sec-fetch headers; attaches `cf_clearance` cookie from `VERGEX_CF_CLEARANCE` if set.
  - `func NewFreeHTTPClient(timeout time.Duration) (HTTPDoer, error)` — returns a `*http.Client` for `fingerprint`/`stdlib`, or `*curlDoer` for `curl`.
  - `type curlDoer struct{ timeout time.Duration }` implementing `HTTPDoer`.

- [ ] **Step 1: Write the failing test for headers and mode resolution**

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /mnt/e/Users/limli/Documents/GitHub/nofx && go test ./security/ -run "TestResolveFreeHTTPMode|TestSetBrowserHeaders|TestNewFreeHTTPClientCurlMode" -v`
Expected: FAIL — undefined: `ResolveFreeHTTPMode`, `FreeHTTPFingerprint`, etc.

- [ ] **Step 3: Implement `security/freehttp.go`**

```go
package security

import (
	"fmt"
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
		return newFingerprintClient(timeout)
	}
}

func isCloudflareChallenge(resp *http.Response) bool {
	if resp == nil || resp.StatusCode != http.StatusForbidden {
		return false
	}
	return strings.EqualFold(resp.Header.Get("Server"), "cloudflare")
}

var _ = fmt.Sprintf
```

- [ ] **Step 4: Add curl doer (separate file for testability)**

Create `security/curl_doer.go`:

```go
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
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd /mnt/e/Users/limli/Documents/GitHub/nofx && go test ./security/ -run "TestResolveFreeHTTPMode|TestSetBrowserHeaders|TestNewFreeHTTPClientCurlMode" -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
cd /mnt/e/Users/limli/Documents/GitHub/nofx
git add security/freehttp.go security/curl_doer.go security/freehttp_test.go
git commit -m "feat(security): resilient free-HTTP transport with browser headers and curl fallback"
```

---

### Task 2: Fingerprint client with automatic curl demotion

**Files:**
- Modify: `security/freehttp.go`
- Modify: `go.mod`, `go.sum`
- Test: `security/freehttp_test.go`

**Interfaces:**
- Consumes: `curlDoer`, `FreeHTTPMode` from Task 1.
- Produces:
  - `func newFingerprintClient(timeout time.Duration) (HTTPDoer, error)`
  - `type fallbackDoer struct { primary HTTPDoer; secondary HTTPDoer; demoted bool; mu sync.Mutex }` implementing `HTTPDoer`; on a primary response satisfying `isCloudflareChallenge`, it switches to `secondary` for that and subsequent requests.
  - `NewFreeHTTPClient` in `fingerprint` mode returns a `*fallbackDoer` wrapping the fingerprint client and a `curlDoer`.

- [ ] **Step 1: Add dependency**

Run: `cd /mnt/e/Users/limli/Documents/GitHub/nofx && go get github.com/bogdanfinn/tls-client@latest && go mod tidy`
Expected: `go.mod` gains `github.com/bogdanfinn/tls-client`; `go.sum` updated.

- [ ] **Step 2: Write the failing test for demotion**

```go
func TestFallbackDoerDemotesOnChallenge(t *testing.T) {
	primaryCalls, secondaryCalls := 0, 0
	challenge := &http.Response{StatusCode: http.StatusForbidden, Header: http.Header{"Server": []string{"cloudflare"}}}
	ok := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: http.NoBody}

	fd := &fallbackDoer{
		primary:   doerFunc(func(*http.Request) (*http.Response, error) { primaryCalls++; return challenge, nil }),
		secondary: doerFunc(func(*http.Request) (*http.Response, error) { secondaryCalls++; return ok, nil }),
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

type doerFunc func(*http.Request) (*http.Response, error)

func (f doerFunc) Do(r *http.Request) (*http.Response, error) { return f(r) }
```

- [ ] **Step 3: Run test to verify it fails**

Run: `cd /mnt/e/Users/limli/Documents/GitHub/nofx && go test ./security/ -run TestFallbackDoerDemotesOnChallenge -v`
Expected: FAIL — undefined: `fallbackDoer`, `doerFunc`.

- [ ] **Step 4: Implement fingerprint client and fallback doer**

Append to `security/freehttp.go`:

```go
import (
	"bytes"
	"io"
	"net/http"
	"sync"
	"time"

	tlsclient "github.com/bogdanfinn/tls-client"
	"github.com/bogdanfinn/tls-client/profiles"
)

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
	return a.client.Do(req)
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
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			retry := req.Clone(req.Context())
			retry.Body = io.NopCloser(bytes.NewReader(body))
			return f.secondary.Do(retry)
		}
		return resp, err
	}
	return f.secondary.Do(req)
}
```

Note: `tlsclient.NewHttpClient` requires a logger implementing its `Logger` interface. If `tlsclient.NewNoopLogger()` is not present in the pinned version, use the package's documented noop logger or provide a tiny local type satisfying the interface; run `go doc github.com/bogdanfinn/tls-client` to confirm the exact symbol before writing this step.

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd /mnt/e/Users/limli/Documents/GitHub/nofx && go test ./security/ -run "TestFallbackDoerDemotesOnChallenge|TestResolveFreeHTTPMode|TestSetBrowserHeaders|TestNewFreeHTTPClientCurlMode" -v`
Expected: PASS

- [ ] **Step 6: Build the whole module**

Run: `cd /mnt/e/Users/limli/Documents/GitHub/nofx && go build ./...`
Expected: success.

- [ ] **Step 7: Commit**

```bash
cd /mnt/e/Users/limli/Documents/GitHub/nofx
git add security/freehttp.go security/freehttp_test.go go.mod go.sum
git commit -m "feat(security): Chrome-fingerprint client with automatic curl demotion on Cloudflare challenge"
```

---

### Task 3: Wire the resilient transport into nofxos free client

**Files:**
- Modify: `provider/nofxos/free.go:1-33, 245-261`
- Test: `provider/nofxos/free_test.go`

**Interfaces:**
- Consumes: `security.NewFreeHTTPClient`, `security.SetBrowserHeaders`, `security.HTTPDoer`.
- Produces: `FreeTrendingClient.http` field type changes from `*http.Client` to `security.HTTPDoer`. `NewFreeTrendingClient()` unchanged externally.

- [ ] **Step 1: Write the failing test asserting browser headers on the outgoing request**

```go
func TestFreeTrendingClient_SendsBrowserHeaders(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// go test http server runs on 127.0.0.1; the SSRF-guarded clients would block it,
		// so assert headers via a direct call path with a stdlib-backed client.
		if !strings.HasPrefix(r.Header.Get("User-Agent"), "Mozilla") {
			t.Errorf("missing browser UA, got %q", r.Header.Get("User-Agent"))
		}
		if r.Header.Get("Referer") != "https://vergex.trade/" {
			t.Errorf("missing Referer, got %q", r.Header.Get("Referer"))
		}
		w.Write([]byte(`{"category":{"assets":[]}}`))
	}))
	defer srv.Close()

	c := NewFreeTrendingClient()
	c.baseURL = srv.URL
	c.http = srv.Client() // stdlib client bypasses fingerprint/curl for this assertion
	if _, err := c.GetAI500(); err != nil {
		t.Fatalf("GetAI500: %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /mnt/e/Users/limli/Documents/GitHub/nofx && go test ./provider/nofxos/ -run TestFreeTrendingClient_SendsBrowserHeaders -v`
Expected: FAIL — headers not set (the `get()` method does not call `SetBrowserHeaders`), or type mismatch on `c.http`.

- [ ] **Step 3: Update imports, struct, constructor, and `get()`**

In `provider/nofxos/free.go`:
- Change the `http` field to `http security.HTTPDoer`.
- In `NewFreeTrendingClient()`, initialize via `client, err := security.NewFreeHTTPClient(30 * time.Second); if err != nil { client = security.SafeHTTPClient(30 * time.Second) }` and set `http: client`.
- In `get()` (currently lines ~245-260), after `http.NewRequestWithContext`, replace the manual `req.Header.Set("User-Agent", ...)` with `security.SetBrowserHeaders(req)`.

```go
func (c *FreeTrendingClient) get(fullURL string) (json.RawMessage, error) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, fullURL, nil)
	if err != nil {
		return nil, fmt.Errorf("trending request: %w", err)
	}
	security.SetBrowserHeaders(req)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("trending GET: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("trending GET: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	return io.ReadAll(resp.Body)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /mnt/e/Users/limli/Documents/GitHub/nofx && go test ./provider/nofxos/ -count=1 -v`
Expected: PASS (existing `free_test.go` tests still pass; new header test passes).

- [ ] **Step 5: Commit**

```bash
cd /mnt/e/Users/limli/Documents/GitHub/nofx
git add provider/nofxos/free.go provider/nofxos/free_test.go
git commit -m "feat(nofxos): route free trending client through resilient transport"
```

---

### Task 4: Wire the resilient transport into vergex free client

**Files:**
- Modify: `provider/vergex/client.go:174-188, 355-379`
- Test: `provider/vergex/client_free_test.go` (create if absent; otherwise append)

**Interfaces:**
- Consumes: `security.NewFreeHTTPClient`, `security.SetBrowserHeaders`, `security.HTTPDoer`.
- Produces: `Client.httpClient` type changes from `*http.Client` to `security.HTTPDoer`; `NewFreeClient` unchanged externally.

- [ ] **Step 1: Write the failing test**

```go
func TestFreeClient_doFreeGET_SendsBrowserHeaders(t *testing.T) {
	t.Setenv("ALLOW_LOCAL_CUSTOM_API", "1")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("User-Agent"), "Mozilla") {
			t.Errorf("missing browser UA, got %q", r.Header.Get("User-Agent"))
		}
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	c, err := NewFreeClient(srv.URL, "", nil)
	if err != nil {
		t.Fatalf("NewFreeClient: %v", err)
	}
	c.httpClient = srv.Client()
	if _, err := c.doFreeGET(context.Background(), srv.URL+"/x"); err != nil {
		t.Fatalf("doFreeGET: %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /mnt/e/Users/limli/Documents/GitHub/nofx && go test ./provider/vergex/ -run TestFreeClient_doFreeGET_SendsBrowserHeaders -v`
Expected: FAIL — headers not set or type mismatch.

- [ ] **Step 3: Update struct, `NewFreeClient`, and `doFreeGET`**

In `provider/vergex/client.go`:
- Change `httpClient *http.Client` to `httpClient security.HTTPDoer`.
- In `NewClient` (paid), keep `&http.Client{Timeout: 30 * time.Second}` — it satisfies `HTTPDoer`.
- In `NewFreeClient`, set `httpClient` from `security.NewFreeHTTPClient(30 * time.Second)` with fallback to `security.SafeHTTPClient(30 * time.Second)`.
- In `doFreeGET`, replace `req.Header.Set("User-Agent", ...)` with `security.SetBrowserHeaders(req)`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /mnt/e/Users/limli/Documents/GitHub/nofx && go test ./provider/vergex/ -count=1`
Expected: PASS.

- [ ] **Step 5: Build and commit**

```bash
cd /mnt/e/Users/limli/Documents/GitHub/nofx
go build ./...
git add provider/vergex/client.go provider/vergex/client_free_test.go
git commit -m "feat(vergex): route free client through resilient transport"
```

---

### Task 5: TTL cache with 2-hour stale-serve in nofxos

**Files:**
- Create: `provider/nofxos/free_cache.go`
- Test: `provider/nofxos/free_cache_test.go`

**Interfaces:**
- Consumes: nothing external.
- Produces:
  - `const freePoolFreshTTL = 15 * time.Minute`
  - `const freePoolMaxStale = 2 * time.Hour`
  - `type poolResult[T any] struct { Value T; Stale bool; Age time.Duration }`
  - `type poolCache[T any] struct { mu sync.Mutex; value T; fetchedAt time.Time; hasValue bool }`
  - `func (c *poolCache[T]) get(fetch func() (T, error)) (poolResult[T], error)` — the core logic:
    - fresh (`time.Since(fetchedAt) < freePoolFreshTTL`) → return cached, `Stale=false`.
    - else call `fetch()`; on success store and return fresh; on error and `hasValue && time.Since(fetchedAt) < freePoolMaxStale` → return cached with `Stale=true, Age=time.Since(fetchedAt)`; else return zero + error.

- [ ] **Step 1: Write the failing tests**

```go
package nofxos

import (
	"errors"
	"testing"
	"time"
)

func TestPoolCacheFreshHitSkipsFetch(t *testing.T) {
	var c poolCache[int]
	fetches := 0
	fetch := func() (int, error) { fetches++; return 1, nil }

	if r, err := c.get(fetch); err != nil || r.Value != 1 || r.Stale {
		t.Fatalf("first get: %+v err=%v", r, err)
	}
	if r, err := c.get(fetch); err != nil || r.Stale {
		t.Fatalf("second get: %+v err=%v", r, err)
	}
	if fetches != 1 {
		t.Fatalf("expected 1 fetch within fresh window, got %d", fetches)
	}
}

func TestPoolCacheStaleServeOnErrorWithinMaxStale(t *testing.T) {
	var c poolCache[int]
	if _, err := c.get(func() (int, error) { return 7, nil }); err != nil {
		t.Fatalf("seed: %v", err)
	}
	c.fetchedAt = time.Now().Add(-30 * time.Minute)

	r, err := c.get(func() (int, error) { return 0, errors.New("403") })
	if err != nil {
		t.Fatalf("expected stale serve, got err %v", err)
	}
	if !r.Stale || r.Value != 7 {
		t.Fatalf("expected stale value 7, got %+v", r)
	}
}

func TestPoolCacheExpiresAfterMaxStale(t *testing.T) {
	var c poolCache[int]
	if _, err := c.get(func() (int, error) { return 7, nil }); err != nil {
		t.Fatalf("seed: %v", err)
	}
	c.fetchedAt = time.Now().Add(-3 * time.Hour)

	if _, err := c.get(func() (int, error) { return 0, errors.New("403") }); err == nil {
		t.Fatalf("expected error after max stale")
	}
}

func TestPoolCacheColdFetchError(t *testing.T) {
	var c poolCache[int]
	if _, err := c.get(func() (int, error) { return 0, errors.New("403") }); err == nil {
		t.Fatalf("expected error on cold failure")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /mnt/e/Users/limli/Documents/GitHub/nofx && go test ./provider/nofxos/ -run "TestPoolCache" -v`
Expected: FAIL — undefined: `poolCache`, `poolResult`.

- [ ] **Step 3: Implement `provider/nofxos/free_cache.go`**

```go
package nofxos

import (
	"sync"
	"time"
)

const (
	freePoolFreshTTL = 15 * time.Minute
	freePoolMaxStale = 2 * time.Hour
)

type poolResult[T any] struct {
	Value T
	Stale bool
	Age   time.Duration
}

type poolCache[T any] struct {
	mu        sync.Mutex
	value     T
	fetchedAt time.Time
	hasValue  bool
}

func (c *poolCache[T]) get(fetch func() (T, error)) (poolResult[T], error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.hasValue && time.Since(c.fetchedAt) < freePoolFreshTTL {
		return poolResult[T]{Value: c.value, Stale: false, Age: time.Since(c.fetchedAt)}, nil
	}

	val, err := fetch()
	if err == nil {
		c.value = val
		c.fetchedAt = time.Now()
		c.hasValue = true
		return poolResult[T]{Value: val, Stale: false}, nil
	}

	if c.hasValue {
		age := time.Since(c.fetchedAt)
		if age < freePoolMaxStale {
			return poolResult[T]{Value: c.value, Stale: true, Age: age}, nil
		}
	}
	var zero T
	return poolResult[T]{Value: zero}, err
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /mnt/e/Users/limli/Documents/GitHub/nofx && go test ./provider/nofxos/ -run "TestPoolCache" -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd /mnt/e/Users/limli/Documents/GitHub/nofx
git add provider/nofxos/free_cache.go provider/nofxos/free_cache_test.go
git commit -m "feat(nofxos): add TTL pool cache with 2h stale-serve"
```

---

### Task 6: Add stale-aware cached getters to FreeTrendingClient

**Files:**
- Modify: `provider/nofxos/free.go`
- Modify: `provider/nofxos/free_test.go`
- Test: `provider/nofxos/free_cache_integration_test.go` (create)

**Interfaces:**
- Consumes: `poolCache`, `poolResult` from Task 5.
- Produces new methods, each returning `poolResult[...]`:
  - `GetAI500Cached() (poolResult[[]CoinData], error)`
  - `GetOITopCached(limit int) (poolResult[[]OIPosition], error)`, `GetOILowCached(limit int) (poolResult[[]OIPosition], error)`
  - `GetNetFlowTopCached(limit int) (poolResult[[]NetFlowPosition], error)`, `GetNetFlowLowCached(limit int) (poolResult[[]NetFlowPosition], error)`
  - `GetPriceTopCached(limit int) (poolResult[[]PriceRankingItem], error)`, `GetPriceLowCached(limit int) (poolResult[[]PriceRankingItem], error)`
  - `GetOIDataCached(duration string, limit int) (poolResult[*OIDataEnvelope], error)`, `GetNetflowDataCached(duration string, limit int) (poolResult[*NetflowEnvelope], error)`, `GetPriceDataCached(duration string, limit int) (poolResult[*PriceEnvelope], error)`
- Existing uncached methods (`GetAI500()` etc.) remain and keep their signatures.

- [ ] **Step 1: Write the failing integration test**

```go
func TestGetAI500Cached_ServesStaleOnFetchFailure(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.Write([]byte(`{"category":{"assets":[{"symbol":"BR","pair":"BRUSDT","score":75}]}}`))
			return
		}
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte("<html>Just a moment...</html>"))
	}))
	defer srv.Close()

	c := NewFreeTrendingClient()
	c.baseURL = srv.URL
	c.http = srv.Client()

	r1, err := c.GetAI500Cached()
	if err != nil || r1.Stale || len(r1.Value) != 1 {
		t.Fatalf("first: %+v err=%v", r1, err)
	}
	c.ai500Cache.fetchedAt = time.Now().Add(-30 * time.Minute)

	r2, err := c.GetAI500Cached()
	if err != nil {
		t.Fatalf("expected stale serve, got %v", err)
	}
	if !r2.Stale || len(r2.Value) != 1 || r2.Value[0].Pair != "BRUSDT" {
		t.Fatalf("expected stale BRUSDT, got %+v", r2)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /mnt/e/Users/limli/Documents/GitHub/nofx && go test ./provider/nofxos/ -run TestGetAI500Cached_ServesStaleOnFetchFailure -v`
Expected: FAIL — undefined: `GetAI500Cached`, `ai500Cache`.

- [ ] **Step 3: Add cache fields and cached getters**

Add to `FreeTrendingClient` struct:
```go
	ai500Cache  poolCache[[]CoinData]
	oiTopCache  poolCache[[]OIPosition]
	oiLowCache  poolCache[[]OIPosition]
	nfTopCache  poolCache[[]NetFlowPosition]
	nfLowCache  poolCache[[]NetFlowPosition]
	pxTopCache  poolCache[[]PriceRankingItem]
	pxLowCache  poolCache[[]PriceRankingItem]
	oidCache    poolCache[*OIDataEnvelope]
	nfdCache    poolCache[*NetflowEnvelope]
	pxdCache    poolCache[*PriceEnvelope]
```

Add methods (same file), e.g.:
```go
func (c *FreeTrendingClient) GetAI500Cached() (poolResult[[]CoinData], error) {
	return c.ai500Cache.get(func() ([]CoinData, error) { return c.GetAI500() })
}

func (c *FreeTrendingClient) GetOITopCached(limit int) (poolResult[[]OIPosition], error) {
	return c.oiTopCache.get(func() ([]OIPosition, error) { return c.GetOITop(limit) })
}

func (c *FreeTrendingClient) GetOILowCached(limit int) (poolResult[[]OIPosition], error) {
	return c.oiLowCache.get(func() ([]OIPosition, error) { return c.GetOILow(limit) })
}

func (c *FreeTrendingClient) GetNetFlowTopCached(limit int) (poolResult[[]NetFlowPosition], error) {
	return c.nfTopCache.get(func() ([]NetFlowPosition, error) { return c.GetNetFlowTop(limit) })
}

func (c *FreeTrendingClient) GetNetFlowLowCached(limit int) (poolResult[[]NetFlowPosition], error) {
	return c.nfLowCache.get(func() ([]NetFlowPosition, error) { return c.GetNetFlowLow(limit) })
}

func (c *FreeTrendingClient) GetPriceTopCached(limit int) (poolResult[[]PriceRankingItem], error) {
	return c.pxTopCache.get(func() ([]PriceRankingItem, error) { return c.GetPriceTop(limit) })
}

func (c *FreeTrendingClient) GetPriceLowCached(limit int) (poolResult[[]PriceRankingItem], error) {
	return c.pxLowCache.get(func() ([]PriceRankingItem, error) { return c.GetPriceLow(limit) })
}

func (c *FreeTrendingClient) GetOIDataCached(duration string, limit int) (poolResult[*OIDataEnvelope], error) {
	return c.oidCache.get(func() (*OIDataEnvelope, error) { return c.GetOIData(duration, limit) })
}

func (c *FreeTrendingClient) GetNetflowDataCached(duration string, limit int) (poolResult[*NetflowEnvelope], error) {
	return c.nfdCache.get(func() (*NetflowEnvelope, error) { return c.GetNetflowData(duration, limit) })
}

func (c *FreeTrendingClient) GetPriceDataCached(duration string, limit int) (poolResult[*PriceEnvelope], error) {
	return c.pxdCache.get(func() (*PriceEnvelope, error) { return c.GetPriceData(duration, limit) })
}
```

Import `"time"` in the integration test file for `time.Now()`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /mnt/e/Users/limli/Documents/GitHub/nofx && go test ./provider/nofxos/ -run "TestGetAI500Cached|TestPoolCache" -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd /mnt/e/Users/limli/Documents/GitHub/nofx
git add provider/nofxos/free.go provider/nofxos/free_test.go provider/nofxos/free_cache_integration_test.go
git commit -m "feat(nofxos): stale-aware cached getters for all free pools"
```

---

### Task 7: Propagate staleness through kernel candidate getters

**Files:**
- Modify: `kernel/engine.go:866-972`
- Test: `kernel/engine_free_test.go` (append)

**Interfaces:**
- Consumes: `GetAI500Cached`, `GetOITopCached`, `GetOILowCached`, `GetNetFlowTopCached`, `GetNetFlowLowCached`, `GetPriceTopCached`, `GetPriceLowCached` (Task 6).
- Produces:
  - New `StrategyEngine` fields `lastPoolStale bool` and `lastPoolWarning string`, with accessor:
    - `func (e *StrategyEngine) ConsumePoolWarning() string` — returns and clears the accumulated warning text for this cycle.
  - `getAI500Coins`, `getOITopCoins`, `getOILowCoins`, `getNetflowTopCoins`, `getNetflowLowCoins`, `getPriceTopCoins`, `getPriceLowCoins` switch to the cached variants and, when `Stale`, set `e.lastPoolWarning` to `"Using cached <source> pool (age <age>) — live fetch failed"`.

- [ ] **Step 1: Write the failing test**

```go
func TestGetAI500CoinsMarksStale(t *testing.T) {
	t.Setenv("ALLOW_LOCAL_CUSTOM_API", "1")
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.Write([]byte(`{"category":{"assets":[{"symbol":"BR","pair":"BRUSDT","score":75}]}}`))
			return
		}
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	tr := nofxos.NewFreeTrendingClient()
	tr.SetBaseURL(srv.URL)
	engine := &StrategyEngine{trending: tr}

	if coins, err := engine.getAI500Coins(10); err != nil || len(coins) != 1 {
		t.Fatalf("first: %v %d", err, len(coins))
	}
	if w := engine.ConsumePoolWarning(); w != "" {
		t.Fatalf("expected no warning on fresh fetch, got %q", w)
	}
	tr.ForceStaleForTest(30 * time.Minute) // test helper added in Step 3

	if coins, err := engine.getAI500Coins(10); err != nil || len(coins) != 1 {
		t.Fatalf("stale: %v %d", err, len(coins))
	}
	if w := engine.ConsumePoolWarning(); w == "" {
		t.Fatalf("expected stale warning")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /mnt/e/Users/limli/Documents/GitHub/nofx && go test ./kernel/ -run TestGetAI500CoinsMarksStale -v`
Expected: FAIL — undefined `ConsumePoolWarning`, `ForceStaleForTest`.

- [ ] **Step 3: Add test helper, engine fields, and switch getters**

In `provider/nofxos/free.go`, add a test-only helper:
```go
func (c *FreeTrendingClient) ForceStaleForTest(age time.Duration) {
	c.ai500Cache.mu.Lock()
	c.ai500Cache.fetchedAt = time.Now().Add(-age)
	c.ai500Cache.mu.Unlock()
}
```

In `kernel/engine.go`, add fields near the other engine state and the accessor:
```go
	lastPoolStale   bool
	lastPoolWarning string
```
```go
func (e *StrategyEngine) ConsumePoolWarning() string {
	w := e.lastPoolWarning
	e.lastPoolWarning = ""
	return w
}
```

Rewrite each getter, e.g.:
```go
func (e *StrategyEngine) getAI500Coins(limit int) ([]CandidateCoin, error) {
	if limit <= 0 {
		limit = 30
	}
	res, err := e.trending.GetAI500Cached()
	if err != nil {
		return nil, err
	}
	if res.Stale {
		e.lastPoolWarning = fmt.Sprintf("Using cached ai500 pool (age %s) — live fetch failed", res.Age.Round(time.Minute))
		logger.Warnf("⚠️ %s", e.lastPoolWarning)
	}
	var candidates []CandidateCoin
	for _, c := range res.Value {
		symbol := market.Normalize(c.Pair)
		candidates = append(candidates, CandidateCoin{Symbol: symbol, Sources: []string{"ai500"}})
	}
	return candidates, nil
}
```

Apply the analogous change to `getOITopCoins`, `getOILowCoins`, `getNetflowTopCoins`, `getNetflowLowCoins`, `getPriceTopCoins`, `getPriceLowCoins` using their respective `*Cached` methods and source labels (`oi_top`, `oi_low`, `netflow_top`, `netflow_low`, `price_top`, `price_low`).

Add `"time"` and `"fmt"` to `kernel/engine.go` imports if not already present (verify with `goimports`/build).

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /mnt/e/Users/limli/Documents/GitHub/nofx && go test ./kernel/ -run "TestGetAI500CoinsMarksStale|TestGetCandidateCoins" -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd /mnt/e/Users/limli/Documents/GitHub/nofx
git add kernel/engine.go kernel/engine_free_test.go provider/nofxos/free.go
git commit -m "feat(kernel): propagate stale pool flag through candidate getters"
```

---

### Task 8: Route prompt-path fetchers through the cache

**Files:**
- Modify: `kernel/engine_analysis.go:218-265`
- Test: `kernel/engine_analysis_test.go` (append)

**Interfaces:**
- Consumes: `GetAI500Cached`, `GetOIDataCached`, `GetNetflowDataCached`, `GetPriceDataCached` (Task 6).
- Produces: prompt enrichment uses stale values on fetch failure (with warning) instead of omitting; per-coin empty entries remain omitted.

- [ ] **Step 1: Write the failing test**

```go
func TestAttachPerCoinSignalsUsesStaleAI500(t *testing.T) {
	t.Setenv("ALLOW_LOCAL_CUSTOM_API", "1")
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.Write([]byte(`{"category":{"assets":[{"symbol":"BR","pair":"BRUSDT","score":75}]}}`))
			return
		}
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	tr := nofxos.NewFreeTrendingClient()
	tr.SetBaseURL(srv.URL)
	engine := &StrategyEngine{trending: tr, config: &store.StrategyConfig{
		Indicators: store.IndicatorConfig{EnableAI500Data: true},
	}}
	// warm cache
	_ = engine.getAI500Coins(10)
	tr.ForceStaleForTest(30 * time.Minute)

	ctx := &Context{CandidateCoins: []CandidateCoin{{Symbol: "BRUSDT"}}}
	if err := AttachPerCoinSignals(ctx, engine); err != nil {
		t.Fatalf("attach: %v", err)
	}
	sig := engine.GetPerCoinSignal("BRUSDT")
	if sig == nil || sig.AI500 == nil {
		t.Fatalf("expected stale AI500 signal attached, got %+v", sig)
	}
}
```

Confirm the exact accessor name for per-coin signals (`GetPerCoinSignal`) via `grep -n "func (e \*StrategyEngine) GetPerCoinSignal" kernel/*.go` before writing the test; if the name differs, use the existing accessor.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /mnt/e/Users/limli/Documents/GitHub/nofx && go test ./kernel/ -run TestAttachPerCoinSignalsUsesStaleAI500 -v`
Expected: FAIL — stale signal omitted (current code warns and skips on error).

- [ ] **Step 3: Switch the three fetch sites to cached variants**

In `kernel/engine_analysis.go`:
- AI500 block: replace `coins, err := engine.trending.GetAI500()` with `res, err := engine.trending.GetAI500Cached()`; use `res.Value`; if `res.Stale` log the warning but still attach.
- OI/netflow/price duration blocks: replace `engine.trending.GetOIData/GetNetflowData/GetPriceData` with `...Cached`, use `res.Value.Top/Low`, and on `err == nil` (stale or fresh) populate the map. Keep the existing `else { logger.Warnf(...) }` for genuine errors (no cache).

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /mnt/e/Users/limli/Documents/GitHub/nofx && go test ./kernel/ -run "TestAttachPerCoinSignals|TestGetCandidateCoins" -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd /mnt/e/Users/limli/Documents/GitHub/nofx
git add kernel/engine_analysis.go kernel/engine_analysis_test.go
git commit -m "feat(kernel): serve stale per-coin prompt data on fetch failure"
```

---

### Task 9: Trader loop manages positions when pool is unavailable

**Files:**
- Modify: `trader/auto_trader_loop.go:86-102`
- Test: `trader/auto_trader_loop_test.go` (append)

**Interfaces:**
- Consumes: `at.strategyEngine.GetCandidateCoins()`, `ctx.Positions`, `kernel.GetFullDecisionWithStrategy`.
- Produces: cycle no longer early-returns when `len(ctx.CandidateCoins)==0 && len(ctx.Positions)>0`; appends execution-log warnings from the pool cache.

- [ ] **Step 1: Write the failing test**

Since `runCycle` is heavy, test the extracted decision function. Extract the early-return predicate into a pure helper:
```go
func shouldSkipForNoCandidates(candidateCount, positionCount int) bool {
	return candidateCount == 0 && positionCount == 0
}
```

```go
func TestShouldSkipForNoCandidates(t *testing.T) {
	cases := []struct {
		candidates, positions int
		want                  bool
	}{
		{0, 0, true},
		{0, 1, false},
		{3, 0, false},
		{3, 2, false},
	}
	for _, c := range cases {
		if got := shouldSkipForNoCandidates(c.candidates, c.positions); got != c.want {
			t.Fatalf("candidates=%d positions=%d: got %v want %v", c.candidates, c.positions, got, c.want)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /mnt/e/Users/limli/Documents/GitHub/nofx && go test ./trader/ -run TestShouldSkipForNoCandidates -v`
Expected: FAIL — undefined: `shouldSkipForNoCandidates`.

- [ ] **Step 3: Replace the early-return block**

Replace `trader/auto_trader_loop.go:86-102` with:
```go
	// If no candidate coins AND no open positions, nothing to manage: skip.
	if shouldSkipForNoCandidates(len(ctx.CandidateCoins), len(ctx.Positions)) {
		at.logInfof("ℹ️ No candidate coins available, skipping this cycle")
		record.Success = true
		record.ExecutionLog = append(record.ExecutionLog, "No candidate coins available, cycle skipped")
		record.AccountState = store.AccountSnapshot{
			TotalBalance:          ctx.Account.TotalEquity,
			AvailableBalance:      ctx.Account.AvailableBalance,
			TotalUnrealizedProfit: ctx.Account.UnrealizedPnL,
			PositionCount:         ctx.Account.PositionCount,
			InitialBalance:        at.initialBalance,
		}
		if err := at.saveDecision(record); err != nil {
			at.logWarnf("⚠ Failed to save decision record: %v", err)
		}
		return nil
	}

	// Candidate pool unavailable (fetch failed and cache expired) but positions
	// are open: still call the LLM to manage them (positions-only context).
	if len(ctx.CandidateCoins) == 0 && len(ctx.Positions) > 0 {
		msg := fmt.Sprintf("⚠️ Candidate pool unavailable; managing %d existing position(s) only (no new positions)", len(ctx.Positions))
		at.logWarnf(msg)
		record.ExecutionLog = append(record.ExecutionLog, msg)
	} else if w := at.strategyEngine.ConsumePoolWarning(); w != "" {
		record.ExecutionLog = append(record.ExecutionLog, "⚠️ "+w)
	}
```

Add the helper near other pure helpers:
```go
func shouldSkipForNoCandidates(candidateCount, positionCount int) bool {
	return candidateCount == 0 && positionCount == 0
}
```

- [ ] **Step 4: Add the empty-candidates prompt instruction**

In `kernel/prompt_builder.go` or `kernel/engine_prompt.go` where the candidate section is built (`engine_prompt.go:915` region), when `len(ctx.CandidateCoins)==0` write: `"No candidate pool is available this cycle; manage existing positions only.\n\n"`. Locate the exact block with `grep -n "formatCandidateCoins\|Candidate Coins" kernel/engine_prompt.go` and insert the conditional immediately before the candidate loop.

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd /mnt/e/Users/limli/Documents/GitHub/nofx && go test ./trader/ -run TestShouldSkipForNoCandidates -v && go test ./kernel/ -count=1`
Expected: PASS.

- [ ] **Step 6: Build and commit**

```bash
cd /mnt/e/Users/limli/Documents/GitHub/nofx
go build ./...
git add trader/auto_trader_loop.go trader/auto_trader_loop_test.go kernel/prompt_builder.go kernel/engine_prompt.go
git commit -m "feat(trader): manage open positions when candidate pool unavailable"
```

---

### Task 10: Documentation and env template

**Files:**
- Modify: `.env.example`
- Modify: `data-alt-endpoints.md`
- Modify: `docs/superpowers/specs/2026-09-19-altfins-and-vergex-per-coin-data-sources-design.md` (access caveat)

**Interfaces:**
- Consumes: env vars `VERGEX_HTTP_MODE`, `VERGEX_CF_CLEARANCE`.
- Produces: documentation only.

- [ ] **Step 1: Add env vars to `.env.example`**

Append:
```
# Free vergex.trade transport mode: fingerprint (default) | curl | stdlib
VERGEX_HTTP_MODE=fingerprint

# Optional Cloudflare clearance cookie (IP- and User-Agent-bound; do not commit a real value)
# VERGEX_CF_CLEARANCE=
```

- [ ] **Step 2: Update `data-alt-endpoints.md` ai500 row and add a Cloudflare note**

Add a note under the endpoint table: vergex.trade now returns a Cloudflare managed challenge to non-browser TLS fingerprints; the backend uses a Chrome-fingerprint transport with curl fallback (`VERGEX_HTTP_MODE`), and free candidate pools are cached with a 15-minute fresh window and 2-hour stale-serve.

- [ ] **Step 3: Update the 2026-09-19 spec's Access caveat**

Replace the "may or may not be challenged" uncertainty with the confirmed finding (managed challenge observed 2026-09-23; mitigated by fingerprint transport) and reference this design's spec.

- [ ] **Step 4: Commit**

```bash
cd /mnt/e/Users/limli/Documents/GitHub/nofx
git add .env.example data-alt-endpoints.md docs/superpowers/specs/2026-09-19-altfins-and-vergex-per-coin-data-sources-design.md
git commit -m "docs: document vergex transport modes and stale cache behavior"
```

---

### Task 11: Server verification (manual, required)

**Files:** none (verification only).

- [ ] **Step 1: Build on the server**

Run on the server:
```bash
cd <nofx-dir> && go build -o nofx ./...
```

- [ ] **Step 2: Probe the transport directly**

Create a throwaway probe (do not commit):
```bash
cat > /tmp/vergex_probe.go <<'EOF'
package main

import (
	"fmt"
	"io"
	"net/http"
	"nofx/security"
)

func main() {
	d, err := security.NewFreeHTTPClient(30 * 1e9)
	if err != nil {
		panic(err)
	}
	req, _ := http.NewRequest(http.MethodGet, "https://vergex.trade/trending-category?lang=en&key=ai500", nil)
	security.SetBrowserHeaders(req)
	resp, err := d.Do(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	fmt.Println("status:", resp.StatusCode, "bytes:", len(b))
	if resp.StatusCode == 200 {
		fmt.Println("OK:", string(b[:min(120, len(b))]))
	}
}

func min(a, b int) int { if a < b { return a }; return b }
EOF
cd <nofx-dir> && go run /tmp/vergex_probe.go
```
Expected: `status: 200` and a JSON snippet beginning `{"generatedAt":...`.

- [ ] **Step 3: Run the AI500 trader and confirm candidate logs**

Start the AI500 trader; confirm the log shows candidate coins fetched and no `No candidate coins available` while a live fetch succeeds. Then simulate an outage (block vergex.trade or wait through a challenge) and confirm the execution log shows `⚠️ Using cached ai500 pool (age …) — live fetch failed` and that the LLM still runs when positions are open.

- [ ] **Step 4: Report results**

If `fingerprint` mode still yields 403 on the server, run the probe with `VERGEX_HTTP_MODE=curl` and confirm 200; capture both outputs for follow-up.

---

## Self-Review

**Spec coverage:**
- Layer 1 transport (fingerprint + curl fallback, env, headers, cookie, SSRF) → Tasks 1–2, wired in 3–4. ✓
- Layer 2 cache (15m fresh, 2h stale, fill-only-on-success, all pools) → Tasks 5–6, propagated 7–8. ✓
- Layer 3 loop (empty candidates + positions → LLM; warning in execution_log; per-coin/LLM failure behavior preserved) → Task 9. ✓
- Docs/env → Task 10. ✓
- Server verification → Task 11. ✓
- Non-goals (deterministic fallback, CF solving, frontend changes) → not planned. ✓

**Placeholder scan:** the only intentionally-deferred symbol checks (e.g. `tlsclient.NewNoopLogger`, `GetPerCoinSignal`, candidate-section line) include exact `grep`/`go doc` commands to confirm before writing, rather than vague "handle it".

**Type consistency:** `poolResult[T]`/`poolCache[T]` defined in Task 5 used verbatim in Task 6; `*Cached` method names in Task 6 match consumption in Tasks 7–8; `shouldSkipForNoCandidates` defined and used in Task 9; `ConsumePoolWarning` defined in Task 7 and called in Task 9.

## Known Risk Points

- `github.com/bogdanfinn/tls-client` API surface (logger symbol, profile const name `Chrome_133`) may differ by version; Task 2 Step 4 includes a `go doc` confirmation step.
- `tlsAdapter` must satisfy `security.HTTPDoer` (`Do(*http.Request) (*http.Response, error)`); the tls-client method signature matches `http.Client.Do`, but verify with `go build` in Task 2 Step 6.
