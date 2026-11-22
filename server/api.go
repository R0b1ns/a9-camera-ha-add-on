package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dbuezas/a9-camera-ha-add-on/server/pkg/a9"
)

// deviceJSON is what the Home Assistant integration sees of a camera
type deviceJSON struct {
	UID         string         `json:"uid"`
	Online      bool           `json:"online"`
	Streaming   bool           `json:"streaming"`
	Host        string         `json:"host"`
	Port        int            `json:"port"`
	ConnectedAt time.Time      `json:"connected_at"`
	Info        map[string]any `json:"info"`
	Audio       bool           `json:"audio"`
}

func toJSON(d *a9.Device) deviceJSON {
	host, port, _ := net.SplitHostPort(d.RemoteAddr())
	p, _ := strconv.Atoi(port)
	info := d.Info()
	if info == nil {
		info = map[string]any{}
	}
	return deviceJSON{
		UID: d.UID, Online: d.Online(), Streaming: d.Streaming(),
		Host: host, Port: p, ConnectedAt: d.ConnectedAt, Info: info,
		Audio: d.Settings().Audio,
	}
}

func apiDevices(w http.ResponseWriter, _ *http.Request) {
	items := []deviceJSON{}
	for _, d := range server.Devices() {
		items = append(items, toJSON(d))
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(items)
}

// apiCommand sends a JSON object to the camera, ex: {"code":202,"IrLed":1}.
// The key order is kept, the camera wants "code" first.
func apiCommandJSON(w http.ResponseWriter, r *http.Request) {
	dev := server.Get(r.PathValue("uid"))
	if dev == nil {
		http.Error(w, a9.ErrNotConnected.Error(), http.StatusServiceUnavailable)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8192)
	cmd, err := decodeObj(r.Body)
	if err != nil || len(cmd) == 0 {
		http.Error(w, "body must be a JSON object with a code", http.StatusBadRequest)
		return
	}
	if !commandAllowed(cmd) {
		http.Error(w, "command code not allowed", http.StatusBadRequest)
		return
	}
	dev.SendCommand(cmd)
	// Log only the code: a caller could otherwise smuggle Wi-Fi credentials
	// into the generic command endpoint.
	code, _ := commandCode(cmd)
	log.Info().Str("uid", dev.UID).Int("code", code).Msg("[api] command")
	w.WriteHeader(http.StatusNoContent)
}

// apiAccessPoints scans the HA host's Wi-Fi for access points (camera APs).
func apiAccessPoints(w http.ResponseWriter, r *http.Request) {
	iface := r.URL.Query().Get("iface")
	if iface == "" {
		if found, ok := wifiInterface(); ok {
			iface = found
		} else {
			http.Error(w, "no Wi-Fi interface found", http.StatusServiceUnavailable)
			return
		}
	}
	if !validIfaceName(iface) {
		http.Error(w, "invalid Wi-Fi interface", http.StatusBadRequest)
		return
	}
	aps, err := supervisorAccessPoints(iface)
	if err != nil {
		log.Warn().Err(err).Str("iface", iface).Msg("[api] wifi scan")
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(aps)
}

type wifiAPRequest struct {
	Iface      string `json:"iface"`
	APSSID     string `json:"ap_ssid"`
	APPassword string `json:"ap_password"`
	SSID       string `json:"ssid"`
	Password   string `json:"password"`
}

// Overridable in tests: neither a real camera AP nor a 15 s wait is available
// in CI.
var (
	apSetWiFiFunc       = apSetWiFi
	waitForCameraAPFunc = waitForCameraAP
	// Never run two AP setups at once: they would overwrite each other's
	// wireless configuration on the same interface.
	apConfigureMu sync.Mutex
)

// apiWiFiAP configures a camera that is in its own AP mode: the HA host joins
// the camera's Wi-Fi, sends code 204 over TCP 6123 and rejoins the old network.
func apiWiFiAP(w http.ResponseWriter, r *http.Request) {
	var req wifiAPRequest
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	req.APSSID = strings.TrimSpace(req.APSSID)
	req.SSID = strings.TrimSpace(req.SSID)
	if req.Iface == "" {
		if found, ok := wifiInterface(); ok {
			req.Iface = found
		} else {
			http.Error(w, "no Wi-Fi interface found", http.StatusServiceUnavailable)
			return
		}
	}
	// The camera's AP-mode network is open: no ap_password is needed. The
	// optional field is still accepted for cameras with a protected AP.
	if req.APSSID == "" || req.SSID == "" ||
		len([]rune(req.Password)) < 8 || len([]rune(req.Password)) > 63 {
		http.Error(w, "ap_ssid, ssid and password (8-63 chars) required", http.StatusBadRequest)
		return
	}
	if !validIfaceName(req.Iface) {
		http.Error(w, "invalid Wi-Fi interface", http.StatusBadRequest)
		return
	}

	apConfigureMu.Lock()
	defer apConfigureMu.Unlock()

	// This flow assumes the HA host is on LAN and its Wi-Fi is unused. If a
	// home Wi-Fi is active, do not touch it: we could not restore its password.
	// If the state cannot be determined, abort too: failing open here would
	// overwrite an existing home Wi-Fi configuration.
	active, err := supervisorWiFiActive(req.Iface)
	if err != nil {
		log.Warn().Err(err).Str("iface", req.Iface).Msg("[api] wifi ap: state check failed")
		http.Error(w, "cannot verify Wi-Fi state; refusing to change the home network",
			http.StatusBadGateway)
		return
	}
	if active {
		http.Error(w, "Wi-Fi is in use by Home Assistant; this setup requires LAN and a free Wi-Fi interface",
			http.StatusConflict)
		return
	}
	log.Info().Str("iface", req.Iface).Str("ap", req.APSSID).Str("target", req.SSID).
		Msg("[api] wifi ap: joining camera")

	auth, psk := "open", ""
	if req.APPassword != "" {
		auth, psk = "wpa-psk", req.APPassword
	}
	if err := supervisorConnectWiFi(req.Iface, wifiConfig{
		Mode: "infrastructure", Auth: auth, SSID: req.APSSID, PSK: psk,
	}); err != nil {
		log.Warn().Err(err).Str("iface", req.Iface).Msg("[api] wifi ap: cannot join camera")
		http.Error(w, "cannot join camera AP", http.StatusBadGateway)
		return
	}
	if !waitForCameraAPFunc(15 * time.Second) {
		_ = disconnectWiFi(req.Iface)
		http.Error(w, "camera AP did not assign an address", http.StatusGatewayTimeout)
		return
	}
	err = apSetWiFiFunc(req.SSID, req.Password)
	// Always disconnect from the isolated camera AP, even on failure.
	if disconnectErr := disconnectWiFi(req.Iface); disconnectErr != nil {
		log.Error().Err(disconnectErr).Msg("[api] wifi ap: cannot disconnect Wi-Fi")
		if err == nil {
			err = disconnectErr
		}
	}
	if err != nil {
		log.Error().Err(err).Msg("[api] wifi ap: failed")
		http.Error(w, "camera did not accept the Wi-Fi settings", http.StatusBadGateway)
		return
	}
	log.Info().Str("ssid", req.SSID).Msg("[api] wifi ap: camera configured, Wi-Fi disconnected")
	w.WriteHeader(http.StatusNoContent)
}

func waitForCameraAP(timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", apHost, apPort), time.Second)
		if err == nil {
			_ = conn.Close()
			return true
		}
		time.Sleep(500 * time.Millisecond)
	}
	return false
}

func apiRefresh(w http.ResponseWriter, r *http.Request) {
	dev := server.Get(r.PathValue("uid"))
	if dev == nil {
		http.Error(w, a9.ErrNotConnected.Error(), http.StatusServiceUnavailable)
		return
	}
	dev.RefreshInfo()
	w.WriteHeader(http.StatusNoContent)
}

func decodeObj(r interface{ Read([]byte) (int, error) }) (a9.Obj, error) {
	dec := json.NewDecoder(r)
	dec.UseNumber()
	if t, err := dec.Token(); err != nil || t != json.Delim('{') {
		return nil, errors.New("not an object")
	}
	var obj a9.Obj
	for dec.More() {
		t, err := dec.Token()
		if err != nil {
			return nil, err
		}
		var v any
		if err = dec.Decode(&v); err != nil {
			return nil, err
		}
		if n, ok := v.(json.Number); ok {
			if i, err := n.Int64(); err == nil {
				v = i
			}
		}
		obj = append(obj, a9.KV{Key: t.(string), Value: v})
	}
	return obj, nil
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

// events sends every device change to the connected integrations (Server-Sent Events)
type events struct {
	mu      sync.Mutex
	clients map[chan []byte]struct{}
}

var bus = &events{clients: map[chan []byte]struct{}{}}

func (e *events) publish(d *a9.Device) {
	b := mustJSON(toJSON(d))
	e.mu.Lock()
	defer e.mu.Unlock()
	for c := range e.clients {
		select {
		case c <- b:
		default:
			// a slow client loses the connection and gets the full state again on reconnect
			close(c)
			delete(e.clients, c)
		}
	}
}

func apiEvents(w http.ResponseWriter, r *http.Request) {
	c := make(chan []byte, 64)
	bus.mu.Lock()
	bus.clients[c] = struct{}{}
	bus.mu.Unlock()
	defer func() {
		bus.mu.Lock()
		if _, ok := bus.clients[c]; ok {
			delete(bus.clients, c)
			close(c)
		}
		bus.mu.Unlock()
	}()

	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	rc := http.NewResponseController(w)

	write := func(b []byte) error {
		var buf bytes.Buffer
		fmt.Fprintf(&buf, "event: device\ndata: %s\n\n", b)
		if _, err := w.Write(buf.Bytes()); err != nil {
			return err
		}
		return rc.Flush()
	}

	for _, d := range server.Devices() {
		if write(mustJSON(toJSON(d))) != nil {
			return
		}
	}

	ping := time.NewTicker(30 * time.Second)
	defer ping.Stop()
	for {
		select {
		case b, ok := <-c:
			if !ok || write(b) != nil {
				return
			}
		case <-ping.C:
			if _, err := w.Write([]byte(": ping\n\n")); err != nil || rc.Flush() != nil {
				return
			}
		case <-r.Context().Done():
			return
		}
	}
}

// discover tells Home Assistant that the add-on runs, so the integration can set itself up
func discover(httpPort int) {
	token := os.Getenv("SUPERVISOR_TOKEN")
	if token == "" {
		return
	}
	body := mustJSON(map[string]any{
		"service": "a9_cam",
		"config":  map[string]any{"host": "127.0.0.1", "port": httpPort},
	})
	req, _ := http.NewRequest("POST", "http://supervisor/discovery", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Warn().Err(err).Msg("[main] discovery")
		return
	}
	_ = res.Body.Close()
	log.Debug().Int("status", res.StatusCode).Msg("[main] discovery")
}

type addonOptions struct {
	LogLevel       string `json:"log_level"`
	CameraHTTPPort *int   `json:"camera_http_port"`
	UIAllowedCIDRs string `json:"ui_allowed_cidrs"`
}

// readAddonOptions reads the add-on options. ok is false outside of the add-on.
func readAddonOptions() (opts addonOptions, ok bool) {
	b, err := os.ReadFile("/data/options.json")
	if err != nil {
		return opts, false
	}
	_ = json.Unmarshal(b, &opts)
	return opts, true
}

// apiWiFi asks a connected camera to join another WiFi network (code 204).
// It needs an active TCP session, so this works while the camera is online.
func apiWiFi(w http.ResponseWriter, r *http.Request) {
	dev := server.Get(r.PathValue("uid"))
	if dev == nil {
		http.Error(w, a9.ErrNotConnected.Error(), http.StatusServiceUnavailable)
		return
	}
	var body struct {
		SSID     string `json:"ssid"`
		Password string `json:"password"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "body must be a JSON object with ssid and password", http.StatusBadRequest)
		return
	}
	body.SSID = strings.TrimSpace(body.SSID)
	if body.SSID == "" || len([]rune(body.Password)) < 8 || len([]rune(body.Password)) > 63 {
		http.Error(w, "ssid must not be empty, password must be 8-63 characters", http.StatusBadRequest)
		return
	}
	// Same order and keys as the original app. The camera applies the change
	// and reconnects; the password is never written to the log.
	dev.SendCommand(a9.Obj{
		{"code", a9.FwdSetWiFi},
		{"devTarget", a9.CLITarget},
		{"s", body.SSID},
		{"p", body.Password},
	})
	log.Info().Str("uid", dev.UID).Str("ssid", body.SSID).Msg("[api] set wifi")
	w.WriteHeader(http.StatusNoContent)
}

// apiAccess changes who may use a camera's direct links on port 8557. The web
// UI and the integration API always stay behind the Home Assistant login.
func apiAccess(w http.ResponseWriter, r *http.Request) {
	uid := r.PathValue("uid")
	if uid == "" {
		http.Error(w, "missing uid", http.StatusBadRequest)
		return
	}
	var body struct {
		NetworkAccess    *bool `json:"network_access"`
		PasswordRequired *bool `json:"password_required"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	ac := access.get(uid)
	if body.NetworkAccess != nil {
		// When the direct links become reachable again, restore the safe
		// default: a password is required unless turned off in a later request.
		if *body.NetworkAccess && !ac.NetworkAccess {
			ac.PasswordRequired = true
		}
		ac.NetworkAccess = *body.NetworkAccess
	}
	if body.PasswordRequired != nil {
		ac.PasswordRequired = *body.PasswordRequired
	}
	if err := access.set(uid, ac); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	log.Info().Str("uid", uid).Bool("network_access", ac.NetworkAccess).
		Bool("password_required", ac.PasswordRequired).Msg("[api] access")
	w.WriteHeader(http.StatusNoContent)
}

// apiSettings changes server side settings of a camera, ex: {"audio": false}
func apiSettings(w http.ResponseWriter, r *http.Request) {
	dev := server.Get(r.PathValue("uid"))
	if dev == nil {
		http.Error(w, a9.ErrNotConnected.Error(), http.StatusServiceUnavailable)
		return
	}
	var body struct {
		Audio *bool `json:"audio"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Audio == nil {
		http.Error(w, `body must be {"audio": true|false}`, http.StatusBadRequest)
		return
	}
	if err := dev.SetAudio(*body.Audio); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	log.Info().Str("uid", dev.UID).Bool("audio", *body.Audio).Msg("[api] settings")
	w.WriteHeader(http.StatusNoContent)
}
