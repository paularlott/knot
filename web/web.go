package web

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
)

var (
  //go:embed public_html/*
  publicHTML embed.FS
)

func Routes() chi.Router {
  router := chi.NewRouter()

  // Serve static content
  router.Get("/*", func(w http.ResponseWriter, r *http.Request) {
		rctx := chi.RouteContext(r.Context())
		pathPrefix := strings.TrimSuffix(rctx.RoutePattern(), "/*")

    fsys := fs.FS(publicHTML)
    contentStatic, _ := fs.Sub(fsys, "public_html")

		fs := http.StripPrefix(pathPrefix, http.FileServer(http.FS(contentStatic)))
		fs.ServeHTTP(w, r)
	})

  return router
}

