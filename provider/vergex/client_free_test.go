package vergex

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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
	c.freeHTTP = srv.Client()
	if _, err := c.doFreeGET(context.Background(), srv.URL+"/x"); err != nil {
		t.Fatalf("doFreeGET: %v", err)
	}
}
