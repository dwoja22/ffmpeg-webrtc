package server

import (
	"context"
	"ffmpeg-webrtc/pkg/webrtc"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/mux"
)

type Server struct {
	room *webrtc.Room
	done chan bool
}

func NewServer(room *webrtc.Room, done chan bool) *Server {
	return &Server{
		room: room,
		done: done,
	}
}

func (s *Server) Start() {
	//create a server instance
	server := &http.Server{
		Addr:         ":7000",
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
		Handler:      nil,
	}

	router := mux.NewRouter()
	server.Handler = router

	registerHandlers(router, s.room)

	ctx, cancel := context.WithCancel(context.Background())

	serverErrors := make(chan error, 1)

	go func() {
		<-s.done
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
