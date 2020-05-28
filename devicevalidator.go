package main

import (
	"errors"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

func checkDevices() ([]string, error) {
	devices, err := filepath.Glob("/dev/video*")
	if err != nil {
		return nil, err
	}

	if len(devices) < 1 {
		return nil, errors.New("no devices found")
	}

	supportedDevices := []string{}

	for _, device := range devices {
		out, err := exec.Command("v4l2-ctl", "--device="+device, "--list-formats").Output()
		if err != nil {
			return nil, errors.New(err.Error())
		}

		match, err := regexp.MatchString("H.264", string(out))
		if err != nil {
			return nil, err
		}

		if match {
			supportedDevices = append(supportedDevices, device)
		}
	}

	return supportedDevices, nil
}

func processV4l2(input string) string {
	data := strings.Replace(input, "\n\n", "\n", -1)
	data = strings.Replace(data, "\n\t", "\n", -1)
	data = strings.Replace(data, "\n\n\t", "\n", -1)

	return data
}
