package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
)

// supervisorURL is a variable so tests can point it at a local server.
var supervisorURL = "http://supervisor"

type supervisorEnvelope struct {
	Result  string          `json:"result"`
	Data    json.RawMessage `json:"data"`
	Message string          `json:"message"`
}

// Interface names come from API requests. Never let them become path segments
// of their own: "../../core" must not reach the Supervisor with our token.
var ifaceNameRE = regexp.MustCompile(`^[A-Za-z0-9_-]{1,32}$`)

func validIfaceName(iface string) bool {
	return ifaceNameRE.MatchString(iface)
}

// networkIfacePath builds the only Supervisor paths this add-on may call.
func networkIfacePath(iface, suffix string) (string, error) {
	if !validIfaceName(iface) {
		return "", fmt.Errorf("invalid network interface name")
	}
	switch suffix {
	case "accesspoints", "info", "update":
	default:
		return "", fmt.Errorf("invalid supervisor endpoint")
	}
	return "/network/interface/" + url.PathEscape(iface) + "/" + suffix, nil
}

func supervisorRequest(method, path string, body any) (json.RawMessage, error) {
	if !strings.HasPrefix(path, "/network/interface/") {
		return nil, fmt.Errorf("supervisor path not allowed: %s", path)
	}
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
	client := &http.Client{
		Timeout: 20 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			// Go keeps custom headers on redirects to a different host. Stop
			// here so X-Supervisor-Token can never follow a redirect.
			return http.ErrUseLastResponse
		},
	}
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
		// The body may contain configuration details (up to 1 MiB). Never send
		// it to an API client or log it verbatim.
		return nil, fmt.Errorf("supervisor %s %s: HTTP %d", method, path, res.StatusCode)
	}
	var env supervisorEnvelope
	if err := json.Unmarshal(b, &env); err != nil {
		return nil, err
	}
	if env.Result != "ok" {
		return nil, fmt.Errorf("supervisor %s: request rejected (%s)", path, env.Result)
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
	path, err := networkIfacePath(iface, "accesspoints")
	if err != nil {
		return nil, err
	}
	data, err := supervisorRequest(http.MethodGet, path, nil)
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
	PSK  string `json:"psk,omitempty"`
}

// supervisorConnectWiFi joins an SSID. The wifi block replaces the whole
// wireless config of that interface, so callers must restore the old SSID.
// enabled:true is required because disconnectWiFi turns the interface off
// again; ipv4:auto is required because the Supervisor defaults to static and
// the camera AP can only hand out 192.168.169.x via DHCP.
func supervisorConnectWiFi(iface string, wifi wifiConfig) error {
	path, err := networkIfacePath(iface, "update")
	if err != nil {
		return err
	}
	_, err = supervisorRequest(http.MethodPost, path, map[string]any{
		"enabled": true,
		"wifi":    wifi,
		"ipv4":    map[string]any{"method": "auto"},
	})
	return err
}

// supervisorWiFiActive reports whether HA currently uses this interface. The
// AP flow is only safe when LAN is connected and Wi-Fi is free; a saved home
// network cannot be restored because Supervisor never returns its password.
func supervisorWiFiActive(iface string) (bool, error) {
	path, err := networkIfacePath(iface, "info")
	if err != nil {
		return false, err
	}
	data, err := supervisorRequest(http.MethodGet, path, nil)
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
	path, err := networkIfacePath(iface, "update")
	if err != nil {
		return err
	}
	_, err = supervisorRequest(http.MethodPost, path, map[string]any{"enabled": false})
	return err
}
