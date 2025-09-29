package templates

import (
	"context"
	"io"

	"github.com/a-h/templ"
)

// RawHTML returns a templ component that writes the provided HTML without escaping.
func RawHTML(html string) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		_, err := io.WriteString(w, html)
		return err
	})
}
