package httputil

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// requestError preserves the original cause for errors.Is/As while exposing
// only the credential-safe message to logs and other error formatting.
type requestError struct {
	message string
	cause   error
}

func (e *requestError) Error() string { return e.message }
func (e *requestError) Unwrap() error { return e.cause }

// RedactRequestError returns err with request credentials removed from its
// message, for providers whose APIs authenticate through the URL. It strips
// the query string and user info from any *url.Error in the chain, then
// replaces each non-empty secret, in raw, JSON-escaped and query-escaped form,
// with REDACTED. The result still unwraps to err, so errors.Is and errors.As
// keep matching cancellation, deadlines and provider error classes. It returns
// err unchanged when nothing needed redacting.
func RedactRequestError(err error, secrets ...string) error {
	if err == nil {
		return nil
	}

	message := err.Error()
	var requestErr *url.Error
	if errors.As(err, &requestErr) {
		if requestURL, parseErr := url.Parse(requestErr.URL); parseErr == nil {
			requestURL.RawQuery = ""
			requestURL.ForceQuery = false
			requestURL.User = nil
			// url.Error quotes its URL; handle both that form and an outer
			// error that includes the URL without quotes.
			message = strings.ReplaceAll(message, fmt.Sprintf("%q", requestErr.URL), fmt.Sprintf("%q", requestURL.String()))
			message = strings.ReplaceAll(message, requestErr.URL, requestURL.String())
		}
	}
	for _, secret := range secrets {
		if secret == "" {
			continue
		}
		// API/proxy error bodies and malformed URLs can also echo the secret.
		encodedSecret, _ := json.Marshal(secret) // A string cannot fail JSON encoding.
		message = strings.ReplaceAll(message, string(encodedSecret[1:len(encodedSecret)-1]), "REDACTED")
		message = strings.ReplaceAll(message, url.QueryEscape(secret), "REDACTED")
		message = strings.ReplaceAll(message, secret, "REDACTED")
	}
	if message == err.Error() {
		return err
	}
	return &requestError{message: message, cause: err}
}
