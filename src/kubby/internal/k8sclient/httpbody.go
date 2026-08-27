package k8sclient

import (
	"fmt"
	"io"
)

// readBoundedBody rejects a response before allocation when Content-Length is
// trustworthy, and still enforces the same ceiling for chunked/compressed
// bodies. The extra byte distinguishes an exact-limit response from overflow.
func readBoundedBody(body io.Reader, contentLength, maxBytes int64, label string) ([]byte, error) {
	if maxBytes <= 0 {
		return nil, fmt.Errorf("invalid %s response limit", label)
	}
	if contentLength > maxBytes {
		return nil, fmt.Errorf("%s response is too large (limit %d bytes)", label, maxBytes)
	}
	data, err := io.ReadAll(io.LimitReader(body, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("%s response is too large (limit %d bytes)", label, maxBytes)
	}
	return data, nil
}
