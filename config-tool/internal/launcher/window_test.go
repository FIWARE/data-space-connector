package launcher

import (
	"os"
	"regexp"
	"testing"
)

const indexHTML = "../../ui/index.html"

var titleTag = regexp.MustCompile(`<title>([^<]*)</title>`)

func TestWindowTitleMatchesIndexHTML(t *testing.T) {
	page, err := os.ReadFile(indexHTML)
	if err != nil {
		t.Fatal(err)
	}
	m := titleTag.FindSubmatch(page)
	if m == nil {
		t.Fatalf("%s has no <title>", indexHTML)
	}
	if got := string(m[1]); got != WindowTitle {
		t.Fatalf("%s <title> = %q, want WindowTitle %q", indexHTML, got, WindowTitle)
	}
}
