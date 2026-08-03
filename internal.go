package warmbly

import (
	"context"
	"net/url"
	"strconv"
	"time"
)

// bodyVerb is the shape of the client's POST/PATCH/PUT/DELETE-with-body helpers,
// so the generic request helpers below can be pointed at any of them.
type bodyVerb func(ctx context.Context, path string, body, out any, opts ...RequestOption) (*Response, error)

// fetch GETs path and decodes the body into a freshly allocated T.
func fetch[T any](ctx context.Context, c *Client, path string, opts []RequestOption) (*T, *Response, error) {
	out := new(T)
	resp, err := c.get(ctx, path, out, opts...)
	if err != nil {
		return nil, resp, err
	}
	return out, resp, nil
}

// fetchSlice GETs path and decodes a bare JSON array into a []T. It normalizes
// a JSON null to an empty slice so callers can range over the result
// unconditionally.
func fetchSlice[T any](ctx context.Context, c *Client, path string, opts []RequestOption) ([]T, *Response, error) {
	var out []T
	resp, err := c.get(ctx, path, &out, opts...)
	if err != nil {
		return nil, resp, err
	}
	if out == nil {
		out = []T{}
	}
	return out, resp, nil
}

// dataEnvelope is the {"data": [...]} wrapper used by the unpaginated list
// endpoints.
type dataEnvelope[T any] struct {
	Data []T `json:"data"`
}

// fetchData GETs path and unwraps a {"data": [...]} envelope into a []T.
func fetchData[T any](ctx context.Context, c *Client, path string, opts []RequestOption) ([]T, *Response, error) {
	var env dataEnvelope[T]
	resp, err := c.get(ctx, path, &env, opts...)
	if err != nil {
		return nil, resp, err
	}
	if env.Data == nil {
		env.Data = []T{}
	}
	return env.Data, resp, nil
}

// sendData is [fetchData] for a request that carries a body.
func sendData[T any](ctx context.Context, c *Client, verb bodyVerb, path string, body any, opts []RequestOption) ([]T, *Response, error) {
	var env dataEnvelope[T]
	resp, err := verb(ctx, path, body, &env, opts...)
	if err != nil {
		return nil, resp, err
	}
	if env.Data == nil {
		env.Data = []T{}
	}
	return env.Data, resp, nil
}

// send calls verb with body and decodes the response into a freshly allocated T.
func send[T any](ctx context.Context, c *Client, verb bodyVerb, path string, body any, opts []RequestOption) (*T, *Response, error) {
	out := new(T)
	resp, err := verb(ctx, path, body, out, opts...)
	if err != nil {
		return nil, resp, err
	}
	return out, resp, nil
}

// sendSlice is [send] for endpoints that answer with a bare JSON array.
func sendSlice[T any](ctx context.Context, c *Client, verb bodyVerb, path string, body any, opts []RequestOption) ([]T, *Response, error) {
	var out []T
	resp, err := verb(ctx, path, body, &out, opts...)
	if err != nil {
		return nil, resp, err
	}
	if out == nil {
		out = []T{}
	}
	return out, resp, nil
}

// --- query-string helpers used by the list parameter types ---

func setNonEmpty(q url.Values, key, value string) {
	if value != "" {
		q.Set(key, value)
	}
}

func setPositive(q url.Values, key string, value int) {
	if value > 0 {
		q.Set(key, strconv.Itoa(value))
	}
}

func setBool(q url.Values, key string, value *bool) {
	if value != nil {
		q.Set(key, strconv.FormatBool(*value))
	}
}

func setTime(q url.Values, key string, value *time.Time) {
	if value != nil && !value.IsZero() {
		q.Set(key, value.UTC().Format(time.RFC3339))
	}
}

func setCSV(q url.Values, key string, values []string) {
	for _, v := range values {
		if v != "" {
			q.Add(key, v)
		}
	}
}

// Bool, Int, Float, String and Time return pointers to their arguments, for
// filling the optional fields of parameter structs inline:
//
//	params := &warmbly.EmailUpdateParams{Name: warmbly.String("Sales")}
func Bool(v bool) *bool { return &v }

// Int returns a pointer to v.
func Int(v int) *int { return &v }

// Int64 returns a pointer to v.
func Int64(v int64) *int64 { return &v }

// Float64 returns a pointer to v.
func Float64(v float64) *float64 { return &v }

// String returns a pointer to v.
func String(v string) *string { return &v }

// Time returns a pointer to v.
func Time(v time.Time) *time.Time { return &v }
