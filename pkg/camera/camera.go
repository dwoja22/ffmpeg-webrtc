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
	"github.com/pion/webrtc/v3/pkg/media"
)

type Camera struct {
	App        string   `json:"app"`
	Args       []string `json:"args"`
	Stderr     bool     `json:"stderr"`
	StreamType string   `json:"stream_type"`
	room       *ws.Room
	server     server.Server
	done       chan bool
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
	h264FrameDuration := time.Millisecond * 33

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
	rtpFrames := make(chan []byte, 24)
	//channel to read rtp packets from pion packetizer and send to webrtc
	rtpPackets := make(chan *rtp.Packet, 240)

	go func() {
		for {
			if len(c.room.TracksSample) > 0 || len(c.room.TracksRTP) > 0 {
				fmt.Println("start reading stdout")
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
			}
		}
	}()

	if c.StreamType == "rtp" {
		go func() {
			//use the pion payloader to packetize the frames, mine is too slow
			payloader := &codecs.H264Payloader{}
			packetizer := rtp.NewPacketizer(1300, 96, c.room.SSRC, payloader, rtp.NewRandomSequencer(), 90000)

			for frame := range rtpFrames {
				packets := packetizer.Packetize(frame, uint32(time.Now().UnixNano()))
				for _, packet := range packets {
					rtpPackets <- packet
				}
			}
		}()
	}

	go func() {
		for {
			if len(c.room.TracksSample) > 0 && c.StreamType == "sample" {
				for frame := range sampleFrames {
					for trackID, track := range c.room.TracksSample {
						if err := track.WriteSample(media.Sample{Data: frame, Duration: time.Duration(h264FrameDuration)}); err != nil {
							fmt.Printf("error writing sample: %v\n", err)
							c.room.UnregisterTracks(trackID)
						}
					}
				}
			}

			if len(c.room.TracksRTP) > 0 && c.StreamType == "rtp" {
				for packet := range rtpPackets {
					for trackID, track := range c.room.TracksRTP {
						if err := track.WriteRTP(packet); err != nil {
							fmt.Printf("error writing sample: %v\n", err)
							c.room.UnregisterTracks(trackID)
						}
					}
				}
			}
		}
	}()

	return cmd.Start()
}

func (c *Camera) Stop() error {
	close(c.done)
	c.cmd.Process.Signal(syscall.SIGTERM)
	return nil
}
