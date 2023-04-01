package server

import (
	"context"
	"fmt"
	"net/http"
	"time"

	ws "ffmpeg-webrtc/pkg/websocket"

	"github.com/gorilla/mux"
)

type Server struct {
	Room *ws.Room
	Done chan bool
}

func (s *Server) StartServer() {
	//create a server instance
	server := &http.Server{
		Addr:         ":7000",
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
		Handler:      nil,
	}

	router := mux.NewRouter()
	server.Handler = router

	registerHandlers(router, s.Room)

	ctx, cancel := context.WithCancel(context.Background())

	serverErrors := make(chan error, 1)

	go func() {
		<-s.Done
		cancel()
	}()

	go func() {
		fmt.Println("server is ready to handle requests at", server.Addr)
		serverErrors <- server.ListenAndServe()
	}()

	select {
	case err := <-serverErrors:
		fmt.Println(err)
	case <-ctx.Done():
		fmt.Println("shutting down the server")
		server.Shutdown(ctx)
	}
}
