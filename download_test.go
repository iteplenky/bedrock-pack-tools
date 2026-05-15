package main

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

// ---------- isZipFile ----------

func TestIsZipFile_ValidMagic(t *testing.T) {
	p := filepath.Join(t.TempDir(), "x.zip")
	if err := os.WriteFile(p, []byte{'P', 'K', 0x03, 0x04, 'r', 'e', 's', 't'}, 0644); err != nil {
		t.Fatal(err)
	}
	if !isZipFile(p) {
		t.Error("expected true for PK\\x03\\x04 prefix")
	}
}

func TestIsZipFile_HTML(t *testing.T) {
	p := filepath.Join(t.TempDir(), "x.html")
	if err := os.WriteFile(p, []byte("<html>error</html>"), 0644); err != nil {
		t.Fatal(err)
	}
	if isZipFile(p) {
		t.Error("expected false for HTML body — this is the broken-CDN case")
	}
}

func TestIsZipFile_TooShort(t *testing.T) {
	p := filepath.Join(t.TempDir(), "x.bin")
	if err := os.WriteFile(p, []byte{'P', 'K'}, 0644); err != nil {
		t.Fatal(err)
	}
	if isZipFile(p) {
		t.Error("expected false for <4 bytes")
	}
}

func TestIsZipFile_Missing(t *testing.T) {
	if isZipFile(filepath.Join(t.TempDir(), "nope")) {
		t.Error("expected false for missing file")
	}
}

// ---------- httpStatusErr.Retryable ----------

func TestHTTPStatusErr_Retryable(t *testing.T) {
	cases := []struct {
		code int
		want bool
	}{
		{http.StatusBadRequest, false},
		{http.StatusUnauthorized, false},
		{http.StatusForbidden, false},
		{http.StatusNotFound, false},
		{http.StatusGone, false},
		{http.StatusRequestTimeout, true},
		{http.StatusTooManyRequests, true},
		{http.StatusInternalServerError, true},
		{http.StatusBadGateway, true},
		{http.StatusServiceUnavailable, true},
		{http.StatusGatewayTimeout, true},
	}
	for _, tc := range cases {
		e := &httpStatusErr{code: tc.code}
		if got := e.Retryable(); got != tc.want {
			t.Errorf("code %d: Retryable()=%v, want %v", tc.code, got, tc.want)
		}
	}
}

// ---------- fetchToTemp ----------

func newTestTracker(t *testing.T, ctx context.Context) *downloadTracker {
	t.Helper()
	return &downloadTracker{
		ctx:        ctx,
		httpClient: &http.Client{Timeout: 5 * time.Second},
		outDir:     t.TempDir(),
	}
}

func TestFetchToTemp_Success(t *testing.T) {
	body := []byte{'P', 'K', 0x03, 0x04, 'h', 'i'}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write(body)
	}))
	defer srv.Close()

	tr := newTestTracker(t, context.Background())
	path, size, err := tr.fetchToTemp(srv.URL)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	defer os.Remove(path)

	if size != int64(len(body)) {
		t.Errorf("size: got %d, want %d", size, len(body))
	}
	got, _ := os.ReadFile(path)
	if !bytes.Equal(got, body) {
		t.Errorf("body mismatch: got %x, want %x", got, body)
	}
	// Temp file should live in outDir for atomic same-FS rename.
	if filepath.Dir(path) != tr.outDir {
		t.Errorf("temp file lives in %q, want under outDir %q", filepath.Dir(path), tr.outDir)
	}
}

func TestFetchToTemp_404NoRetry(t *testing.T) {
	withFastBackoff(t)

	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		http.Error(w, "nope", http.StatusNotFound)
	}))
	defer srv.Close()

	tr := newTestTracker(t, context.Background())
	_, _, err := tr.fetchToTemp(srv.URL)
	if err == nil {
		t.Fatal("expected error on 404")
	}
	var hse *httpStatusErr
	if !errors.As(err, &hse) || hse.code != http.StatusNotFound {
		t.Errorf("expected httpStatusErr{404}, got %T %v", err, err)
	}
	if got := attempts.Load(); got != 1 {
		t.Errorf("attempts: got %d, want 1 (404 must not retry)", got)
	}
}

func TestFetchToTemp_403NoRetry(t *testing.T) {
	withFastBackoff(t)

	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		http.Error(w, "denied", http.StatusForbidden)
	}))
	defer srv.Close()

	tr := newTestTracker(t, context.Background())
	_, _, err := tr.fetchToTemp(srv.URL)
	if err == nil {
		t.Fatal("expected error on 403")
	}
	if got := attempts.Load(); got != 1 {
		t.Errorf("attempts: got %d, want 1 (403 must not retry)", got)
	}
}

func TestFetchToTemp_500ThenSuccess(t *testing.T) {
	withFastBackoff(t)

	body := []byte{'P', 'K', 3, 4, 'd', 'a', 't', 'a'}
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := attempts.Add(1)
		if n < int32(cdnMaxRetries) {
			http.Error(w, "transient", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write(body)
	}))
	defer srv.Close()

	tr := newTestTracker(t, context.Background())
	path, size, err := tr.fetchToTemp(srv.URL)
	if err != nil {
		t.Fatalf("expected success after retry, got: %v", err)
	}
	defer os.Remove(path)

	if got := attempts.Load(); got != int32(cdnMaxRetries) {
		t.Errorf("attempts: got %d, want %d", got, cdnMaxRetries)
	}
	if size != int64(len(body)) {
		t.Errorf("size: got %d, want %d", size, len(body))
	}
}

func TestFetchToTemp_500ExhaustsRetries(t *testing.T) {
	withFastBackoff(t)

	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		http.Error(w, "permanent 5xx", http.StatusInternalServerError)
	}))
	defer srv.Close()

	tr := newTestTracker(t, context.Background())
	_, _, err := tr.fetchToTemp(srv.URL)
	if err == nil {
		t.Fatal("expected error after all retries")
	}
	if got := attempts.Load(); got != int32(cdnMaxRetries) {
		t.Errorf("attempts: got %d, want %d (full retry budget)", got, cdnMaxRetries)
	}
	var hse *httpStatusErr
	if !errors.As(err, &hse) || hse.code != http.StatusInternalServerError {
		t.Errorf("expected last-error httpStatusErr{500}, got %T %v", err, err)
	}
}

func TestFetchToTemp_429Retries(t *testing.T) {
	withFastBackoff(t)

	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		http.Error(w, "rate limited", http.StatusTooManyRequests)
	}))
	defer srv.Close()

	tr := newTestTracker(t, context.Background())
	_, _, err := tr.fetchToTemp(srv.URL)
	if err == nil {
		t.Fatal("expected error")
	}
	if got := attempts.Load(); got != int32(cdnMaxRetries) {
		t.Errorf("429 should retry: got %d attempts, want %d", got, cdnMaxRetries)
	}
}

func TestFetchToTemp_CtxCancelDuringBackoff(t *testing.T) {
	// Use a longer backoff so we have a clear window to cancel inside.
	old := cdnInitialBackoff
	cdnInitialBackoff = 200 * time.Millisecond
	t.Cleanup(func() { cdnInitialBackoff = old })

	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		http.Error(w, "5xx", http.StatusInternalServerError)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	tr := newTestTracker(t, ctx)

	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	_, _, err := tr.fetchToTemp(srv.URL)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
	if got := attempts.Load(); got >= int32(cdnMaxRetries) {
		t.Errorf("ctx-cancel should stop early: got %d attempts (cdnMaxRetries=%d)", got, cdnMaxRetries)
	}
}

// ---------- fetchOnce ----------

func TestFetchOnce_NonOKReturnsTypedErr(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "gone", http.StatusGone)
	}))
	defer srv.Close()

	tr := newTestTracker(t, context.Background())
	_, _, err := tr.fetchOnce(srv.URL)
	var hse *httpStatusErr
	if !errors.As(err, &hse) {
		t.Fatalf("expected *httpStatusErr, got %T: %v", err, err)
	}
	if hse.code != http.StatusGone {
		t.Errorf("got code %d, want 410", hse.code)
	}
	if hse.Retryable() {
		t.Error("410 should not be retryable")
	}
}

func TestFetchOnce_CtxCancelMidBody(t *testing.T) {
	// Server hangs the body forever; cancelling ctx must abort the read.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	tr := newTestTracker(t, ctx)

	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	_, _, err := tr.fetchOnce(srv.URL)
	if err == nil {
		t.Fatal("expected error from cancelled body read")
	}
}

// ---------- helpers ----------

func withFastBackoff(t *testing.T) {
	t.Helper()
	old := cdnInitialBackoff
	cdnInitialBackoff = 1 * time.Millisecond
	t.Cleanup(func() { cdnInitialBackoff = old })
}
