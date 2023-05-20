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
)

type Camera struct {
	App    string   `json:"app"`
	Args   []string `json:"args"`
	room   *ws.Room
	server server.Server
	done   chan bool
	cmd    *exec.Cmd
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

	cmd.Stderr = os.Stderr

	pipe, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("error creating stdout pipe for command: %v", err)
	}

	buf := make([]byte, 1024*1024)
	frames := make(chan []byte, 120)

	go func() {
		for {
			if len(c.room.Tracks) > 0 {
				fmt.Println("reading from stdout")
				for {
					n, err := pipe.Read(buf)
					if err != nil {
						fmt.Printf("error reading stdout: %v\n", err)
						return
					}

					frames <- buf[:n]
				}
			}
		}
	}()

	go func() {
		//payloader := NewPayloader()
		payloader := &codecs.H264Payloader{}
		packetizer := rtp.NewPacketizer(1200, 96, c.room.SSRC, payloader, rtp.NewRandomSequencer(), 90000)

		for {
			if len(c.room.Tracks) > 0 {
				for frame := range frames {
					packets := packetizer.Packetize(frame, uint32(time.Now().UnixNano()))

					for _, track := range c.room.Tracks {
						for _, packet := range packets {
							if err := track.WriteRTP(packet); err != nil {
								fmt.Printf("error writing sample: %v\n", err)
								c.room.UnregisterTrack(track)
							}
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
