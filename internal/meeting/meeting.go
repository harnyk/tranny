package meeting

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const profileFile = ".tranny-profile"

var nonAlphanumRe = regexp.MustCompile(`[^a-zA-Z0-9]+`)

type MeetingDir struct {
	Path        string
	Name        string
	Time        time.Time
	ProfileName string // recording profile used; defaults to "default"
}

// NewMeetingDir creates a new meeting directory under baseDir.
// Name is slugified; if empty, defaults to "meeting".
func NewMeetingDir(baseDir, name string) (*MeetingDir, error) {
	if name == "" {
		name = "meeting"
	}
	slug := slugify(name)
	now := time.Now()
	dirName := fmt.Sprintf("%s--%s", now.Format("2006-01-02-15-04-05"), slug)
	absPath := filepath.Join(baseDir, dirName)
	if err := os.MkdirAll(absPath, 0755); err != nil {
		return nil, fmt.Errorf("create meeting dir: %w", err)
	}
	return &MeetingDir{Path: absPath, Name: slug, Time: now}, nil
}

// Detect finds a meeting directory by looking for a recording file in cwd.
// Reads the profile name from .tranny-profile if present; defaults to "default".
func Detect(cwd string) (*MeetingDir, error) {
	found := false
	for _, ext := range []string{"mkv", "mp4"} {
		if _, err := os.Stat(filepath.Join(cwd, "record."+ext)); err == nil {
			found = true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("no recording found in %s — run 'tranny rec' first", cwd)
	}
	m := &MeetingDir{Path: cwd, ProfileName: "default"}
	_ = m.readProfile() // best-effort; missing file is fine
	return m, nil
}

// WriteProfile saves the profile name to .tranny-profile in the meeting dir.
func (m *MeetingDir) WriteProfile(name string) error {
	m.ProfileName = name
	return os.WriteFile(filepath.Join(m.Path, profileFile), []byte(name), 0644)
}

func (m *MeetingDir) readProfile() error {
	data, err := os.ReadFile(filepath.Join(m.Path, profileFile))
	if err != nil {
		return err
	}
	m.ProfileName = strings.TrimSpace(string(data))
	return nil
}

// RecordingPath returns the path to the actual recording file (mkv or mp4).
func (m *MeetingDir) RecordingPath() (string, error) {
	for _, ext := range []string{"mkv", "mp4"} {
		p := filepath.Join(m.Path, "record."+ext)
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("no recording file found in %s", m.Path)
}

func (m *MeetingDir) RecordMKVPath() string {
	return filepath.Join(m.Path, "record.mkv")
}

// RecordPath returns the path for a recording with the given file extension (without dot).
func (m *MeetingDir) RecordPath(ext string) string {
	return filepath.Join(m.Path, "record."+ext)
}

// MP3Pattern returns the ffmpeg segment output pattern (1-indexed, zero-padded).
func (m *MeetingDir) MP3Pattern() string {
	return filepath.Join(m.Path, "record-%03d.mp3")
}

func (m *MeetingDir) TranscriptPath() string {
	return filepath.Join(m.Path, "transcript.txt")
}

func (m *MeetingDir) CompressedMP4Path() string {
	return filepath.Join(m.Path, "record-compressed.mp4")
}

// ListMP3s returns all record-NNN.mp3 files sorted lexicographically.
func (m *MeetingDir) ListMP3s() ([]string, error) {
	matches, err := filepath.Glob(filepath.Join(m.Path, "record-*.mp3"))
	if err != nil {
		return nil, err
	}
	sort.Strings(matches)
	return matches, nil
}

func slugify(name string) string {
	slug := nonAlphanumRe.ReplaceAllString(name, "-")
	slug = strings.Trim(slug, "-")
	return slug
}
