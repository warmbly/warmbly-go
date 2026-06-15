package warmbly

import (
	"context"
	"errors"
	"iter"
	"net/url"
	"strconv"
)

// ErrNoMorePages is returned by [Page.Next] when the current page is the last.
var ErrNoMorePages = errors.New("warmbly: no more pages")

// Pagination is the cursor-based pagination envelope returned by list
// endpoints. Cursors are opaque: never construct or parse them, just pass
// NextCursor back to fetch the following page.
type Pagination struct {
	// Total is the total number of matching items, when the API computes it.
	Total *int64 `json:"total"`
	// NextCursor is the opaque token for the next page, or nil on the last.
	NextCursor *string `json:"next_cursor"`
	// HasMore reports whether further pages exist.
	HasMore bool `json:"has_more"`
}

// ListOptions are the pagination controls common to every list endpoint.
// Resource-specific list parameter types embed it.
type ListOptions struct {
	// Limit is the maximum number of items per page (server-capped, typically
	// at 100). Zero uses the server default.
	Limit int
	// Cursor is the opaque pagination token from a previous page's NextCursor.
	Cursor string
}

func (o *ListOptions) apply(q url.Values) {
	if o == nil {
		return
	}
	if o.Limit > 0 {
		q.Set("limit", strconv.Itoa(o.Limit))
	}
	if o.Cursor != "" {
		q.Set("cursor", o.Cursor)
	}
}

// Page is one page of a list endpoint's results. Beyond Data and Pagination it
// can fetch subsequent pages ([Page.Next]) or iterate every item across all
// pages ([Page.All]).
type Page[T any] struct {
	// Data holds the items on this page.
	Data []T `json:"data"`
	// Pagination holds the cursor metadata for this page.
	Pagination Pagination `json:"pagination"`

	resp  *Response
	fetch func(ctx context.Context, cursor string) (*Page[T], error)
}

// Response returns the HTTP response that produced this page, including
// rate-limit metadata.
func (p *Page[T]) Response() *Response { return p.resp }

// HasMore reports whether a further page is available. It requires both the
// has_more flag and a non-empty cursor, so a server that reports has_more with
// an empty/null cursor cannot drive an infinite re-fetch loop.
func (p *Page[T]) HasMore() bool {
	return p.Pagination.HasMore && p.NextCursor() != ""
}

// NextCursor returns the opaque cursor for the next page, or "" if none.
func (p *Page[T]) NextCursor() string {
	if p.Pagination.NextCursor != nil {
		return *p.Pagination.NextCursor
	}
	return ""
}

// Next fetches the following page. It returns [ErrNoMorePages] when the current
// page is the last.
func (p *Page[T]) Next(ctx context.Context) (*Page[T], error) {
	if !p.HasMore() || p.fetch == nil {
		return nil, ErrNoMorePages
	}
	return p.fetch(ctx, p.NextCursor())
}

// All returns an iterator over every item across all pages, fetching
// subsequent pages on demand. Iteration stops at the first error, which is
// yielded with the zero value of T:
//
//	for campaign, err := range page.All(ctx) {
//		if err != nil {
//			return err
//		}
//		fmt.Println(campaign.Name)
//	}
func (p *Page[T]) All(ctx context.Context) iter.Seq2[T, error] {
	return func(yield func(T, error) bool) {
		page := p
		for {
			for i := range page.Data {
				if !yield(page.Data[i], nil) {
					return
				}
			}
			if !page.HasMore() || page.fetch == nil {
				return
			}
			next, err := page.fetch(ctx, page.NextCursor())
			if err != nil {
				var zero T
				yield(zero, err)
				return
			}
			page = next
		}
	}
}

// listJSON fetches one page from a list endpoint and wires up the closure used
// to fetch following pages. It is the generic backend for every service's List
// method (Go method sets cannot be generic, so this is a free function).
func listJSON[T any](ctx context.Context, c *Client, path string, query url.Values) (*Page[T], error) {
	u := path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	page := &Page[T]{}
	resp, err := c.get(ctx, u, page)
	if err != nil {
		return nil, err
	}
	page.resp = resp
	page.fetch = func(ctx context.Context, cursor string) (*Page[T], error) {
		q := cloneValues(query)
		q.Set("cursor", cursor)
		return listJSON[T](ctx, c, path, q)
	}
	return page, nil
}

func cloneValues(v url.Values) url.Values {
	out := make(url.Values, len(v))
	for k, vs := range v {
		out[k] = append([]string(nil), vs...)
	}
	return out
}
