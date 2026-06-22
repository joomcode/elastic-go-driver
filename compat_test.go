// Copyright 2012-present Oliver Eilhard. All rights reserved.
// Use of this source code is governed by a MIT-license.
// See http://olivere.mit-license.org/license.txt for details.

package elastic

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCompatibilityMediaTypes(t *testing.T) {
	if got, want := compatibilityMediaTypeJSON(0), "application/json"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if got, want := compatibilityMediaTypeJSON(8), "application/vnd.elasticsearch+json;compatible-with=8"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if got, want := compatibilityMediaTypeNDJSON(8), "application/vnd.elasticsearch+x-ndjson;compatible-with=8"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestApplyCompatibilityHeaders(t *testing.T) {
	client := &Client{compatibilityVersion: 8}

	req, err := NewRequest("GET", "/")
	if err != nil {
		t.Fatal(err)
	}
	client.applyCompatibilityHeaders(req, "")
	if got, want := req.Header.Get("Accept"), "application/vnd.elasticsearch+json;compatible-with=8"; got != want {
		t.Fatalf("Accept: got %q, want %q", got, want)
	}
	if got, want := req.Header.Get("Content-Type"), "application/vnd.elasticsearch+json;compatible-with=8"; got != want {
		t.Fatalf("Content-Type: got %q, want %q", got, want)
	}

	req, err = NewRequest("POST", "/_bulk")
	if err != nil {
		t.Fatal(err)
	}
	client.applyCompatibilityHeaders(req, "application/x-ndjson")
	if got, want := req.Header.Get("Content-Type"), "application/vnd.elasticsearch+x-ndjson;compatible-with=8"; got != want {
		t.Fatalf("Content-Type: got %q, want %q", got, want)
	}
}

func TestSetCompatibilityWithInvalid(t *testing.T) {
	if err := SetCompatibilityWith(-1)(&Client{}); err == nil {
		t.Fatal("expected error")
	}
}

func TestSetCompatibilityWithAutoElasticsearch7(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"version": map[string]string{"number": "7.17.13"},
		})
	}))
	defer srv.Close()

	client, err := NewSimpleClient(
		SetURL(srv.URL),
		SetSniff(false),
		SetHealthcheck(false),
		SetCompatibilityWithAuto(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if client.compatibilityVersion != 0 {
		t.Fatalf("compatibilityVersion = %d, want 0", client.compatibilityVersion)
	}
}

func TestSetCompatibilityWithAutoElasticsearch9(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"version": map[string]string{"number": "9.4.2"},
		})
	}))
	defer srv.Close()

	client, err := NewSimpleClient(
		SetURL(srv.URL),
		SetSniff(false),
		SetHealthcheck(false),
		SetCompatibilityWithAuto(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if client.compatibilityVersion != 8 {
		t.Fatalf("compatibilityVersion = %d, want 8", client.compatibilityVersion)
	}
}

func TestSetCompatibilityWithOverridesAuto(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"version": map[string]string{"number": "9.4.2"},
		})
	}))
	defer srv.Close()

	client, err := NewSimpleClient(
		SetURL(srv.URL),
		SetSniff(false),
		SetHealthcheck(false),
		SetCompatibilityWithAuto(),
		SetCompatibilityWith(0),
	)
	if err != nil {
		t.Fatal(err)
	}
	if client.compatibilityVersion != 0 {
		t.Fatalf("compatibilityVersion = %d, want 0", client.compatibilityVersion)
	}
	if client.compatibilityAutoDetect {
		t.Fatal("expected auto-detect to be disabled after explicit SetCompatibilityWith")
	}
}

func TestResolveCompatibilityAutoUsesContext(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"version": map[string]string{"number": "8.15.0"},
		})
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	client := &Client{
		c:                       http.DefaultClient,
		urls:                    []string{srv.URL},
		compatibilityAutoDetect: true,
	}
	if err := client.resolveCompatibilityAuto(ctx); err == nil {
		t.Fatal("expected context error")
	}
}

func TestResolveCompatibilityAutoProbeTimeout(t *testing.T) {
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer slow.Close()

	const probeTimeout = 100 * time.Millisecond
	client := &Client{
		c: &http.Client{
			Timeout: 20 * time.Minute,
		},
		urls: []string{
			slow.URL,
			slow.URL,
			"http://127.0.0.1:1",
		},
		compatibilityAutoDetect:   true,
		compatibilityProbeTimeout: probeTimeout,
	}

	start := time.Now()
	err := client.resolveCompatibilityAuto(context.Background())
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected error when all probes fail")
	}
	if elapsed >= 2*time.Second {
		t.Fatalf("probe took %v, expected about %v", elapsed, 3*probeTimeout)
	}
}

func TestSetCompatibilityWithAutoSkipsUnreachableHosts(t *testing.T) {
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer slow.Close()

	alive := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"version": map[string]string{"number": "9.4.2"},
		})
	}))
	defer alive.Close()

	start := time.Now()
	client, err := NewSimpleClient(
		SetURL(slow.URL, "http://127.0.0.1:1", alive.URL),
		SetSniff(false),
		SetHealthcheck(false),
		SetCompatibilityWithAuto(),
		SetCompatibilityProbeTimeout(200*time.Millisecond),
		SetHttpClient(&http.Client{Timeout: 20 * time.Minute}),
	)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatal(err)
	}
	if client.compatibilityVersion != 8 {
		t.Fatalf("compatibilityVersion = %d, want 8", client.compatibilityVersion)
	}
	if elapsed >= 2*time.Second {
		t.Fatalf("client creation took %v, expected well under 2s", elapsed)
	}
}
