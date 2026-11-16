package httputil

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"testing"
)

func TestRedactRequestErrorRemovesEverySecretAndKeepsTheCause(t *testing.T) {
	const first, second = "first+secret&value", "second-secret"
	cause := &url.Error{Op: "Get", URL: "http://api.example.com/v1?key=" + url.QueryEscape(first), Err: context.DeadlineExceeded}
	err := fmt.Errorf("upstream echoed %q and %s: %w", first, second, cause)

	got := RedactRequestError(err, first, second)

	for _, secret := range []string{first, url.QueryEscape(first), second, "key="} {
		if strings.Contains(got.Error(), secret) {
			t.Errorf("redacted error still contains %q", secret)
		}
	}
	if !strings.Contains(got.Error(), "http://api.example.com/v1") {
		t.Errorf("redaction removed the endpoint a reader needs: %v", got)
	}
	if !errors.Is(got, context.DeadlineExceeded) {
		t.Error("redacted error no longer matches its cause")
	}
}

func TestRedactRequestErrorReturnsTheOriginalWhenNothingIsSecret(t *testing.T) {
	err := errors.New("connection refused")

	// Identity is the contract here: an error with nothing to redact is returned as is.
	if got := RedactRequestError(err, "", "absent-secret"); got != err { //nolint:errorlint // identity, not error matching, is under test
		t.Errorf("RedactRequestError wrapped an error with nothing to redact: %v", got)
	}
	if RedactRequestError(nil, "secret") != nil {
		t.Error("RedactRequestError(nil) returned an error")
	}
}
