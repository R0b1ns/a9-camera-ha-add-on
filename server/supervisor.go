package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// supervisorURL is a variable so tests can point it at a local server.
var supervisorURL = "http://supervisor"

type supervisorEnvelope struct {
	Result  string          `json:"result"`
	Data    json.RawMessage `json:"data"`
	Message string          `json:"message"`
}

func supervisorRequest(method, path string, body any) (json.RawMessage, error) {
	token := os.Getenv("SUPERVISOR_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("no SUPERVISOR_TOKEN (add-on only)")
	}
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, supervisorURL+path, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Supervisor-Token", token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	client := &http.Client{Timeout: 20 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	b, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("supervisor %s %s: HTTP %d: %s", method, path, res.StatusCode, string(b))
	}
	var env supervisorEnvelope
	if err := json.Unmarshal(b, &env); err != nil {
		return nil, err
	}
	if env.Result != "ok" {
		return nil, fmt.Errorf("supervisor %s: %s", path, env.Message)
	}
	return env.Data, nil
}

type accessPoint struct {
	SSID   string `json:"ssid"`
	Signal int    `json:"signal"`
	MAC    string `json:"mac,omitempty"`
}

// supervisorAccessPoints scans for Wi-Fi networks on the given interface.
func supervisorAccessPoints(iface string) ([]accessPoint, error) {
	data, err := supervisorRequest(http.MethodGet,
		"/network/interface/"+iface+"/accesspoints", nil)
	if err != nil {
		return nil, err
	}
	var out struct {
		AccessPoints []accessPoint `json:"accesspoints"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return out.AccessPoints, nil
}

type wifiConfig struct {
	Mode string `json:"mode"`
	Auth string `json:"auth"`
	SSID string `json:"ssid"`
	PSK  string `json:"psk"`
}

// supervisorConnectWiFi joins an SSID. The wifi block replaces the whole
// wireless config of that interface, so callers must restore the old SSID.
func supervisorConnectWiFi(iface string, wifi wifiConfig) error {
	_, err := supervisorRequest(http.MethodPost,
		"/network/interface/"+iface+"/update", map[string]any{"wifi": wifi})
	return err
}

// supervisorWiFiActive reports whether HA currently uses this interface. The
// AP flow is only safe when LAN is connected and Wi-Fi is free; a saved home
// network cannot be restored because Supervisor never returns its password.
func supervisorWiFiActive(iface string) (bool, error) {
	data, err := supervisorRequest(http.MethodGet,
		"/network/interface/"+iface+"/info", nil)
	if err != nil {
		return false, err
	}
	var info struct {
		Connected bool `json:"connected"`
	}
	if err := json.Unmarshal(data, &info); err != nil {
		return false, err
	}
	return info.Connected, nil
}

// disconnectWiFi drops the (isolated) camera AP connection after the setup.
func disconnectWiFi(iface string) error {
	_, err := supervisorRequest(http.MethodPost,
		"/network/interface/"+iface+"/update", map[string]any{"enabled": false})
	return err
}
