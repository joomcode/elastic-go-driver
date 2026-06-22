// Copyright 2012-present Oliver Eilhard. All rights reserved.
// Use of this source code is governed by a MIT-license.
// See http://olivere.mit-license.org/license.txt for details.

package elastic

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/pkg/errors"
)

// DefaultCompatibilityProbeTimeout is the per-host timeout used when probing
// Elasticsearch version during SetCompatibilityWithAuto. It is independent of
// the HTTP client timeout so auto-detect does not hang on unreachable seeds.
const DefaultCompatibilityProbeTimeout = 5 * time.Second

// SetCompatibilityWith enables Elasticsearch REST API compatibility headers.
// Pass the major version to declare compatibility with, e.g. 8 when talking
// to Elasticsearch 8.x or 9.x. Zero (the default) keeps plain application/json
// headers, which is appropriate for Elasticsearch 7.x clusters.
//
// See https://www.elastic.co/docs/reference/elasticsearch/rest-apis/compatibility
func SetCompatibilityWith(majorVersion int) ClientOptionFunc {
	return func(c *Client) error {
		if majorVersion < 0 {
			return fmt.Errorf("elastic: compatibility version must be >= 0")
		}
		c.compatibilityVersion = majorVersion
		c.compatibilityAutoDetect = false
		return nil
	}
}

// SetCompatibilityWithAuto probes the cluster at client startup and enables
// compatible-with=8 headers when the remote major version is 8 or higher.
// On Elasticsearch 7.x clusters, compatibility headers stay disabled.
//
// Use this for clients that may point at different cluster versions over time.
// For clients that always target Elasticsearch 7.x, omit this option (or pass
// SetCompatibilityWith(0) explicitly).
//
// SetCompatibilityWith takes precedence over auto-detect when passed with a
// value greater than zero.
func SetCompatibilityWithAuto() ClientOptionFunc {
	return func(c *Client) error {
		c.compatibilityAutoDetect = true
		return nil
	}
}

// SetCompatibilityProbeTimeout sets the per-host timeout for version probing
// performed by SetCompatibilityWithAuto. When zero, DefaultCompatibilityProbeTimeout
// is used. The timeout applies to each seed URL independently.
func SetCompatibilityProbeTimeout(timeout time.Duration) ClientOptionFunc {
	return func(c *Client) error {
		if timeout < 0 {
			return fmt.Errorf("elastic: compatibility probe timeout must be >= 0")
		}
		c.compatibilityProbeTimeout = timeout
		return nil
	}
}

func compatibilityMediaTypeJSON(version int) string {
	if version <= 0 {
		return "application/json"
	}
	return fmt.Sprintf("application/vnd.elasticsearch+json;compatible-with=%d", version)
}

func compatibilityMediaTypeNDJSON(version int) string {
	if version <= 0 {
		return "application/x-ndjson"
	}
	return fmt.Sprintf("application/vnd.elasticsearch+x-ndjson;compatible-with=%d", version)
}

func (c *Client) applyCompatibilityHeaders(req *Request, explicitContentType string) {
	v := c.compatibilityVersion
	if v <= 0 {
		return
	}

	req.Header.Set("Accept", compatibilityMediaTypeJSON(v))

	ct := explicitContentType
	if ct == "" {
		ct = req.Header.Get("Content-Type")
	}
	switch ct {
	case "application/x-ndjson":
		req.Header.Set("Content-Type", compatibilityMediaTypeNDJSON(v))
	default:
		req.Header.Set("Content-Type", compatibilityMediaTypeJSON(v))
	}
}

func parseElasticsearchMajorVersion(version string) (int, error) {
	var major int
	if _, err := fmt.Sscanf(version, "%d", &major); err != nil {
		return 0, err
	}
	return major, nil
}

func probeElasticsearchMajorVersion(ctx context.Context, httpClient Doer, url string, basicAuthUsername, basicAuthPassword string, headers http.Header) (int, error) {
	endpoint := strings.TrimRight(url, "/") + "/"
	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return 0, err
	}
	if basicAuthUsername != "" || basicAuthPassword != "" {
		req.SetBasicAuth(basicAuthUsername, basicAuthPassword)
	}
	for key, values := range headers {
		for _, v := range values {
			req.Header.Add(key, v)
		}
	}
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", "elastic/"+Version)
	}

	res, err := httpClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer res.Body.Close()

	var info struct {
		Version struct {
			Number string `json:"number"`
		} `json:"version"`
	}
	if err := json.NewDecoder(res.Body).Decode(&info); err != nil {
		return 0, err
	}
	return parseElasticsearchMajorVersion(info.Version.Number)
}

func (c *Client) resolveCompatibilityAuto(ctx context.Context) error {
	if !c.compatibilityAutoDetect || c.compatibilityVersion > 0 {
		return nil
	}

	c.mu.RLock()
	urls := c.urls
	headers := c.headers
	basicAuthUsername := c.basicAuthUsername
	basicAuthPassword := c.basicAuthPassword
	httpClient := c.c
	probeTimeout := c.compatibilityProbeTimeout
	c.mu.RUnlock()

	if len(urls) == 0 {
		urls = []string{DefaultURL}
	}
	if probeTimeout <= 0 {
		probeTimeout = DefaultCompatibilityProbeTimeout
	}

	var lastErr error
	for _, url := range urls {
		probeCtx, cancel := context.WithTimeout(ctx, probeTimeout)
		major, err := probeElasticsearchMajorVersion(probeCtx, httpClient, url, basicAuthUsername, basicAuthPassword, headers)
		cancel()
		if err != nil {
			lastErr = err
			continue
		}
		if major >= 8 {
			c.compatibilityVersion = 8
		}
		return nil
	}
	return errors.Wrap(lastErr, "elastic: detect Elasticsearch version for compatibility headers")
}
