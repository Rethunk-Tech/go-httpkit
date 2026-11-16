package download

import (
	"net/http"
	"net/url"
	"testing"
)

func TestDropAuthorizedDefaultPortKeepsHeaders(t *testing.T) {
	authorize := func(req *http.Request) {
		req.Header.Set("Authorization", "Bearer secret")
	}
	policy := dropAuthorized(authorize)

	tests := []struct {
		name, via, next string
		keep            bool
	}{
		{"https default port", "https://cdn.example.com/file", "https://cdn.example.com:443/file", true},
		{"http default port", "http://cdn.example.com/file", "http://cdn.example.com:80/file", true},
		{"other host", "https://cdn.example.com/file", "https://other.example.com/file", false},
		{"non-default port", "https://cdn.example.com/file", "https://cdn.example.com:444/file", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			viaURL, err := url.Parse(tt.via)
			if err != nil {
				t.Fatal(err)
			}
			reqURL, err := url.Parse(tt.next)
			if err != nil {
				t.Fatal(err)
			}
			req := &http.Request{URL: reqURL, Header: http.Header{"Authorization": []string{"Bearer secret"}}}
			if err := policy(req, []*http.Request{{URL: viaURL}}); err != nil {
				t.Fatal(err)
			}
			got := req.Header.Get("Authorization")
			if tt.keep && got != "Bearer secret" {
				t.Fatalf("Authorization = %q, want kept", got)
			}
			if !tt.keep && got != "" {
				t.Fatalf("Authorization = %q, want stripped", got)
			}
		})
	}
}
