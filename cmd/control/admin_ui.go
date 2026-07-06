package main

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed admin_ui/*
var adminUI embed.FS

func serveAdminUIRoutes(mux *http.ServeMux) {
	ui, err := fs.Sub(adminUI, "admin_ui")
	if err != nil {
		panic(err)
	}

	files := http.FileServer(http.FS(ui))

	mux.Handle("GET /admin", http.RedirectHandler("/admin/", http.StatusMovedPermanently))
	mux.Handle("GET /admin/", http.StripPrefix("/admin/", files))
}
