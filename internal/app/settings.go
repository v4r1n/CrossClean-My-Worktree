package app

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sync"
)

const maxRecentFolders = 5

// Settings are the scan target and recent folders, saved as JSON in the user's
// config folder (e.g. %AppData%\CleanMyWT on Windows).
type Settings struct {
	mu   sync.Mutex
	path string
	data settingsData
}

type settingsData struct {
	Target Target   `json:"target"`
	Recent []string `json:"recentFolders"`
}

// LoadSettings reads saved settings; missing or unreadable files give defaults.
func LoadSettings() *Settings {
	s := &Settings{data: settingsData{Target: Target{Kind: "home"}}}
	dir, err := os.UserConfigDir()
	if err != nil {
		return s
	}
	s.path = filepath.Join(dir, "CleanMyWT", "settings.json")
	if raw, err := os.ReadFile(s.path); err == nil {
		var data settingsData
		if err := json.Unmarshal(raw, &data); err != nil {
			log.Printf("ignoring unreadable settings %s: %v", s.path, err)
		} else if validTarget(data.Target) {
			s.data = data
		}
	}
	return s
}

// MemorySettings are settings that are never saved (for tests).
func MemorySettings() *Settings {
	return &Settings{data: settingsData{Target: Target{Kind: "home"}}}
}

func validTarget(t Target) bool {
	switch t.Kind {
	case "home", "all":
		return true
	case "folder":
		return filepath.IsAbs(t.Path)
	}
	return false
}

// Target is the saved scan target.
func (s *Settings) Target() Target {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.data.Target
}

// Recent are recently scanned folders, newest first.
func (s *Settings) Recent() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.data.Recent...)
}

// SetTarget saves the target and, for a folder, moves it to the top of the recents.
func (s *Settings) SetTarget(t Target) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.Target = t
	if t.Kind == "folder" {
		recent := []string{t.Path}
		for _, p := range s.data.Recent {
			if p != t.Path && len(recent) < maxRecentFolders {
				recent = append(recent, p)
			}
		}
		s.data.Recent = recent
	}
	s.save()
}

func (s *Settings) save() {
	if s.path == "" {
		return
	}
	raw, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		log.Printf("can't save settings: %v", err)
		return
	}
	if err := os.WriteFile(s.path, raw, 0o644); err != nil {
		log.Printf("can't save settings: %v", err)
	}
}
