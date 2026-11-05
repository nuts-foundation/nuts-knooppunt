package fhirutil

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sync"

	fhirclient "github.com/SanteonNL/go-fhir-client"
)

var _ fhirclient.Client = (*CachingClient)(nil)

// CachingClient is a fhirclient.Client that remembers what it has read, so code that follows the same
// reference more than once reads it from the server once. It is meant to live for one unit of work,
// such as handling one request: it never expires anything on its own.
//
// Only reads are cached, by path, and only their outcomes that can't change by asking again: a
// success, or a 404/410. A read with a pre-request option (e.g. extra headers or query parameters)
// isn't cached, since the option may change the answer. Any write through the client empties the
// cache, so a read after it sees the change. Searches always go to the server.
type CachingClient struct {
	next  fhirclient.Client
	mux   sync.Mutex
	reads map[string]cachedRead
}

// cachedRead is a read's outcome, kept so it can be replayed to a later caller with the same options.
type cachedRead struct {
	data   []byte
	status int
	header http.Header
	err    error
}

// NewCachingClient returns a CachingClient that sends what it can't answer from its cache to next.
func NewCachingClient(next fhirclient.Client) *CachingClient {
	return &CachingClient{next: next, reads: map[string]cachedRead{}}
}

func (c *CachingClient) ReadWithContext(ctx context.Context, path string, target any, opts ...fhirclient.Option) error {
	for _, opt := range opts {
		if _, ok := opt.(fhirclient.PreRequestOption); ok {
			return c.next.ReadWithContext(ctx, path, target, opts...)
		}
	}
	c.mux.Lock()
	read, cached := c.reads[path]
	c.mux.Unlock()
	if !cached {
		var headers fhirclient.Headers
		read.err = c.next.ReadWithContext(ctx, path, &read.data, fhirclient.ResponseHeaders(&headers), fhirclient.ResponseStatusCode(&read.status))
		read.header = headers.Header
		if read.err == nil || read.status == http.StatusNotFound || read.status == http.StatusGone {
			c.mux.Lock()
			c.reads[path] = read
			c.mux.Unlock()
		}
	}
	return c.replay(read, target, opts)
}

// replay hands a read's outcome to the caller as the underlying client would have: post-request
// options see the response status and headers, then the body is unmarshalled into the target, then
// post-parse options run.
func (c *CachingClient) replay(read cachedRead, target any, opts []fhirclient.Option) error {
	if read.status != 0 {
		response := &http.Response{StatusCode: read.status, Header: read.header}
		if response.Header == nil {
			response.Header = http.Header{}
		}
		for _, opt := range opts {
			if fn, ok := opt.(fhirclient.PostRequestOption); ok {
				if err := fn(c, response); err != nil {
					return err
				}
			}
		}
	}
	if read.err != nil {
		return read.err
	}
	if target != nil {
		if raw, ok := target.(*[]byte); ok {
			*raw = append([]byte(nil), read.data...)
		} else if err := json.Unmarshal(read.data, target); err != nil {
			return fmt.Errorf("FHIR response unmarshal failed: %w", err)
		}
	}
	for _, opt := range opts {
		if fn, ok := opt.(fhirclient.PostParseOption); ok {
			if err := fn(c, target); err != nil {
				return err
			}
		}
	}
	return nil
}

// invalidate empties the cache: after a write, any cached read may be out of date.
func (c *CachingClient) invalidate() {
	c.mux.Lock()
	c.reads = map[string]cachedRead{}
	c.mux.Unlock()
}

func (c *CachingClient) Read(path string, target any, opts ...fhirclient.Option) error {
	return c.ReadWithContext(context.Background(), path, target, opts...)
}

func (c *CachingClient) SearchWithContext(ctx context.Context, resourceType string, query url.Values, target any, opts ...fhirclient.Option) error {
	return c.next.SearchWithContext(ctx, resourceType, query, target, opts...)
}

func (c *CachingClient) Search(resourceType string, query url.Values, target any, opts ...fhirclient.Option) error {
	return c.SearchWithContext(context.Background(), resourceType, query, target, opts...)
}

func (c *CachingClient) CreateWithContext(ctx context.Context, resource any, result any, opts ...fhirclient.Option) error {
	defer c.invalidate()
	return c.next.CreateWithContext(ctx, resource, result, opts...)
}

func (c *CachingClient) Create(resource any, result any, opts ...fhirclient.Option) error {
	return c.CreateWithContext(context.Background(), resource, result, opts...)
}

func (c *CachingClient) UpdateWithContext(ctx context.Context, path string, resource any, result any, opts ...fhirclient.Option) error {
	defer c.invalidate()
	return c.next.UpdateWithContext(ctx, path, resource, result, opts...)
}

func (c *CachingClient) Update(path string, resource any, result any, opts ...fhirclient.Option) error {
	return c.UpdateWithContext(context.Background(), path, resource, result, opts...)
}

func (c *CachingClient) DeleteWithContext(ctx context.Context, path string, opts ...fhirclient.Option) error {
	defer c.invalidate()
	return c.next.DeleteWithContext(ctx, path, opts...)
}

func (c *CachingClient) Delete(path string, opts ...fhirclient.Option) error {
	return c.DeleteWithContext(context.Background(), path, opts...)
}

func (c *CachingClient) Path(path ...string) *url.URL {
	return c.next.Path(path...)
}
