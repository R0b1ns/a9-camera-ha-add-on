package main

import (
	"encoding/json"
	"os"
	"regexp"
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

// writePrivateFile writes state files atomically and readable only by the
// add-on user (the session key uses the same permissions).
func writePrivateFile(path string, b []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (s *accessStore) set(uid string, v cameraAccess) error {
	return s.update(uid, func(cur *cameraAccess) { *cur = v })
}

// update reads, changes and writes one camera's flags under a single lock, so
// parallel API calls cannot lose each other's changes.
func (s *accessStore) update(uid string, fn func(*cameraAccess)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.m[uid]
	if !ok {
		v = defaultCameraAccess
	}
	fn(&v)
	s.m[uid] = v
	if s.path == "" {
		return nil
	}
	b, _ := json.MarshalIndent(s.m, "", "  ")
	return writePrivateFile(s.path, b)
}

var access = &accessStore{m: map[string]cameraAccess{}}

var uidRE = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

func validUID(uid string) bool { return uidRE.MatchString(uid) }

// directLinkUID returns the camera UID for the two HTTP links served on port
// 8557. Other paths (the UI, the integration API) are never public. r.URL.Path
// is already decoded by net/http; decoding it again could make the auth check
// see a different UID than the handler.
func directLinkUID(path string) (string, bool) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) != 3 || parts[0] != "dev" {
		return "", false
	}
	if parts[2] != "live" && parts[2] != "snapshot" {
		return "", false
	}
	if !validUID(parts[1]) {
		return "", false
	}
	return parts[1], true
}
