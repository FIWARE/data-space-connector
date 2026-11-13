package launcher

import (
	"net/url"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/FIWARE/data-space-connector/config-tool/internal/server"
)

func TestCommandNeverCarriesTheRunToken(t *testing.T) {
	srv, err := server.New(fstest.MapFS{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = srv.Close() })
	tokenURL, err := url.Parse(srv.URL())
	if err != nil {
		t.Fatal(err)
	}
	token := tokenURL.Query().Get("token")
	launchURL, err := srv.LaunchURL()
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		goos     string
		wantName string
	}{
		{"linux", "xdg-open"},
		{"freebsd", "xdg-open"},
		{"darwin", "open"},
		{"windows", "rundll32"},
	}
	for _, tc := range tests {
		t.Run(tc.goos, func(t *testing.T) {
			name, args, err := command(tc.goos, launchURL)
			if err != nil {
				t.Fatal(err)
			}
			if name != tc.wantName {
				t.Errorf("name = %q, want %q", name, tc.wantName)
			}
			if !slices.Contains(args, launchURL) {
				t.Errorf("argv %q does not contain the launch URL %q", args, launchURL)
			}
			for _, arg := range append(args, name) {
				if strings.Contains(arg, token) {
					t.Errorf("argv element %q contains the run token", arg)
				}
			}
		})
	}
}

func TestCommandUnsupportedOS(t *testing.T) {
	if _, _, err := command("plan9", "http://127.0.0.1:1/?launch=x"); err == nil {
		t.Error("error = nil, want an unsupported-OS error")
	}
}
