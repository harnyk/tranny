//go:build darwin

package devices

import (
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

var avfoundationDeviceLine = regexp.MustCompile(`^\[AVFoundation indev @ [^\]]+\] \[(\d+)\] (.+)$`)

// ListInputDevices returns avfoundation audio input devices via ffmpeg.
func ListInputDevices(ffmpegBin string) ([]InputDevice, error) {
	cmd := exec.Command(ffmpegBin, "-f", "avfoundation", "-list_devices", "true", "-i", "")
	out, _ := cmd.CombinedOutput()

	devices, err := parseAVFoundationAudioDevices(string(out))
	if err != nil {
		return nil, fmt.Errorf("list avfoundation devices: %w", err)
	}
	return devices, nil
}

func parseAVFoundationAudioDevices(output string) ([]InputDevice, error) {
	var devices []InputDevice
	inAudio := false

	for line := range strings.SplitSeq(output, "\n") {
		if strings.Contains(line, "AVFoundation audio devices:") {
			inAudio = true
			continue
		}
		if strings.Contains(line, "AVFoundation video devices:") {
			inAudio = false
			continue
		}
		if !inAudio {
			continue
		}

		m := avfoundationDeviceLine.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		devices = append(devices, InputDevice{Index: m[1], Name: m[2]})
	}

	if len(devices) == 0 {
		return nil, fmt.Errorf("no audio devices found in ffmpeg output")
	}
	return devices, nil
}
