package http

import (
	"bytes"
	stdhttp "net/http"
	"time"

	_ "embed"
)

//go:embed static/favicon.ico
var favicon []byte

func faviconHandler(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	if len(favicon) == 0 {
		w.WriteHeader(stdhttp.StatusNotFound)
		return
	}

	reader := bytes.NewReader(favicon)
	w.Header().Set("Content-Type", "image/x-icon")
	stdhttp.ServeContent(w, r, "favicon.ico", time.Time{}, reader)
}
