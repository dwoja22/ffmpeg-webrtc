package webrtc

import (
	"encoding/json"
	"ffmpeg-webrtc/pkg/h264"
	"fmt"

	"github.com/google/uuid"
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v3"
)

const (
	OFFER = iota
	ANSWER
	ICECANDIDATE
	STOP
)

type Room struct {
	Clients    map[string]*Client
	Broadcast  chan []byte
	Register   chan *Client
	Unregister chan *Client
	done       chan bool
}

func NewRoom(done chan bool) *Room {
	return &Room{
		Clients:    make(map[string]*Client),
		Broadcast:  make(chan []byte, 1),
		Register:   make(chan *Client, 1),
		Unregister: make(chan *Client, 1),
		done:       done,
	}
}

func (r *Room) Start() {
	var peerConnection *webrtc.PeerConnection

	for {
		select {
		case client := <-r.Register:
			fmt.Println("registering client with id: ", client.id)
			r.Clients[client.id] = client
		case client := <-r.Unregister:
			if _, ok := r.Clients[client.id]; ok {
				delete(r.Clients, client.id)
				close(client.send)
			}
		case msg := <-r.Broadcast:
			var m Message
			if err := json.Unmarshal(msg, &m); err != nil {
				fmt.Println(err)
				continue
			}

			client, exists := r.Clients[m.ClientID]
			if !exists {
				fmt.Println("client does not exist")
				continue
			}

			if m.Kind == OFFER {
				mediaEngine := webrtc.MediaEngine{}

				codec := webrtc.RTPCodecCapability{
					MimeType:    webrtc.MimeTypeH264,
					ClockRate:   90000,
					Channels:    0,
					SDPFmtpLine: "packetization-mode=1",
					RTCPFeedback: []webrtc.RTCPFeedback{
						{Type: "nack"},
						{Type: "nack", Parameter: "pli"},
						{Type: "ccm", Parameter: "fir"},
						{Type: "goog-remb"},
						{Type: "transport-cc"},
					},
				}

				err := mediaEngine.RegisterCodec(webrtc.RTPCodecParameters{RTPCodecCapability: codec, PayloadType: 96}, webrtc.RTPCodecTypeVideo)
				if err != nil {
					fmt.Println("error registering codec: ", err)
				}

				api := webrtc.NewAPI(webrtc.WithMediaEngine(&mediaEngine))

				peerConnection, err = api.NewPeerConnection(webrtc.Configuration{PeerIdentity: m.ClientID, ICEServers: []webrtc.ICEServer{{URLs: []string{"stun:stun.l.google.com:19302"}}}})
				if err != nil {
					fmt.Println("error creating peer connection: ", err)
					continue
				}

				client.PC = peerConnection

				r.HandlePeer(peerConnection, client.id)

				peerConnection.SetRemoteDescription(m.Offer)

				peerConnection.OnICECandidate(func(candidate *webrtc.ICECandidate) {
					if candidate == nil {
						return
					}

					msg := Message{
						ClientID:     client.id,
						Kind:         ICECANDIDATE,
						ICECandidate: candidate,
					}

					msgJSON, err := json.Marshal(msg)
					if err != nil {
						fmt.Println("error marshalling iceCandidate message: ", err)
						return
					}

					client.Send(msgJSON)
				})

				_, err = peerConnection.AddTransceiverFromKind(webrtc.RTPCodecTypeVideo, webrtc.RtpTransceiverInit{Direction: webrtc.RTPTransceiverDirectionSendonly})
				if err != nil {
					fmt.Println("error adding transceiver: ", err)
					continue
				}

				streamID := uuid.New().String()
				trackID := uuid.New().String()

				trackLocalStaticRTP, err := webrtc.NewTrackLocalStaticRTP(codec, streamID, trackID)
				if err != nil {
					fmt.Println("error creating rtp track: ", err)
				}

				rtpSender, err := peerConnection.AddTrack(trackLocalStaticRTP)
				if err != nil {
					fmt.Println("error adding rtp video track: ", err)
					continue
				}

				encoding := rtpSender.GetParameters().Encodings

				client.Track = trackLocalStaticRTP
				client.RTPSender = rtpSender
				client.SSRC = encoding[0].SSRC

				payloader := h264.NewPayloader()
				client.Packetizer = rtp.NewPacketizer(1400, 96, uint32(client.SSRC), payloader, rtp.NewRandomSequencer(), 90000)

				answer, err := peerConnection.CreateAnswer(nil)
				if err != nil {
					fmt.Println("error creating answer: ", err)
					continue
				}

				if err := peerConnection.SetLocalDescription(answer); err != nil {
					fmt.Println("error setting local description: ", err)
					continue
				}

				msg := Message{
					ClientID: client.id,
					Kind:     ANSWER,
					Answer:   answer,
				}

				msgJSON, err := json.Marshal(msg)
				if err != nil {
					fmt.Println("error marshalling answer message: ", err)
					continue
				}

				client.Send(msgJSON)
				continue
			}

			if m.Kind == ICECANDIDATE {
				fmt.Println("iceCandidate from client received")

				peerConnection.AddICECandidate(m.ClientICECandidate)
				continue
			}

			//TODO: handle stop
			if m.Kind == STOP {
				fmt.Println("stop from client received")
			}
		}
	}
}

func (r *Room) HandlePeer(pc *webrtc.PeerConnection, clientID string) {
	pc.OnICEConnectionStateChange(func(connectionState webrtc.ICEConnectionState) {
		fmt.Printf("ICE connection state for peer:%v has changed:%v\n", clientID, connectionState.String())

		if connectionState == webrtc.ICEConnectionStateConnected {
			fmt.Println("peer connected")

			go r.Clients[clientID].WriteRTP()
			return
		}

		if connectionState == webrtc.ICEConnectionStateDisconnected {
			fmt.Printf("peer %v disconnected\n", clientID)
			return
		}

		if connectionState == webrtc.ICEConnectionStateFailed {
			fmt.Printf("peer %v failed\n", clientID)
			return
		}
	})
}

type Message struct {
	ClientID           string                    `json:"client_id"`
	Kind               int                       `json:"kind"`
	Offer              webrtc.SessionDescription `json:"offer"`
	Answer             webrtc.SessionDescription `json:"answer"`
	ICECandidate       *webrtc.ICECandidate      `json:"ice_candidate"`
	ClientICECandidate webrtc.ICECandidateInit   `json:"client_ice_candidate"`
}
