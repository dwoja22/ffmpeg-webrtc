package webrtc

import (
	"ffmpeg-webrtc/pkg/h264"
	"fmt"
	"log"
	"time"

	"github.com/gorilla/websocket"
	"github.com/pion/interceptor/pkg/cc"
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
	SSRC      webrtc.SSRC
	RTPSender *webrtc.RTPSender
	PC        *webrtc.PeerConnection
	Estimator cc.BandwidthEstimator
	Packets   chan *rtp.Packet
	Frames    chan []byte
	done      chan bool
}

func NewClient(conn *websocket.Conn, clientID string, room *Room) *Client {
	client := Client{
		id:      clientID,
		conn:    conn,
		send:    make(chan []byte, 1),
		room:    room,
		Packets: make(chan *rtp.Packet, 240),
		Frames:  make(chan []byte, 240),
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
	payloader := h264.NewPayloader()
	packetizer := rtp.NewPacketizer(1460, 96, uint32(c.SSRC), payloader, rtp.NewRandomSequencer(), 90000)

	for {
		select {
		case packet := <-c.Packets:
			c.Track.WriteRTP(packet)
		case frame := <-c.Frames:
			packets := packetizer.Packetize(frame, uint32(160))
			for _, packet := range packets {
				c.Track.WriteRTP(packet)
			}
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
				case *rtcp.PictureLossIndication:
					fmt.Println("received pli")
				case *rtcp.TransportLayerNack:
					fmt.Println("received nack")
					fmt.Println(packet.(*rtcp.TransportLayerNack).Nacks)
				}
			}
		}
	}
}

func (c *Client) BandwidthEstimator() {
	ticker := time.NewTicker(100 * time.Millisecond)

	// Keep a table of powers to units for fast conversion.
	bitUnits := []string{"b", "Kb", "Mb", "Gb", "Tb", "Pb", "Eb"}

	// Do some unit conversions because b/s is far too difficult to read.
	powers := 0

	for {
		select {
		case <-ticker.C:
			bitrate := float64(c.Estimator.GetTargetBitrate())
			// Keep dividing the bitrate until it's under 1000
			for bitrate >= 1000.0 && powers < len(bitUnits) {
				bitrate /= 1000.0
				powers++
			}

			unit := bitUnits[powers]
			powers = 0

			fmt.Printf("client %v estimated available bandwidth: %.2f %s/s\n", c.id, bitrate, unit)
		case <-c.done:
			return
		}
	}
}

func (c *Client) Send(msg []byte) {
	c.send <- msg
}

func (c *Client) Stop() {
	if c.conn != nil {
		c.conn.Close()
	}

	select {
	case <-c.done:
		return
	default:
		close(c.done)
	}
}

func (c *Client) Room() *Room {
	return c.room
}

func (c *Client) Conn() *websocket.Conn {
	return c.conn
}
