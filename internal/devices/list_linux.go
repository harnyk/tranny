//go:build linux

package devices

import (
	"fmt"
	"os/exec"
	"strings"
)

// ListInputDevices returns PulseAudio/PipeWire capture sources.
func ListInputDevices(_ string) ([]InputDevice, error) {
	out, err := exec.Command("pactl", "list", "sources", "short").Output()
	if err != nil {
		return nil, fmt.Errorf("pactl list sources: %w", err)
	}

	defaultSource, _ := exec.Command("pactl", "get-default-source").Output()
	defaultName := strings.TrimSpace(string(defaultSource))

	var devices []InputDevice
	for line := range strings.SplitSeq(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		name := fields[1]
		if strings.HasSuffix(name, ".monitor") {
			continue
		}
		devices = append(devices, InputDevice{
			Index: name,
			Name:  name,
		})
	}

	if len(devices) == 0 {
		return nil, fmt.Errorf("no pulseaudio capture sources found")
	}

	if defaultName != "" {
		for i := range devices {
			if devices[i].Index == defaultName {
				devices[i].Name = defaultName + " (default)"
				break
			}
		}
	}

	return devices, nil
}
