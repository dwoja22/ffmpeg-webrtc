package stream

import (
	"encoding/json"
	"ffmpeg-webrtc/pkg/server"
	wbrtc "ffmpeg-webrtc/pkg/webrtc"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/pion/webrtc/v3"
	"golang.org/x/sys/unix"
)

const H264FRAMEDURATION = time.Millisecond * 33

type Stream struct {
	App      string   `json:"app"`
	Args     []string `json:"args"`
	Type     string   `json:"type"`
	PipeName string   `json:"pipe_name"`
	FromFile bool     `json:"from_file"`
	room     *wbrtc.Room
	server   *server.Server
	cmd      *exec.Cmd
	done     chan bool
	pipe     *os.File
	logger   *os.File
}

func NewStream() (*Stream, error) {
	var stream Stream

	config, err := ioutil.ReadFile("config.json")
	if err != nil {
		return nil, err
	}

	if err = json.Unmarshal(config, &stream); err != nil {
		return nil, err
	}

	if _, err := exec.LookPath(stream.App); err != nil {
		return nil, fmt.Errorf("app %s does not exist", stream.App)
	}

	if len(stream.Args) == 0 {
		return nil, fmt.Errorf("args cannot be empty")
	}

	if stream.PipeName == "" {
		return nil, fmt.Errorf("pipe_name must not be empty")
	}

	done := make(chan bool, 1)
	room := wbrtc.NewRoom(done)
	server := server.NewServer(room, done)

	stream.server = server
	stream.room = room
	stream.done = done

	return &stream, nil
}

func (s *Stream) Start() error {
	cmd := exec.Command(s.App, s.Args...)
	s.cmd = cmd

	fmt.Println(cmd.Args)

	if err := s.initIO(cmd); err != nil {
		return err
	}

	go s.server.Start()
	go s.room.Start()

	if s.FromFile {
		if err := s.streamFromFile(); err != nil {
			return err
		}
	} else {
		if err := s.streamFromDevice(); err != nil {
			return err
		}
	}

	return nil
}

func (s *Stream) Stop() error {
	//stop the ffmpeg process
	s.cmd.Process.Signal(syscall.SIGTERM)

	//close the pipe
	if err := s.pipe.Close(); err != nil {
		return fmt.Errorf("error closing pipe: %v", err)
	}

	s.logger.Close()

	close(s.done)

	return nil
}

func (s *Stream) initIO(cmd *exec.Cmd) error {
	if _, err := os.Stat(s.PipeName); os.IsNotExist(err) {
		if err := syscall.Mkfifo(s.PipeName, 0666); err != nil {
			return fmt.Errorf("error creating named pipe: %v", err)
		}
	}

	//set pipe to non-blocking
	pipe, err := os.OpenFile(s.PipeName, os.O_RDWR|syscall.O_NONBLOCK, os.ModeNamedPipe)
	if err != nil {
		return fmt.Errorf("error opening named pipe: %v", err)
	}

	//set the pipe size to 1MB
	if _, err := unix.FcntlInt(pipe.Fd(), syscall.F_SETPIPE_SZ, 1024*1024); err != nil {
		return fmt.Errorf("error setting pipe size: %v", err)
	}

	//check size of pipe
	pipeSize, err := unix.FcntlInt(pipe.Fd(), syscall.F_GETPIPE_SZ, 0)
	if err != nil {
		return fmt.Errorf("error getting pipe size: %v", err)
	}

	fmt.Printf("created named pipe with name %v and size %v\n", s.PipeName, pipeSize)

	s.pipe = pipe

	//create log file for app
	logger, err := os.OpenFile(s.App+".log", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0666)
	if err != nil {
		fmt.Printf("error creating log file: %v\n", err)
	}

	//set io for cmd
	s.logger = logger
	cmd.Stderr = logger
	cmd.Stdout = pipe

	return nil
}

func (s *Stream) streamFromFile() error {
	connected := false

	for !connected {
		if len(s.room.Clients) < 1 {
			continue
		}

		for _, client := range s.room.Clients {
			if client.PC == nil {
				continue
			}

			if client.PC.ConnectionState() == webrtc.PeerConnectionStateConnected {
				connected = true
				break
			}
		}
	}

	go s.stream()

	return nil
}

func (s *Stream) streamFromDevice() error {
	go s.stream()

	return nil
}

func (s *Stream) stream() {
	buf := make([]byte, 1024*1024)
	frames := make(chan []byte, 240)

	go func() {
		for {
			n, err := s.pipe.Read(buf)
			if err != nil {
				continue
			}

			frames <- buf[:n]
		}
	}()

	go func() {
		for frame := range frames {
			for _, client := range s.room.Clients {
				if client.PC != nil {
					if client.PC.ConnectionState() == webrtc.PeerConnectionStateConnected {
						client.Frames <- frame
					}
				}
			}
		}
	}()

	s.cmd.Start()
}
