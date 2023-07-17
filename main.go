package main

import (
	"ffmpeg-webrtc/pkg/camera"
	"log"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	camera, err := camera.NewCamera()
	if err != nil {
		log.Fatal(err)
	}

	if err := camera.Start(); err != nil {
		log.Fatal(err)
	}

	osSignals := make(chan os.Signal, 1)
	signal.Notify(osSignals, os.Interrupt, os.Kill, syscall.SIGTERM)
	<-osSignals
	camera.Stop()
}
