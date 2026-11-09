package web_test

import (
	"io/fs"
	"testing"

	"github.com/1995parham-me/didban/internal/infra/web"
)

func TestStaticHasThePage(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"index.html", "style.css", "app.js", "icon.svg"} {
		if _, err := fs.Stat(web.Static(), name); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}
