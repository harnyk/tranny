package recorder

import "fmt"

// RecordingParams holds the audio/video sources for a recording session.
type RecordingParams struct {
	Display string
	Monitor string // PulseAudio monitor (system audio)
	Mic     string // PulseAudio mic source
}

// Profile defines a set of ffmpeg parameters for a recording session.
type Profile struct {
	Name        string
	Description string
	Ext         string // output file extension without dot, e.g. "mkv" or "mp4"
	AudioMap    string // ffmpeg stream specifier used to extract audio (for mp3/transcript)
	BuildArgs   func(p RecordingParams, outputPath string) []string
}

// Profiles is the registry of all predefined recording profiles.
var Profiles = map[string]*Profile{
	"default":  profileDefault,
	"telegram": profileTelegram,
	"lowres":   profileLowres,
}

// GetProfile returns a profile by name, or an error listing valid names.
func GetProfile(name string) (*Profile, error) {
	p, ok := Profiles[name]
	if !ok {
		return nil, fmt.Errorf("unknown profile %q — valid profiles: default, telegram, lowres", name)
	}
	return p, nil
}

// profileDefault: full-resolution, three audio tracks (system/mic/mix), MKV.
// Suitable for archiving; preserves all audio for later processing.
var profileDefault = &Profile{
	Name:        "default",
	Description: "Full quality: 3 audio tracks (system, mic, mix), libx264 veryfast crf23, MKV",
	Ext:         "mkv",
	AudioMap:    "0:a:m:title:mix",
	BuildArgs: func(p RecordingParams, outputPath string) []string {
		return []string{
			"-f", "x11grab",
			"-framerate", "30",
			"-i", p.Display,

			"-f", "pulse",
			"-i", p.Monitor,

			"-f", "pulse",
			"-i", p.Mic,

			"-filter_complex", "[1:a][2:a]amix=inputs=2:normalize=0[mix]",
			"-map", "0:v",
			"-map", "1:a",
			"-map", "2:a",
			"-map", "[mix]",

			"-c:v", "libx264", "-preset", "veryfast", "-crf", "23",
			"-c:a", "aac", "-b:a", "160k",

			"-metadata:s:a:0", "title=system",
			"-metadata:s:a:1", "title=mic",
			"-metadata:s:a:2", "title=mix",
			"-disposition:a:0", "0",
			"-disposition:a:1", "0",
			"-disposition:a:2", "default",

			"-y",
			outputPath,
		}
	},
}

// profileTelegram: Telegram-compatible MP4.
// Scale to 1280px wide, single mix audio, yuv420p, faststart.
// Matches the output of the compress command so it can be shared directly.
var profileTelegram = &Profile{
	Name:        "telegram",
	Description: "Telegram-compatible MP4: 1280px wide, crf28 ultrafast, mix audio only, faststart",
	Ext:         "mp4",
	AudioMap:    "0:a:0",
	BuildArgs: func(p RecordingParams, outputPath string) []string {
		return []string{
			"-f", "x11grab",
			"-framerate", "30",
			"-i", p.Display,

			"-f", "pulse",
			"-i", p.Monitor,

			"-f", "pulse",
			"-i", p.Mic,

			"-filter_complex", "[1:a][2:a]amix=inputs=2:normalize=0[mix]",
			"-map", "0:v",
			"-map", "[mix]",

			"-c:v", "libx264",
			"-profile:v", "main",
			"-crf", "28",
			"-preset", "ultrafast",
			"-vf", "scale=1280:-2",
			"-pix_fmt", "yuv420p",
			"-c:a", "aac", "-b:a", "128k",
			"-movflags", "+faststart",

			"-y",
			outputPath,
		}
	},
}

// profileLowres: Compact MP4 optimised for readability.
// 1280px wide, 15fps, crf22 — smaller file than default but text remains legible.
var profileLowres = &Profile{
	Name:        "lowres",
	Description: "Low-res MP4: 1280px wide, 15fps, crf22 veryfast, mix audio only — readable text, smaller file",
	Ext:         "mp4",
	AudioMap:    "0:a:0",
	BuildArgs: func(p RecordingParams, outputPath string) []string {
		return []string{
			"-f", "x11grab",
			"-framerate", "15",
			"-i", p.Display,

			"-f", "pulse",
			"-i", p.Monitor,

			"-f", "pulse",
			"-i", p.Mic,

			"-filter_complex", "[1:a][2:a]amix=inputs=2:normalize=0[mix]",
			"-map", "0:v",
			"-map", "[mix]",

			"-c:v", "libx264",
			"-profile:v", "main",
			"-crf", "22",
			"-preset", "veryfast",
			"-vf", "scale=1280:-2",
			"-pix_fmt", "yuv420p",
			"-c:a", "aac", "-b:a", "96k",
			"-movflags", "+faststart",

			"-y",
			outputPath,
		}
	},
}
