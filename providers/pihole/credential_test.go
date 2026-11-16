package pihole

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/maxfield-allison/dnsweaver/pkg/provider"
)

const testV5Password = "test-password-with+reserved&characters"

// testV5Hash is the WEBPASSWORD Pi-hole v5 stores for testV5Password and
// accepts in the auth parameter, computed with Pi-hole's own method:
// printf %s "$password" | sha256sum | cut -d' ' -f1 | tr -d '\n' | sha256sum
const testV5Hash = "c45947ffc92517fee7aa3c5135d5e5fffc389fcb17dedf40c43d559972bba4a7"

func TestV5AuthUsesPiholeWebPasswordHash(t *testing.T) {
	client := NewAPIClient("http://pihole.example.com", testV5Password, "")

	if got := client.hashPassword(); got != testV5Hash {
		t.Errorf("v5 auth value = %s, want the WEBPASSWORD Pi-hole stores for this password", got)
	}
}

type failingTransport struct {
	err error
}

func (tr failingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, tr.err
}

// v5Operations exercises every public v5 method, so a new request path cannot
// skip redaction unnoticed.
func v5Operations(client *APIClient) map[string]func(context.Context) error {
	return map[string]func(context.Context) error{
		"ping": client.Ping,
		"list": func(ctx context.Context) error {
			_, err := client.List(ctx)
			return err
		},
		"create A": func(ctx context.Context) error {
			return client.Create(ctx, piholeRecord{Hostname: "app.example.com", Type: provider.RecordTypeA, Target: "10.0.0.1"})
		},
		"create CNAME": func(ctx context.Context) error {
			return client.Create(ctx, piholeRecord{Hostname: "www.example.com", Type: provider.RecordTypeCNAME, Target: "app.example.com"})
		},
		"delete A": func(ctx context.Context) error {
			return client.Delete(ctx, piholeRecord{Hostname: "app.example.com", Type: provider.RecordTypeA, Target: "10.0.0.1"})
		},
		"delete CNAME": func(ctx context.Context) error {
			return client.Delete(ctx, piholeRecord{Hostname: "www.example.com", Type: provider.RecordTypeCNAME, Target: "app.example.com"})
		},
	}
}

func assertNoV5Credential(t *testing.T, text string) {
	t.Helper()
	for _, secret := range []string{testV5Hash, testV5Password, "auth=" + testV5Hash} {
		if strings.Contains(text, secret) {
			t.Errorf("output exposes the Pi-hole v5 credential")
		}
	}
}

func TestV5FailedRequestErrorsDoNotContainPasswordHash(t *testing.T) {
	for _, cause := range []error{context.DeadlineExceeded, context.Canceled, errors.New("connection refused")} {
		client := NewAPIClient("http://pihole.example.com", testV5Password, "",
			WithHTTPClient(&http.Client{Transport: failingTransport{err: cause}}))
		for name, operation := range v5Operations(client) {
			t.Run(cause.Error()+"/"+name, func(t *testing.T) {
				var logs bytes.Buffer
				logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))

				err := operation(context.Background())
				logger.Warn("provider request failed", "error", err)

				if !errors.Is(err, cause) {
					t.Fatalf("request error does not preserve its cause: %v", err)
				}
				assertNoV5Credential(t, fmt.Sprintf("%+v", err))
				assertNoV5Credential(t, logs.String())
			})
		}
	}
}

func TestV5APIErrorResponsesDoNotContainPasswordHash(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("auth") != testV5Hash {
			t.Error("request no longer authenticates with the v5 auth parameter")
		}
		// A proxy or the API can echo the request URL, and so the credential, in an error body.
		w.WriteHeader(http.StatusBadGateway)
		_, _ = fmt.Fprintf(w, "upstream failed for %s", r.URL.String())
	}))
	defer server.Close()
	client := NewAPIClient(server.URL, testV5Password, "")

	for name, operation := range v5Operations(client) {
		t.Run(name, func(t *testing.T) {
			err := operation(context.Background())

			if err == nil {
				t.Fatal("expected an API error")
			}
			assertNoV5Credential(t, fmt.Sprintf("%+v", err))
		})
	}
}
