package franchise

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// httpClient gives franchise calls their own timeout. http.DefaultClient
// has none and would hang on a stalled TCP read.
var httpClient = &http.Client{Timeout: 30 * time.Second}

// bodyLimit caps response body reads. Real payloads are sub-100 KiB.
const bodyLimit = 4 << 20

// apiData wraps {status, code, data}. Observed on /discovery/blob/client.
type apiData[T any] struct {
	Data T `json:"data"`
}

// apiResult wraps {status, code, result}. Observed on /config/public,
// /venue/{id}, /join/experience.
type apiResult[T any] struct {
	Result T `json:"result"`
}

// httpStatusErr carries an HTTP status code through error wrapping so
// callers can switch on it without parsing the message.
type httpStatusErr struct {
	code int
	body []byte
}

func (e *httpStatusErr) Error() string { return decodeAPIError(e.code, e.body).Error() }

// httpStatusOf returns the HTTP status code wrapped in err, or 0.
func httpStatusOf(err error) int {
	var e *httpStatusErr
	if errors.As(err, &e) {
		return e.code
	}
	return 0
}

// request is the one HTTP wrapper for every franchise call: MCToken
// auth, libhttpclient UA (matching the real Bedrock client), body
// limit, status routing.
func request(ctx context.Context, method, urlStr string, payload []byte, authHeader string) ([]byte, error) {
	var bodyReader io.Reader
	if payload != nil {
		bodyReader = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, urlStr, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", authHeader)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Language", "en-US,en;q=0.5")
	req.Header.Set("User-Agent", "libhttpclient/1.0.0.0")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("franchise request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, bodyLimit))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	switch resp.StatusCode {
	case http.StatusOK:
		return body, nil
	case http.StatusUnauthorized, http.StatusForbidden:
		return nil, ErrAuthRejected
	default:
		return nil, &httpStatusErr{code: resp.StatusCode, body: body}
	}
}

// decodeAPIError extracts the {namespace, code, message} payload
// observed on live franchise 4xx/5xx, with raw-body fallback.
func decodeAPIError(status int, body []byte) error {
	var apiErr struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &apiErr); err == nil && apiErr.Message != "" {
		return fmt.Errorf("HTTP %d (%s): %s", status, apiErr.Code, apiErr.Message)
	}
	return fmt.Errorf("HTTP %d: %s", status, previewBody(body))
}

// previewBody truncates a response body to 240 bytes for error messages.
func previewBody(body []byte) string {
	const max = 240
	s := string(body)
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

// decodeAPI unmarshals a franchise body into T with a uniform error.
func decodeAPI[T any](body []byte, label string) (T, error) {
	var v T
	if err := json.Unmarshal(body, &v); err != nil {
		return v, fmt.Errorf("decode %s response: %w", label, err)
	}
	return v, nil
}
