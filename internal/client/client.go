// Package client is a thin HTTP client for the OpenProject API v3.
//
// It speaks HAL+JSON, authenticates with an API key (Basic "apikey:<key>"),
// retries on 429/503 and turns OpenProject error payloads into *APIError.
// Responses are decoded into map[string]any: the HAL flattening lives in
// internal/hal so every resource type is handled by the same code path.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// APIPrefix is the path prefix of every API v3 endpoint.
const APIPrefix = "/api/v3"

// Client talks to a single OpenProject instance.
type Client struct {
	BaseURL string
	APIKey  string
	HTTP    *http.Client
	// UserAgent is sent on every request.
	UserAgent string
	// DryRun, when set, records writes instead of sending them; reads and
	// validation requests (forms, render) still reach the server.
	DryRun *Recorder
}

// Recorder collects the writes of a dry run.
type Recorder struct {
	Requests []Recorded
}

// Recorded is one write a dry run did not send.
type Recorded struct {
	Method string `json:"method"`
	Path   string `json:"path"`
	Query  string `json:"query,omitempty"`
	Body   any    `json:"body,omitempty"`
	File   string `json:"file,omitempty"`
}

// sendsInDryRun reports whether a request has no side effects: reads and
// the POSTs that only validate or render.
func sendsInDryRun(method, path string) bool {
	if method == http.MethodGet || method == http.MethodHead {
		return true
	}
	p := strings.SplitN(path, "?", 2)[0]
	return method == http.MethodPost && (strings.HasSuffix(p, "/form") || strings.Contains(p, "/render/"))
}

func (r *Recorder) record(c *Client, method, path string, query url.Values, body []byte, contentType, file string) []byte {
	rec := Recorded{Method: method, Path: strings.TrimPrefix(c.URL(path, nil), c.BaseURL), File: file}
	if len(query) > 0 {
		rec.Query = query.Encode()
	}
	if file == "" && len(body) > 0 {
		var v any
		if strings.Contains(contentType, "json") && json.Unmarshal(body, &v) == nil {
			rec.Body = v
		} else {
			rec.Body = string(body)
		}
	}
	r.Requests = append(r.Requests, rec)
	return []byte(`{"id":0}`)
}

// RequestTimeout bounds one request. It is generous because a full page
// of work packages (MaxPageSize, schemas embedded) takes half a minute on
// a busy server; an unreachable one still fails at connect time.
const RequestTimeout = 3 * time.Minute

// New builds a client for baseURL (e.g. https://op.example.com) using apiKey.
func New(baseURL, apiKey string) *Client {
	return &Client{
		BaseURL:   strings.TrimRight(baseURL, "/"),
		APIKey:    apiKey,
		HTTP:      &http.Client{Timeout: RequestTimeout},
		UserAgent: "opcli",
	}
}

// APIError is an error answered by OpenProject.
type APIError struct {
	Status     int
	Identifier string
	Message    string
	Details    []string
}

func (e *APIError) Error() string {
	msg := fmt.Sprintf("openproject: HTTP %d: %s", e.Status, e.Message)
	if len(e.Details) > 0 {
		msg += " (" + strings.Join(e.Details, "; ") + ")"
	}
	return msg
}

// IsNotFound reports whether err is a 404 from the API.
func IsNotFound(err error) bool {
	e, ok := err.(*APIError)
	return ok && e.Status == http.StatusNotFound
}

// IsConflict reports whether err is a 409 (stale lockVersion).
func IsConflict(err error) bool {
	e, ok := err.(*APIError)
	return ok && e.Status == http.StatusConflict
}

// URL resolves an API path ("/api/v3/..." or "work_packages/1") to an absolute URL.
func (c *Client) URL(path string, query url.Values) string {
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return path
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	if !strings.HasPrefix(path, APIPrefix) {
		path = APIPrefix + path
	}
	u := c.BaseURL + path
	if len(query) > 0 {
		sep := "?"
		if strings.Contains(u, "?") {
			sep = "&"
		}
		u += sep + query.Encode()
	}
	return u
}

// Get fetches path and decodes the JSON body.
func (c *Client) Get(ctx context.Context, path string, query url.Values) (map[string]any, error) {
	return c.JSON(ctx, http.MethodGet, path, query, nil)
}

// Post sends body as JSON and decodes the answer.
func (c *Client) Post(ctx context.Context, path string, body any) (map[string]any, error) {
	return c.JSON(ctx, http.MethodPost, path, nil, body)
}

// Patch sends body as JSON and decodes the answer.
func (c *Client) Patch(ctx context.Context, path string, body any) (map[string]any, error) {
	return c.JSON(ctx, http.MethodPatch, path, nil, body)
}

// Delete removes the resource at path.
func (c *Client) Delete(ctx context.Context, path string) error {
	_, err := c.JSON(ctx, http.MethodDelete, path, nil, nil)
	return err
}

// JSON performs a request with an optional JSON body and decodes a JSON
// object answer. Empty answers (204) yield a nil map.
func (c *Client) JSON(ctx context.Context, method, path string, query url.Values, body any) (map[string]any, error) {
	var payload []byte
	if body != nil {
		switch b := body.(type) {
		case []byte:
			payload = b
		case json.RawMessage:
			payload = b
		default:
			var err error
			if payload, err = json.Marshal(body); err != nil {
				return nil, fmt.Errorf("encoding request: %w", err)
			}
		}
	}
	data, _, err := c.Raw(ctx, method, path, query, payload, "application/json")
	if err != nil {
		return nil, err
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, nil
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	return out, nil
}

// Raw performs a request and returns the raw body and content type. It is
// the single point where auth, retries and error decoding happen.
func (c *Client) Raw(ctx context.Context, method, path string, query url.Values, body []byte, contentType string) ([]byte, string, error) {
	if c.DryRun != nil && !sendsInDryRun(method, path) {
		return c.DryRun.record(c, method, path, query, body, contentType, ""), "application/json", nil
	}
	target := c.URL(path, query)
	const attempts = 3
	for attempt := 1; ; attempt++ {
		var rdr io.Reader
		if body != nil {
			rdr = bytes.NewReader(body)
		}
		req, err := http.NewRequestWithContext(ctx, method, target, rdr)
		if err != nil {
			return nil, "", err
		}
		req.SetBasicAuth("apikey", c.APIKey)
		req.Header.Set("Accept", "application/hal+json, application/json;q=0.9, */*;q=0.8")
		req.Header.Set("User-Agent", c.UserAgent)
		if body != nil {
			req.Header.Set("Content-Type", contentType)
		}

		resp, err := c.HTTP.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return nil, "", ctx.Err()
			}
			return nil, "", fmt.Errorf("cannot reach OpenProject at %s (check the URL, the network or the VPN): %w", c.BaseURL, err)
		}
		data, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			return nil, "", fmt.Errorf("reading response: %w", err)
		}

		retryable := resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusServiceUnavailable
		if retryable && attempt < attempts {
			wait := time.Duration(attempt) * time.Second
			if s, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil && s > 0 && s < 30 {
				wait = time.Duration(s) * time.Second
			}
			select {
			case <-time.After(wait):
				continue
			case <-ctx.Done():
				return nil, "", ctx.Err()
			}
		}
		if resp.StatusCode >= 400 {
			return nil, "", decodeError(resp.StatusCode, data)
		}
		return data, resp.Header.Get("Content-Type"), nil
	}
}

func decodeError(status int, data []byte) error {
	e := &APIError{Status: status, Message: http.StatusText(status)}
	var body struct {
		ErrorIdentifier string `json:"errorIdentifier"`
		Message         string `json:"message"`
		Embedded        struct {
			Errors  []struct{ Message string } `json:"errors"`
			Details struct {
				Attribute string `json:"attribute"`
			} `json:"details"`
		} `json:"_embedded"`
	}
	if json.Unmarshal(data, &body) == nil && body.Message != "" {
		e.Identifier = body.ErrorIdentifier
		e.Message = body.Message
		for _, sub := range body.Embedded.Errors {
			e.Details = append(e.Details, sub.Message)
		}
		if a := body.Embedded.Details.Attribute; a != "" && len(e.Details) == 0 {
			e.Details = append(e.Details, "attribute: "+a)
		}
	} else if s := strings.TrimSpace(string(data)); s != "" && len(s) < 300 {
		e.Message = s
	}
	return e
}

// MaxPageSize is the page size Collect asks for when it pages through a
// whole collection: OpenProject's default ceiling (apiv3_max_page_size).
// A server configured lower answers with its own pageSize, which wins.
const MaxPageSize = 1000

// CollectParallel is how many pages Collect fetches at once once it knows
// the total.
const CollectParallel = 4

// PageFallback recovers the elements of a page whose request failed. It
// gets the page's query (offset and pageSize set) and the error, and
// returns the elements and the collection total; returning an error fails
// the whole Collect.
type PageFallback func(ctx context.Context, q url.Values, err error) ([]map[string]any, int, error)

// Collect pages through a collection endpoint until max elements (max <= 0:
// everything) and returns the elements plus the server-side total.
func (c *Client) Collect(ctx context.Context, path string, query url.Values, max int) ([]map[string]any, int, error) {
	return c.CollectWith(ctx, path, query, max, nil)
}

// CollectWith is Collect with a fallback for pages that fail. The first page
// gives the total; the rest are fetched CollectParallel at a time and
// returned in order, without elements repeated across pages (offset paging
// shifts when the collection changes meanwhile).
func (c *Client) CollectWith(ctx context.Context, path string, query url.Values, max int, fallback PageFallback) ([]map[string]any, int, error) {
	pageSize := MaxPageSize
	if max > 0 {
		pageSize = min(max, MaxPageSize)
	}
	page := func(offset, size int) ([]map[string]any, int, int, error) {
		q := url.Values{}
		for k, v := range query {
			q[k] = v
		}
		q.Set("pageSize", strconv.Itoa(size))
		q.Set("offset", strconv.Itoa(offset))
		res, err := c.Get(ctx, path, q)
		if err != nil {
			if fallback == nil || ctx.Err() != nil {
				return nil, 0, 0, err
			}
			elems, total, ferr := fallback(ctx, q, err)
			return elems, total, size, ferr
		}
		served := toInt(res["pageSize"])
		if served <= 0 || served > size {
			served = size
		}
		return Elements(res), toInt(res["total"]), served, nil
	}

	first, total, size, err := page(1, pageSize)
	if err != nil {
		return nil, 0, err
	}
	want := total
	if max > 0 {
		want = min(max, total)
	}
	pages := (want + size - 1) / size
	if pages < 1 {
		pages = 1
	}
	results := make([][]map[string]any, pages)
	results[0] = first
	if pages > 1 {
		errs := make([]error, pages)
		sem := make(chan struct{}, CollectParallel)
		done := make(chan struct{})
		for p := 2; p <= pages; p++ {
			go func(p int) {
				sem <- struct{}{}
				defer func() { <-sem; done <- struct{}{} }()
				results[p-1], _, _, errs[p-1] = page(p, size)
			}(p)
		}
		for p := 2; p <= pages; p++ {
			<-done
		}
		for _, err := range errs {
			if err != nil {
				return nil, 0, err
			}
		}
	}

	var out []map[string]any
	seen := map[any]bool{}
	for _, elems := range results {
		for _, el := range elems {
			if id, ok := el["id"]; ok && id != nil {
				if seen[id] {
					continue
				}
				seen[id] = true
			}
			out = append(out, el)
		}
	}
	if max > 0 && len(out) > max {
		out = out[:max]
	}
	return out, total, nil
}

// Elements extracts _embedded.elements from a HAL collection.
func Elements(res map[string]any) []map[string]any {
	emb, _ := res["_embedded"].(map[string]any)
	raw, _ := emb["elements"].([]any)
	out := make([]map[string]any, 0, len(raw))
	for _, e := range raw {
		if m, ok := e.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

// Upload posts a file as multipart (metadata + file) to an attachments
// collection path such as /api/v3/work_packages/42/attachments.
func (c *Client) Upload(ctx context.Context, path, file, description string) (map[string]any, error) {
	if c.DryRun != nil {
		c.DryRun.record(c, http.MethodPost, path, nil, nil, "", file)
		return map[string]any{"id": 0}, nil
	}
	content, err := os.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", file, err)
	}
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	meta := map[string]any{"fileName": filepath.Base(file)}
	if description != "" {
		meta["description"] = map[string]string{"raw": description}
	}
	metaJSON, _ := json.Marshal(meta)
	if err := mw.WriteField("metadata", string(metaJSON)); err != nil {
		return nil, err
	}
	fw, err := mw.CreateFormFile("file", filepath.Base(file))
	if err != nil {
		return nil, err
	}
	if _, err := fw.Write(content); err != nil {
		return nil, err
	}
	if err := mw.Close(); err != nil {
		return nil, err
	}
	data, _, err := c.Raw(ctx, http.MethodPost, path, nil, buf.Bytes(), mw.FormDataContentType())
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	return out, nil
}

// Download fetches a binary resource (e.g. an attachment's downloadLocation).
func (c *Client) Download(ctx context.Context, href string) ([]byte, error) {
	if !strings.HasPrefix(href, "http") && !strings.HasPrefix(href, APIPrefix) {
		href = c.BaseURL + href
	}
	data, _, err := c.Raw(ctx, http.MethodGet, href, nil, nil, "")
	return data, err
}

// WebURL returns the browser URL for a path such as /work_packages/42.
func (c *Client) WebURL(path string) string {
	return c.BaseURL + path
}

func toInt(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case json.Number:
		i, _ := n.Int64()
		return int(i)
	}
	return 0
}
