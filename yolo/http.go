package yolo

import (
	"context"
	"embed"
	"net"
	"net/http"
	"time"
)

//go:embed index.html
var indexFile embed.FS

// IndexData is the Go html template input for index.html
type IndexData struct {
	API string
}

func (s *Server) startHttpServer() {
	var mux http.ServeMux
	mux.Handle("/validator-order", http.StripPrefix("/tile", s.handleImgRequest(0)))
	mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		err := s.indexTempl.Execute(w, &IndexData{API: s.publicEndpoint})
		if err != nil {
			s.log.Error("failed to serve index.html page", "err", err)
		}
	}))

	s.srv = &http.Server{
		Addr:              s.listenAddr,
		Handler:           &mux,
		ReadTimeout:       time.Second * 10,
		ReadHeaderTimeout: time.Second * 10,
		WriteTimeout:      time.Second * 10,
		IdleTimeout:       time.Second * 10,
		MaxHeaderBytes:    10_000,
		BaseContext: func(net.Listener) context.Context {
			ctx, _ := context.WithTimeout(s.ctx, time.Second*7)
			return ctx
		},
	}

	go func() {
		err := s.srv.ListenAndServe()
		s.log.Error("http server listen error, shutting down app", "err", err)
		if err := s.Close(); err != nil {
			s.log.Error("error during shutdown", "err", err)
		}
	}()
}
