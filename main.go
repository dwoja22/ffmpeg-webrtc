package main

import (
	"ffmpeg-webrtc/pkg/stream"
	"log"
	"os"
	"os/signal"
	"runtime/pprof"
	"syscall"
)

func main() {
	//cpu profiling
	f, err := os.Create("cpu.prof")
	if err != nil {
		log.Fatal(err)
	}

	if err := pprof.StartCPUProfile(f); err != nil {
		log.Fatal(err)
	}

	defer pprof.StopCPUProfile()

	stream, err := stream.NewStream()
	if err != nil {
		log.Fatal(err)
	}

	if err := stream.Start(); err != nil {
		log.Fatal(err)
	}

	osSignals := make(chan os.Signal, 1)
	signal.Notify(osSignals, os.Interrupt, os.Kill, syscall.SIGTERM)
	<-osSignals
	stream.Stop()
}
