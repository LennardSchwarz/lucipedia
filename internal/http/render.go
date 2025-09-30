package http

import (
	"bytes"
	"context"
	"io"

	"github.com/a-h/templ"
	"github.com/rotisserie/eris"
)

func renderComponent(ctx context.Context, component templ.Component) ([]byte, error) {
	var buf bytes.Buffer
	if err := component.Render(ctx, &buf); err != nil {
		return nil, eris.Wrap(err, "error rendering component")
	}
	return buf.Bytes(), nil
}

func streamComponent(ctx context.Context, w io.Writer, component templ.Component) error {
	if err := component.Render(ctx, w); err != nil {
		return eris.Wrap(err, "error streaming component")
	}
	return nil
}
