package server

import (
	"log"
	"net/http"
	ws "ffmpeg-webrtc/pkg/websocket"
	"text/template"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
)

func indexHandler(t *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		templ := t.Lookup("index.html")

		if templ == nil {
			http.Error(w, "Could not find template", http.StatusInternalServerError)
			return
		}

		host := r.Host

		if err := templ.Execute(w, host); err != nil {
			http.Error(w, "Could not execute template", http.StatusInternalServerError)
		}
	}
}

func wsHandler(room *ws.Room) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{}

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Println("could not upgrade connection to websocket.", err)
			return
		}

		clientID := r.URL.Query().Get("clientID")
		if clientID == "" {
			log.Println("clientID is required")
			return
		}

		client := ws.NewClient(conn, clientID, room)
		room.Register <- client

		go client.Read()
		go client.Write()
	}
}

func registerHandlers(mux *mux.Router, room *ws.Room) {
	indexTemplate := template.Must(template.ParseFiles("src/html/index.html"))
	mux.HandleFunc("/", indexHandler(indexTemplate))
	mux.HandleFunc("/ws", wsHandler(room))
}
