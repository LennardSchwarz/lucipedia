package http

import (
	"bytes"
	"context"

	"github.com/a-h/templ"
	"github.com/rotisserie/eris"
)

func renderComponent(ctx context.Context, component templ.Component) ([]byte, error) {
	var buf bytes.Buffer
	if err := component.Render(ctx, &buf); err != nil {
		return nil, eris.Wrap(err, "rendering component")
	}
	return buf.Bytes(), nil
}
