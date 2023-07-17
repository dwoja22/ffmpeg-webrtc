package camera

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"syscall"
	"time"

	"ffmpeg-webrtc/pkg/server"
	ws "ffmpeg-webrtc/pkg/websocket"

	"github.com/pion/rtp"
	"github.com/pion/rtp/codecs"
	"github.com/pion/webrtc/v3"
	"github.com/pion/webrtc/v3/pkg/media"
)

type Camera struct {
	App        string   `json:"app"`
	Args       []string `json:"args"`
	Stderr     bool     `json:"stderr"` //prints app stderr to console
	StreamType string   `json:"stream_type"`
	room       *ws.Room
	server     server.Server
	done       chan bool
	Debug      bool
	cmd        *exec.Cmd
}

func NewCamera() (*Camera, error) {
	var camera Camera

	file, err := ioutil.ReadFile("config.json")
	if err != nil {
		return nil, err
	}

	if err = json.Unmarshal(file, &camera); err != nil {
		return nil, err
	}

	if _, err := exec.LookPath(camera.App); err != nil {
		return nil, fmt.Errorf("app %s does not exist", camera.App)
	}

	room := ws.NewRoom()
	done := make(chan bool, 1)

	server := server.Server{
		Room: room,
		Done: done,
	}

	camera.room = room
	camera.done = done
	camera.server = server
	camera.room.StreamType = camera.StreamType

	return &camera, nil
}

func (c *Camera) Start() error {
	go c.room.Start()
	go c.server.StartServer()

	fmt.Println(c.App)
	fmt.Println(c.Args)

	cmd := exec.Command(c.App, c.Args...)
	c.cmd = cmd

	fmt.Println(cmd.Args)

	if c.Stderr {
		cmd.Stderr = os.Stderr
	}

	pipe, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("error creating stdout pipe for command: %v", err)
	}

	buf := make([]byte, 1024*1024)
	//channel to read frames from ffmpeg and send to webrtc for type
	sampleFrames := make(chan []byte, 24)
	//channel to read rtp packets from ffmpeg and send to webrtc for type rtp
	rtpFrames := make(chan []byte, 240)
	//channel to read rtp packets from pion packetizer and send to webrtc
	rtpPackets := make(chan *rtp.Packet, 4800)
	h264FrameDuration := time.Millisecond * 33

	go func() {
		for {
			n, err := pipe.Read(buf)
			if err != nil {
				fmt.Printf("error reading stdout: %v\n", err)
				return
			}

			if c.StreamType == "sample" {
				sampleFrames <- buf[:n]
			}

			if c.StreamType == "rtp" {
				rtpFrames <- buf[:n]
			}
		}
	}()

	var start bool

	if c.StreamType == "rtp" {
		go func() {
			//use the pion payloader to packetize the frames, mine is too slow
			payloader := &codecs.H264Payloader{}
			packetizer := rtp.NewPacketizer(1400, 96, uint32(c.room.SSRC), payloader, rtp.NewRandomSequencer(), 90000)

			for frame := range rtpFrames {
				packets := packetizer.Packetize(frame, uint32(time.Now().UnixNano()))
				for _, packet := range packets {
					//fmt.Println(packet)
					rtpPackets <- packet
				}
			}
		}()
	}

	go func() {
		for {
			//NOTE:: dirty code to make sure the webrtc connection is established before reading in the video sample and sending data to the browser
			var connected bool

			for _, peer := range c.room.Peers {
				if peer.ConnectionState() == webrtc.PeerConnectionStateConnected {
					connected = true
					break
				}
			}

			if len(c.room.Media) > 0 && c.StreamType == "sample" {
				if start == false {
					start = true
					cmd.Start()
				}

				if c.Debug {
					for _, med := range c.room.Media {
						go func(rtpSender *webrtc.RTPSender) {
							for {
								rtcpPackets, _, err := rtpSender.ReadRTCP()
								if err != nil {
									fmt.Println("error reading rtcp packets: ", err)
									return
								}

								for _, packet := range rtcpPackets {
									fmt.Println(packet)
								}
							}
						}(med.RTPSender)
					}
				}

				go func() {
					for frame := range sampleFrames {
						for id, med := range c.room.Media {
							if err := med.TrackSample.WriteSample(media.Sample{Data: frame, Duration: time.Duration(h264FrameDuration)}); err != nil {
								fmt.Printf("error writing sample: %v\n", err)
								c.room.UnregisterMedia(id)
							}
						}
					}
				}()

				break
			}

			if len(c.room.Media) > 0 && c.StreamType == "rtp" && connected {
				if start == false {
					start = true
					cmd.Start()
				}

				if c.Debug {
					for _, med := range c.room.Media {
						go func(rtpSender *webrtc.RTPSender) {
							for {
								rtcpPackets, _, err := rtpSender.ReadRTCP()
								if err != nil {
									fmt.Println("error reading rtcp packets: ", err)
									return
								}

								for _, packet := range rtcpPackets {
									fmt.Println(packet)
								}
							}
						}(med.RTPSender)
					}
				}

				go func() {
					for packet := range rtpPackets {
						for id, med := range c.room.Media {
							if err := med.TrackRTP.WriteRTP(packet); err != nil {
								fmt.Printf("error writing sample: %v\n", err)
								c.room.UnregisterMedia(id)
							}
						}
					}
				}()

				break
			}
		}
	}()

	return nil
	//return cmd.Start()
}

func (c *Camera) Stop() error {
	close(c.done)
	c.cmd.Process.Signal(syscall.SIGTERM)
	return nil
}
