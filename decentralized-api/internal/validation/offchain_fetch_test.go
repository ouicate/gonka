package validation

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchPromptFromURL_Success(t *testing.T) {
	var gotValidator string
	var gotAuth string

	payload := `{"messages":[{"role":"user","content":"hello"}],"model":"gpt-3.5"}`
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotValidator = r.Header.Get("X-Validator-Address")
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("X-Prompt-Hash", "dummy-hash")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(payload))
	}))
	defer ts.Close()

	body, ok := fetchPromptFromURL(ts.URL, "inf-1", "validator-addr")
	if !ok {
		t.Fatalf("expected ok, got false")
	}
	if body != payload {
		t.Fatalf("unexpected body: %s", body)
	}
	if gotValidator != "validator-addr" || gotAuth != "validator-addr" {
		t.Fatalf("missing or incorrect headers: X-Validator-Address=%s Authorization=%s", gotValidator, gotAuth)
	}
}

func TestFetchPromptFromURL_Unauthorized(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer ts.Close()

	_, ok := fetchPromptFromURL(ts.URL, "inf-1", "validator-addr")
	if ok {
		t.Fatalf("expected not ok on 401")
	}
}

func TestFetchPromptFromURL_EmptyBody(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		// no body
	}))
	defer ts.Close()

	_, ok := fetchPromptFromURL(ts.URL, "inf-1", "validator-addr")
	if ok {
		t.Fatalf("expected not ok on empty body")
	}
}
