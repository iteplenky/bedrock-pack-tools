package franchise

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ---------- request ----------

func TestFranchiseRequest_OKReturnsBody(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "MCToken test" {
			t.Errorf("Authorization = %q, want %q", got, "MCToken test")
		}
		if got := r.Header.Get("User-Agent"); got != "libhttpclient/1.0.0.0" {
			t.Errorf("User-Agent = %q, want libhttpclient/1.0.0.0", got)
		}
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	defer ts.Close()

	body, err := request(context.Background(), http.MethodGet, ts.URL, nil, "MCToken test")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if string(body) != `{"ok":true}` {
		t.Errorf("body = %q, want %q", body, `{"ok":true}`)
	}
}

func TestFranchiseRequest_AuthRejected(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(status)
		}))
		_, err := request(context.Background(), http.MethodGet, ts.URL, nil, "MCToken x")
		ts.Close()
		if !errors.Is(err, ErrAuthRejected) {
			t.Errorf("status %d: err = %v, want ErrAuthRejected", status, err)
		}
	}
}

func TestFranchiseRequest_OtherStatusCarriesCode(t *testing.T) {
	for _, status := range []int{http.StatusNotFound, http.StatusInternalServerError, http.StatusBadGateway} {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(status)
			_, _ = io.WriteString(w, `{"namespace":"X","code":"Y","message":"z"}`)
		}))
		_, err := request(context.Background(), http.MethodGet, ts.URL, nil, "MCToken x")
		ts.Close()
		if got := httpStatusOf(err); got != status {
			t.Errorf("httpStatusOf = %d, want %d (err: %v)", got, status, err)
		}
	}
}

func TestFranchiseRequest_POSTSetsContentType(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", got)
		}
		body, _ := io.ReadAll(r.Body)
		if string(body) != `{"k":"v"}` {
			t.Errorf("body = %q, want %q", body, `{"k":"v"}`)
		}
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	defer ts.Close()

	_, err := request(context.Background(), http.MethodPost, ts.URL, []byte(`{"k":"v"}`), "MCToken x")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
}

func TestFranchiseRequest_GETOmitsContentType(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Content-Type"); got != "" {
			t.Errorf("Content-Type = %q on GET, want empty", got)
		}
		_, _ = io.WriteString(w, `{}`)
	}))
	defer ts.Close()

	_, err := request(context.Background(), http.MethodGet, ts.URL, nil, "MCToken x")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
}

func TestFranchiseRequest_BodyLimit(t *testing.T) {
	// Server sends bodyLimit+1024 bytes; reader should cap at the limit.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(make([]byte, bodyLimit+1024))
	}))
	defer ts.Close()

	body, err := request(context.Background(), http.MethodGet, ts.URL, nil, "MCToken x")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if len(body) != bodyLimit {
		t.Errorf("body len = %d, want %d (cap)", len(body), bodyLimit)
	}
}

// ---------- decodeAPIError ----------

func TestDecodeAPIError_StructuredMessage(t *testing.T) {
	err := decodeAPIError(404, []byte(`{"namespace":"ServiceRuntime","code":"InputError","message":"No matching target found."}`))
	got := err.Error()
	want := "HTTP 404 (InputError): No matching target found."
	if got != want {
		t.Errorf("Error()\n  got: %s\n  want: %s", got, want)
	}
}

func TestDecodeAPIError_FallbackToRaw(t *testing.T) {
	err := decodeAPIError(500, []byte(`<html>oops</html>`))
	if !strings.HasPrefix(err.Error(), "HTTP 500: ") {
		t.Errorf("expected HTTP 500 prefix, got %q", err.Error())
	}
	if !strings.Contains(err.Error(), "oops") {
		t.Errorf("expected raw body to leak through, got %q", err.Error())
	}
}

// ---------- previewBody ----------

func TestPreviewBody(t *testing.T) {
	cases := []struct {
		name string
		in   []byte
		want string
	}{
		{"empty", []byte{}, ""},
		{"short", []byte("hello"), "hello"},
		{"exact", []byte(strings.Repeat("x", 240)), strings.Repeat("x", 240)},
		{"truncated", []byte(strings.Repeat("y", 500)), strings.Repeat("y", 240) + "..."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := previewBody(tc.in); got != tc.want {
				t.Errorf("previewBody(%d bytes) len = %d, want %d", len(tc.in), len(got), len(tc.want))
			}
		})
	}
}

// ---------- decodeAPI ----------

func TestDecodeAPI_OK(t *testing.T) {
	type payload struct {
		Name string `json:"name"`
	}
	got, err := decodeAPI[apiResult[payload]]([]byte(`{"status":"OK","code":200,"result":{"name":"hi"}}`), "test")
	if err != nil {
		t.Fatalf("decodeAPI: %v", err)
	}
	if got.Result.Name != "hi" {
		t.Errorf("Result.Name = %q, want hi", got.Result.Name)
	}
}

func TestDecodeAPI_InvalidJSON(t *testing.T) {
	type payload struct{}
	_, err := decodeAPI[apiResult[payload]]([]byte(`{not json}`), "test")
	if err == nil {
		t.Fatal("expected decode error, got nil")
	}
	if !strings.Contains(err.Error(), "decode test response") {
		t.Errorf("err = %q, want label in message", err.Error())
	}
}
