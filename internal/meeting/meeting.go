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

var nonAlphanumRe = regexp.MustCompile(`[^a-zA-Z0-9]+`)

type MeetingDir struct {
	Path string
	Name string
	Time time.Time
}

// NewMeetingDir creates a new meeting directory under baseDir with source/, mix/, transcript/ subdirs.
// Name is slugified; if empty, defaults to "meeting".
func NewMeetingDir(baseDir, name string) (*MeetingDir, error) {
	if name == "" {
		name = "meeting"
	}
	slug := slugify(name)
	now := time.Now()
	dirName := fmt.Sprintf("%s--%s", now.Format("2006-01-02-15-04-05"), slug)
	absPath := filepath.Join(baseDir, dirName)
	for _, sub := range []string{"source", "mix", "transcript"} {
		if err := os.MkdirAll(filepath.Join(absPath, sub), 0755); err != nil {
			return nil, fmt.Errorf("create meeting dir: %w", err)
		}
	}
	return &MeetingDir{Path: absPath, Name: slug, Time: now}, nil
}

// Detect finds a meeting directory by looking for a source/ subdirectory in cwd.
func Detect(cwd string) (*MeetingDir, error) {
	src := filepath.Join(cwd, "source")
	if _, err := os.Stat(src); os.IsNotExist(err) {
		return nil, fmt.Errorf("no source/ directory found in %s — run 'tran rec' first", cwd)
	}
	return &MeetingDir{Path: cwd}, nil
}

func (m *MeetingDir) SourceDir() string {
	return filepath.Join(m.Path, "source")
}

func (m *MeetingDir) MixDir() string {
	return filepath.Join(m.Path, "mix")
}

func (m *MeetingDir) TranscriptDir() string {
	return filepath.Join(m.Path, "transcript")
}

func (m *MeetingDir) RecordMP4Path() string {
	return filepath.Join(m.SourceDir(), "record.mp4")
}

func (m *MeetingDir) MicMP3Path() string {
	return filepath.Join(m.SourceDir(), "mic.mp3")
}

func (m *MeetingDir) SysMP3Path() string {
	return filepath.Join(m.SourceDir(), "sys.mp3")
}

// MixMP3Pattern returns the ffmpeg segment output pattern (1-indexed, zero-padded).
func (m *MeetingDir) MixMP3Pattern() string {
	return filepath.Join(m.MixDir(), "record-%03d.mp3")
}

func (m *MeetingDir) TranscriptPath() string {
	return filepath.Join(m.TranscriptDir(), "transcript.txt")
}

// ListMixMP3s returns all mix/record-NNN.mp3 files sorted lexicographically.
func (m *MeetingDir) ListMixMP3s() ([]string, error) {
	matches, err := filepath.Glob(filepath.Join(m.MixDir(), "record-*.mp3"))
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
