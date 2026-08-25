package driver

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestPVListerReloadsServiceAccountToken(t *testing.T) {
	tokenPath := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(tokenPath, []byte("first-token"), 0o600); err != nil {
		t.Fatal(err)
	}
	var authorizations []string
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		authorizations = append(authorizations, request.Header.Get("Authorization"))
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"items":[]}`)),
			Header:     make(http.Header),
		}, nil
	})}
	lister := &apiPVLister{url: "https://kubernetes.test/api/v1/persistentvolumes", tokenPath: tokenPath, client: client, nodeID: "node-a"}

	if _, err := lister.List(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tokenPath, []byte("second-token"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := lister.List(context.Background()); err != nil {
		t.Fatal(err)
	}

	want := []string{"Bearer first-token", "Bearer second-token"}
	if !reflect.DeepEqual(authorizations, want) {
		t.Fatalf("Authorization headers = %v, want %v", authorizations, want)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}
