package http

import (
	"bytes"
	"embed"
	"io/fs"
	stdhttp "net/http"
	"time"

	"github.com/rotisserie/eris"
)

//go:embed static/*
var staticFiles embed.FS

var favicon []byte

func init() {
	data, err := staticFiles.ReadFile("static/favicon.ico")
	if err != nil {
		return
	}
	favicon = data
}

func faviconHandler(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	if len(favicon) == 0 {
		w.WriteHeader(stdhttp.StatusNotFound)
		return
	}

	reader := bytes.NewReader(favicon)
	w.Header().Set("Content-Type", "image/x-icon")
	stdhttp.ServeContent(w, r, "favicon.ico", time.Time{}, reader)
}

func newStaticAssetHandler() (stdhttp.Handler, error) {
	assets, err := fs.Sub(staticFiles, "static")
	if err != nil {
		return nil, eris.Wrap(err, "preparing static assets filesystem")
	}

	return stdhttp.StripPrefix("/static/", stdhttp.FileServer(stdhttp.FS(assets))), nil
}

func (s *Server) registerStaticRoute() {
	handler, err := newStaticAssetHandler()
	if err != nil {
		if s.logger != nil {
			s.logger.WithError(err).Error("registering static assets handler failed")
		}
		return
	}

	s.mux.Handle("GET /static/", handler)
	s.mux.Handle("HEAD /static/", handler)
}
