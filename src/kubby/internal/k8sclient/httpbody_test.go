package k8sclient

import (
	"strings"
	"testing"
)

func TestReadBoundedBodyAcceptsTheLimitAndRejectsOverflow(t *testing.T) {
	data, err := readBoundedBody(strings.NewReader("12345"), -1, 5, "test")
	if err != nil || string(data) != "12345" {
		t.Fatalf("exact-limit response = %q, %v", data, err)
	}
	if _, err := readBoundedBody(strings.NewReader("123456"), -1, 5, "test"); err == nil {
		t.Fatal("chunked response above the limit was accepted")
	}
	if _, err := readBoundedBody(strings.NewReader(""), 6, 5, "test"); err == nil {
		t.Fatal("oversized Content-Length was accepted before reading")
	}
}
