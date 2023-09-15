package webrtc

import (
	"fmt"
	"log"

	"github.com/gorilla/websocket"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v3"
)

type Client struct {
	id        string
	conn      *websocket.Conn
	send      chan []byte
	room      *Room
	Track     *webrtc.TrackLocalStaticRTP
	RTPSender *webrtc.RTPSender
	SSRC      webrtc.SSRC
	PC        *webrtc.PeerConnection
	Packets   chan *rtp.Packet
	done      chan bool
}

func NewClient(conn *websocket.Conn, clientID string, room *Room) *Client {
	client := Client{
		id:      clientID,
		conn:    conn,
		send:    make(chan []byte, 1),
		room:    room,
		Packets: make(chan *rtp.Packet, 240),
		done:    make(chan bool, 1),
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

func (c *Client) WriteRTP() {
	for {
		select {
		case packet := <-c.Packets:
			c.Track.WriteRTP(packet)
		case <-c.done:
			return
		}
	}
}

func (c *Client) ReadRTCP() {
	for {
		select {
		case <-c.done:
			return
		default:
			rtcpPackets, _, err := c.RTPSender.ReadRTCP()
			if err != nil {
				fmt.Println("could not read rtcp:", err)
				return
			}

			for _, packet := range rtcpPackets {
				switch packet.(type) {
				case *rtcp.ReceiverEstimatedMaximumBitrate:
					//implement
					fmt.Println("received remb")
				case *rtcp.PictureLossIndication:
					//implement
					fmt.Println("received pli")
				case *rtcp.TransportLayerNack:
					//implement
					fmt.Println("received nack")
				}
			}
		}
	}
}

func (c *Client) Send(msg []byte) {
	c.send <- msg
}

func (c *Client) Close() {
	c.conn.Close()
	close(c.done)
}

func (c *Client) Room() *Room {
	return c.room
}

func (c *Client) Conn() *websocket.Conn {
	return c.conn
}
