//go:build darwin

package devices

import "testing"

func TestParseAVFoundationAudioDevices(t *testing.T) {
	output := `[AVFoundation indev @ 0xa0301c140] AVFoundation video devices:
[AVFoundation indev @ 0xa0301c140] [0] FaceTime HD Camera
[AVFoundation indev @ 0xa0301c140] AVFoundation audio devices:
[AVFoundation indev @ 0xa0301c140] [0] WH-CH720N
[AVFoundation indev @ 0xa0301c140] [1] MacBook Pro Microphone
[AVFoundation indev @ 0xa0301c140] [2] Microsoft Teams Audio
[in#0 @ 0xa0301c000] Error opening input: Input/output error
`

	devices, err := parseAVFoundationAudioDevices(output)
	if err != nil {
		t.Fatalf("parseAVFoundationAudioDevices: %v", err)
	}

	want := []InputDevice{
		{Index: "0", Name: "WH-CH720N"},
		{Index: "1", Name: "MacBook Pro Microphone"},
		{Index: "2", Name: "Microsoft Teams Audio"},
	}
	if len(devices) != len(want) {
		t.Fatalf("got %d devices, want %d", len(devices), len(want))
	}
	for i := range want {
		if devices[i] != want[i] {
			t.Errorf("device[%d]: got %+v, want %+v", i, devices[i], want[i])
		}
	}
}
