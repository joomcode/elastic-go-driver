package elastic

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"
)

type preferredNodeRetryTransport func(*http.Request) (*http.Response, error)

func (f preferredNodeRetryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func newPreferredNodeRetryClient(t *testing.T, transport http.RoundTripper, retrier Retrier) *Client {
	t.Helper()
	client, err := NewSimpleClient(SetURL("http://removed.invalid:9200", "http://healthy.invalid:9200"),
		SetHttpClient(&http.Client{Transport: transport}), SetRetrier(retrier))
	if err != nil {
		t.Fatal(err)
	}
	client.conns[0].nodeID = "removed"
	client.conns[1].nodeID = "healthy"
	t.Cleanup(client.Stop)
	return client
}

func TestPreferredNodeRetryUsesHealthyNode(t *testing.T) {
	var attempts []string
	transport := preferredNodeRetryTransport(func(req *http.Request) (*http.Response, error) {
		attempts = append(attempts, req.URL.Hostname())
		if req.URL.Hostname() == "removed.invalid" {
			return nil, &net.DNSError{Err: "no such host", Name: req.URL.Hostname(), IsNotFound: true}
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header),
			Body: io.NopCloser(strings.NewReader(`{}`)), Request: req}, nil
	})
	client := newPreferredNodeRetryClient(t, transport, NewBackoffRetrier(NewSimpleBackoff(0, 0)))

	res, err := client.PerformRequest(context.Background(), PerformRequestOptions{
		Method: "POST", Path: "/products/_search", PreferNodeID: "removed",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected successful failover, got HTTP %d", res.StatusCode)
	}
	if want := []string{"removed.invalid", "healthy.invalid"}; !reflect.DeepEqual(attempts, want) {
		t.Fatalf("expected attempts %v, got %v", want, attempts)
	}
	if !client.conns[0].IsDead() {
		t.Fatal("expected failed preferred connection to be marked dead")
	}
}

func TestPreferredNodeRetryStopsAtRetryLimit(t *testing.T) {
	var attempts []string
	transport := preferredNodeRetryTransport(func(req *http.Request) (*http.Response, error) {
		attempts = append(attempts, req.URL.Hostname())
		return nil, &net.DNSError{Err: "no such host", Name: req.URL.Hostname(), IsNotFound: true}
	})
	client := newPreferredNodeRetryClient(t, transport, NewBackoffRetrier(NewSimpleBackoff(0, 0)))

	_, err := client.PerformRequest(context.Background(), PerformRequestOptions{
		Method: "POST", Path: "/products/_search", PreferNodeID: "removed",
	})
	var dnsErr *net.DNSError
	if !errors.As(err, &dnsErr) || !dnsErr.IsNotFound {
		t.Fatalf("expected DNS failure after retry exhaustion, got %v", err)
	}
	if want := []string{"removed.invalid", "healthy.invalid"}; !reflect.DeepEqual(attempts, want) {
		t.Fatalf("expected bounded attempts %v, got %v", want, attempts)
	}
}

func TestPreferredNodeHTTPErrorDoesNotRetry(t *testing.T) {
	for _, status := range []int{http.StatusBadRequest, http.StatusTooManyRequests, http.StatusServiceUnavailable} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			attempts, retries := 0, 0
			transport := preferredNodeRetryTransport(func(req *http.Request) (*http.Response, error) {
				attempts++
				return &http.Response{StatusCode: status, Header: make(http.Header),
					Body: io.NopCloser(strings.NewReader(`{"error":{"type":"test_error","reason":"rejected"}}`)), Request: req}, nil
			})
			retrier := RetrierFunc(func(context.Context, int, *http.Request, *http.Response, error) (time.Duration, bool, error) {
				retries++
				return 0, true, nil
			})
			client := newPreferredNodeRetryClient(t, transport, retrier)

			_, err := client.PerformRequest(context.Background(), PerformRequestOptions{
				Method: "POST", Path: "/products/_search", PreferNodeID: "removed",
			})
			var esErr *Error
			if !errors.As(err, &esErr) || esErr.Status != status {
				t.Fatalf("expected HTTP %d error, got %v", status, err)
			}
			if attempts != 1 || retries != 0 {
				t.Fatalf("expected one attempt without retries, got attempts=%d retries=%d", attempts, retries)
			}
			if client.conns[0].IsDead() {
				t.Fatal("HTTP error must not mark the preferred connection dead")
			}
		})
	}
}

func TestPreferredNodeContextCancellationDoesNotRetry(t *testing.T) {
	attempts, retries := 0, 0
	transport := preferredNodeRetryTransport(func(*http.Request) (*http.Response, error) {
		attempts++
		return nil, context.Canceled
	})
	retrier := RetrierFunc(func(context.Context, int, *http.Request, *http.Response, error) (time.Duration, bool, error) {
		retries++
		return 0, true, nil
	})
	client := newPreferredNodeRetryClient(t, transport, retrier)

	_, err := client.PerformRequest(context.Background(), PerformRequestOptions{
		Method: "POST", Path: "/products/_search", PreferNodeID: "removed",
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
	if attempts != 1 || retries != 0 {
		t.Fatalf("expected cancellation without retries, got attempts=%d retries=%d", attempts, retries)
	}
	if client.conns[0].IsDead() {
		t.Fatal("cancellation must not mark the preferred connection dead")
	}
}
