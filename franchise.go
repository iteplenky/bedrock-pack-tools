package main

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

// franchiseClient gives the franchise-service calls their own timeout
// independent of any caller context. The shared http.DefaultClient has
// no timeout, so a stalled TCP read would hang the whole CLI.
var franchiseClient = &http.Client{Timeout: 30 * time.Second}

// franchiseBodyLimit caps the response body read so a runaway upstream
// payload can't pin the process. Real payloads are sub-100 KiB; 4 MiB
// is 40x headroom.
const franchiseBodyLimit = 4 << 20

// errAuthRejected is returned when a franchise endpoint rejects our
// MCToken (401/403). The caller is expected to drop its cached token,
// re-mint, and retry once; bubbling the raw 401 would leave the user
// stuck on a server-revoked but time-valid cache forever.
var errAuthRejected = errors.New("franchise: token rejected")

// apiData wraps responses that use the {status, code, data: ...} envelope.
// Observed on POST /discovery/blob/client. The status/code fields are
// informational; HTTP status is what we route on.
type apiData[T any] struct {
	Status string `json:"status"`
	Code   int    `json:"code"`
	Data   T      `json:"data"`
}

// apiResult wraps responses that use the {status, code, result: ...}
// envelope. Observed on /config/public, /venue/{id}, /join/experience.
// The franchise services are inconsistent about which key they use
// (even within the same HTTP method), so the right envelope is per-call
// and the caller picks based on observed shape.
type apiResult[T any] struct {
	Status string `json:"status"`
	Code   int    `json:"code"`
	Result T      `json:"result"`
}

// httpStatusErrAPI carries the underlying HTTP status code through
// [fmt.Errorf]-style wrapping so callers can switch on it without
// parsing the message string. Used by [franchiseRequest] for non-2xx
// responses that aren't 401/403.
type httpStatusErrAPI struct {
	code int
	body []byte
}

func (e *httpStatusErrAPI) Error() string {
	return decodeAPIError(e.code, e.body).Error()
}

// httpStatusOf returns the HTTP status code wrapped in err, or 0 if err
// doesn't carry one. Used by [joinExperience] to distinguish 404
// (offline) from any other transport failure.
func httpStatusOf(err error) int {
	var e *httpStatusErrAPI
	if errors.As(err, &e) {
		return e.code
	}
	return 0
}

// franchiseRequest is the one HTTP wrapper for every franchise call:
// MCToken auth header, standard libhttpclient User-Agent (matching the
// real Bedrock client so we don't stand out in Mojang's logs), body
// limit, status routing. Returns [errAuthRejected] on 401/403 so the
// caller can re-mint; returns [*httpStatusErrAPI] for other non-2xx so
// callers can inspect the code (used by [joinExperience] for 404).
func franchiseRequest(ctx context.Context, method, urlStr string, payload []byte, authHeader string) ([]byte, error) {
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

	resp, err := franchiseClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("franchise request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, franchiseBodyLimit))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	switch resp.StatusCode {
	case http.StatusOK:
		return body, nil
	case http.StatusUnauthorized, http.StatusForbidden:
		return nil, errAuthRejected
	default:
		return nil, &httpStatusErrAPI{code: resp.StatusCode, body: body}
	}
}

// decodeAPIError extracts a human-readable message from a franchise
// error body. The shape ({namespace, code, message, customData}) was
// observed from live 4xx/5xx responses; falls back to a truncated raw
// body if the JSON doesn't match.
func decodeAPIError(status int, body []byte) error {
	var apiErr struct {
		Namespace string `json:"namespace"`
		Code      string `json:"code"`
		Message   string `json:"message"`
	}
	if err := json.Unmarshal(body, &apiErr); err == nil && apiErr.Message != "" {
		return fmt.Errorf("HTTP %d (%s): %s", status, apiErr.Code, apiErr.Message)
	}
	return fmt.Errorf("HTTP %d: %s", status, previewBody(body))
}

// previewBody truncates a response body for inclusion in error messages.
// 240 bytes is enough to fit the JSON envelope plus a few of the fields
// inside without flooding the terminal on a multi-kilobyte response.
func previewBody(body []byte) string {
	const max = 240
	s := string(body)
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

// decodeAPI unmarshals a franchise response body into T with a uniform
// "decode <label> response: <err>" error. Callers pass the envelope type
// they expect ([apiData] for /discovery/blob/client, [apiResult] for
// everything else); the helper is purely a boilerplate eliminator.
func decodeAPI[T any](body []byte, label string) (T, error) {
	var v T
	if err := json.Unmarshal(body, &v); err != nil {
		return v, fmt.Errorf("decode %s response: %w", label, err)
	}
	return v, nil
}
