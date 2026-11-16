package launcher

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const testLaunchURL = "http://127.0.0.1:1234/?launch=code"

type countingLaunch struct{ calls int }

func (c *countingLaunch) LaunchURL() (string, error) {
	c.calls++
	return testLaunchURL, nil
}

func TestBootstrapHandler(t *testing.T) {
	tests := []struct {
		path      string
		status    int
		wantCodes int
	}{
		{"/", http.StatusOK, 1},
		{"/index.html", http.StatusOK, 1},
		{"/favicon.ico", http.StatusNotFound, 0},
		{"/assets/app.js", http.StatusNotFound, 0},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			src := &countingLaunch{}
			rec := httptest.NewRecorder()
			bootstrapHandler(src).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tt.path, nil))
			if rec.Code != tt.status {
				t.Errorf("status = %d, want %d", rec.Code, tt.status)
			}
			if src.calls != tt.wantCodes {
				t.Errorf("launch codes issued = %d, want %d", src.calls, tt.wantCodes)
			}
			refresh := `content="0;url=` + strings.ReplaceAll(testLaunchURL, "&", "&amp;")
			if got := strings.Contains(rec.Body.String(), refresh); got != (tt.status == http.StatusOK) {
				t.Errorf("body has meta refresh = %v, body %q", got, rec.Body.String())
			}
		})
	}
}
