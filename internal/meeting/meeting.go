package meeting

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const inventoryFile = "tranny.json"

// Legacy profile file — written by tranny before tranny.json was introduced.
const profileFile = ".tranny-profile"

var nonAlphanumRe = regexp.MustCompile(`[^a-zA-Z0-9]+`)

// AudioTrack describes a single audio track in the recording file.
type AudioTrack struct {
	Index       int    `json:"index"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description"`
}

// Inventory is the self-describing metadata file written by 'tranny rec'.
// It captures enough information for downstream commands (mp3, compress,
// transcript) to operate correctly even if profile definitions change later.
type Inventory struct {
	RecordingFile string       `json:"recording_file"` // e.g. "record.mkv"
	AudioMap      string       `json:"audio_map"`      // ffmpeg stream specifier for the mix track
	AudioTracks   []AudioTrack `json:"audio_tracks"`
}

type MeetingDir struct {
	Path      string
	Name      string
	Time      time.Time
	Inventory *Inventory // nil for dirs created before tranny.json was introduced
	// profileName is kept only for backward compat with old .tranny-profile dirs.
	profileName string
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
// Reads tranny.json if present; falls back to .tranny-profile for old dirs.
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
	m := &MeetingDir{Path: cwd}
	if err := m.readInventory(); err != nil {
		// tranny.json absent: try legacy .tranny-profile for backward compat
		_ = m.readLegacyProfile()
	}
	return m, nil
}

// WriteInventory saves the inventory to tranny.json in the meeting dir.
func (m *MeetingDir) WriteInventory(inv *Inventory) error {
	m.Inventory = inv
	data, err := json.MarshalIndent(inv, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal inventory: %w", err)
	}
	return os.WriteFile(filepath.Join(m.Path, inventoryFile), data, 0644)
}

func (m *MeetingDir) readInventory() error {
	data, err := os.ReadFile(filepath.Join(m.Path, inventoryFile))
	if err != nil {
		return err
	}
	var inv Inventory
	if err := json.Unmarshal(data, &inv); err != nil {
		return fmt.Errorf("parse tranny.json: %w", err)
	}
	m.Inventory = &inv
	return nil
}

// readLegacyProfile reads .tranny-profile written by tranny before tranny.json.
func (m *MeetingDir) readLegacyProfile() error {
	data, err := os.ReadFile(filepath.Join(m.Path, profileFile))
	if err != nil {
		return err
	}
	m.profileName = strings.TrimSpace(string(data))
	return nil
}

// RecordingPath returns the absolute path to the recording file.
// Uses the inventory when available; falls back to probing the filesystem.
func (m *MeetingDir) RecordingPath() (string, error) {
	if m.Inventory != nil && m.Inventory.RecordingFile != "" {
		p := filepath.Join(m.Path, m.Inventory.RecordingFile)
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	// Fallback: probe for known extensions
	for _, ext := range []string{"mkv", "mp4"} {
		p := filepath.Join(m.Path, "record."+ext)
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("no recording file found in %s", m.Path)
}

// AudioMapForFFmpeg returns the ffmpeg stream specifier for the mix audio track.
// Uses the inventory when available; otherwise uses a legacy fallback.
func (m *MeetingDir) AudioMapForFFmpeg() (string, error) {
	if m.Inventory != nil && m.Inventory.AudioMap != "" {
		return m.Inventory.AudioMap, nil
	}
	// Legacy fallback for dirs with only .tranny-profile.
	// These constants mirror the AudioMap values in recorder/profiles.go.
	if m.profileName == "telegram" || m.profileName == "lowres" {
		return "0:a:0", nil
	}
	return "0:a:m:title:mix", nil // "default" profile
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
