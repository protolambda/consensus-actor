package fun

import (
	"embed"
	"fmt"
	"html/template"
	"net/http"
	"time"

	"github.com/ethereum/go-ethereum/log"
)

//go:embed index.html
var indexFile embed.FS

var indexTempl = func() *template.Template {
	indexTempl, err := template.ParseFS(indexFile, "index.html")
	if err != nil {
		panic(fmt.Errorf("failed to load index.html template: %v", err))
	}
	return indexTempl
}()

// IndexData is the Go html template input for index.html
type IndexData struct {
	Title string
	API   string
}

type Backend interface {
	HandleImageRequest()
}

func StartHttpServer(log log.Logger, listenAddr string, indexData *IndexData, handleImgRequest func(tileType uint8) http.Handler) *http.Server {
	var mux http.ServeMux
	mux.Handle("/validator-order", http.StripPrefix("/validator-order", handleImgRequest(0)))
	mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		err := indexTempl.Execute(w, indexData)
		if err != nil {
			log.Error("failed to serve index.html page", "err", err)
		}
	}))

	srv := &http.Server{
		Addr:              listenAddr,
		Handler:           &mux,
		ReadTimeout:       time.Second * 10,
		ReadHeaderTimeout: time.Second * 10,
		WriteTimeout:      time.Second * 10,
		IdleTimeout:       time.Second * 10,
		MaxHeaderBytes:    10_000,
	}

	go func() {
		err := srv.ListenAndServe()
		if err == nil || err == http.ErrServerClosed {
			log.Info("closed http server")
		} else {
			log.Error("http server listen error, shutting down app", "err", err)
		}
	}()

	return srv
}
