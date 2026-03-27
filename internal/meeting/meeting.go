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

// Detect finds a meeting directory by looking for record.mkv in cwd.
func Detect(cwd string) (*MeetingDir, error) {
	mkv := filepath.Join(cwd, "record.mkv")
	if _, err := os.Stat(mkv); os.IsNotExist(err) {
		return nil, fmt.Errorf("no record.mkv found in %s — run 'tranny rec' first", cwd)
	}
	return &MeetingDir{Path: cwd}, nil
}

func (m *MeetingDir) RecordMKVPath() string {
	return filepath.Join(m.Path, "record.mkv")
}

// MP3Pattern returns the ffmpeg segment output pattern (1-indexed, zero-padded).
func (m *MeetingDir) MP3Pattern() string {
	return filepath.Join(m.Path, "record-%03d.mp3")
}

func (m *MeetingDir) TranscriptPath() string {
	return filepath.Join(m.Path, "transcript.txt")
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
