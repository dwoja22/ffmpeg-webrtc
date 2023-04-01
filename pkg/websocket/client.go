package ws

import (
	"log"

	"github.com/gorilla/websocket"
)

type Client struct {
	id   string
	conn *websocket.Conn
	send chan []byte
	room *Room
}

func NewClient(conn *websocket.Conn, clientID string, room *Room) *Client {
	client := Client{
		id:   clientID,
		conn: conn,
		send: make(chan []byte, 1),
		room: room,
	}

	return &client
}

func (c *Client) Read() {
	for {
		_, msg, err := c.conn.ReadMessage()
		if err != nil {
			log.Println(err)
			return
		}
		c.room.Broadcast <- msg
	}
}

func (c *Client) Write() {
	for {
		msg := <-c.send
		err := c.conn.WriteMessage(websocket.TextMessage, msg)
		if err != nil {
			return
		}
	}
}

func (c *Client) Send(msg []byte) {
	c.send <- msg
}

func (c *Client) Close() {
	c.conn.Close()
}

func (c *Client) Room() *Room {
	return c.room
}

func (c *Client) Conn() *websocket.Conn {
	return c.conn
}
