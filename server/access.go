package main

import (
	"encoding/json"
	"net/url"
	"os"
	"strings"
	"sync"
)

// cameraAccess controls anonymous access to a camera's direct links on port
// 8557. Defaults keep the current behaviour: the links are reachable from the
// network, but ask for a Home Assistant login.
type cameraAccess struct {
	NetworkAccess    bool `json:"network_access"`
	PasswordRequired bool `json:"password_required"`
}

var defaultCameraAccess = cameraAccess{NetworkAccess: true, PasswordRequired: true}

// accessStore keeps the per-camera flags in a JSON file. Empty path: memory only.
type accessStore struct {
	path string
	mu   sync.RWMutex
	m    map[string]cameraAccess
}

func (s *accessStore) load() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m = map[string]cameraAccess{}
	if s.path == "" {
		return
	}
	if b, err := os.ReadFile(s.path); err == nil {
		_ = json.Unmarshal(b, &s.m)
	}
}

func (s *accessStore) get(uid string) cameraAccess {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if v, ok := s.m[uid]; ok {
		return v
	}
	return defaultCameraAccess
}

func (s *accessStore) set(uid string, v cameraAccess) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[uid] = v
	if s.path == "" {
		return nil
	}
	b, _ := json.MarshalIndent(s.m, "", "  ")
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

var access = &accessStore{m: map[string]cameraAccess{}}

// directLinkUID returns the camera UID for the two HTTP links served on port
// 8557. Other paths (the UI, the integration API) are never public.
func directLinkUID(path string) (string, bool) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) != 3 || parts[0] != "dev" {
		return "", false
	}
	if parts[2] != "live" && parts[2] != "snapshot" {
		return "", false
	}
	uid, err := url.PathUnescape(parts[1])
	if err != nil || uid == "" {
		return "", false
	}
	return uid, true
}
