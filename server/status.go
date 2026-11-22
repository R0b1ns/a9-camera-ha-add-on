package main

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"time"
)

// The camera resolves these names via the DHCP DNS of its network. If they do
// not point at this host, the camera talks to the real cloud instead of us.
var cloudDNSNames = []string{
	"v720.naxclow.com",
	"v720.p2p.naxclow.com",
	"p2p.v720.naxclow.com",
	"dl2.naxclow.com",
	"dl2.naxclow.com.cn",
	"home.naxclow.com",
	"logo.naxclow.com.cn",
}

type statusJSON struct {
	DNSName       string   `json:"dns_name"`
	DNSIP         string   `json:"dns_ip,omitempty"`
	DNSOK         bool     `json:"dns_ok"`
	LocalIPs      []string `json:"local_ips"`
	WIFIAvail     bool     `json:"wifi_available"`
	WiFiIface     string   `json:"wifi_iface,omitempty"`
	LANConnected  bool     `json:"lan_connected"`
	WiFiConnected bool     `json:"wifi_connected"`
	APReady       bool     `json:"ap_ready"`
}

var lookupDNS = net.DefaultResolver.LookupIPAddr

func apiStatus(w http.ResponseWriter, _ *http.Request) {
	s := statusJSON{DNSName: cloudDNSNames[0], LocalIPs: localIPs()}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if ips, err := lookupDNS(ctx, s.DNSName); err == nil && len(ips) > 0 {
		s.DNSIP = ips[0].IP.String()
		s.DNSOK = containsString(s.LocalIPs, s.DNSIP)
	}
	// The AP setup is only safe when HA is on LAN and its Wi-Fi is unused:
	// Wi-Fi is borrowed briefly and LAN keeps the host reachable.
	s.LANConnected = lanConnected()
	if iface, ok := wifiInterface(); ok {
		s.WIFIAvail, s.WiFiIface = true, iface
		s.WiFiConnected = interfaceIPv4(iface)
		// Supervisor knows better than the interface flags whether an SSID is
		// actually connected. Fall back to the local address check outside HA.
		if active, err := supervisorWiFiActive(iface); err == nil {
			s.WiFiConnected = active
		}
	}
	s.APReady = s.LANConnected && s.WIFIAvail && !s.WiFiConnected
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(s)
}

func localIPs() []string {
	out := []string{}
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return out
	}
	for _, a := range addrs {
		n, ok := a.(*net.IPNet)
		if !ok {
			continue
		}
		ip := n.IP
		if ip.IsGlobalUnicast() && !ip.IsLoopback() && ip.To4() != nil {
			out = append(out, ip.String())
		}
	}
	return out
}

func containsString(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

func interfaceIPv4(name string) bool {
	iface, err := net.InterfaceByName(name)
	if err != nil {
		return false
	}
	addrs, err := iface.Addrs()
	if err != nil {
		return false
	}
	for _, a := range addrs {
		n, ok := a.(*net.IPNet)
		if !ok {
			continue
		}
		if ip := n.IP.To4(); ip != nil && ip.IsGlobalUnicast() {
			return true
		}
	}
	return false
}

// Docker and HA's internal interface are not a LAN connection of the host.
func isVirtualInterface(name string) bool {
	n := strings.ToLower(name)
	for _, prefix := range []string{"docker", "br-", "veth", "virbr", "tun", "tap", "sit", "hassio", "dummy"} {
		if strings.HasPrefix(n, prefix) {
			return true
		}
	}
	return false
}

// lanConnected reports whether a wired interface of the host has an address.
func lanConnected() bool {
	ifs, err := net.Interfaces()
	if err != nil {
		return false
	}
	for _, i := range ifs {
		if i.Flags&net.FlagUp == 0 || i.Flags&net.FlagLoopback != 0 {
			continue
		}
		n := strings.ToLower(i.Name)
		if strings.HasPrefix(n, "wl") || strings.Contains(n, "wireless") ||
			strings.Contains(n, "wifi") || isVirtualInterface(n) {
			continue
		}
		if interfaceIPv4(i.Name) {
			return true
		}
	}
	return false
}

// wifiInterface looks for a wireless LAN interface. Add-ons with host_network
// see the interfaces of the HA host (e.g. wlan0 on a Raspberry Pi).
func wifiInterface() (string, bool) {
	ifs, err := net.Interfaces()
	if err != nil {
		return "", false
	}
	for _, i := range ifs {
		if i.Flags&net.FlagUp == 0 {
			continue
		}
		if name := strings.ToLower(i.Name); strings.HasPrefix(name, "wl") ||
			strings.Contains(name, "wireless") || strings.Contains(name, "wifi") {
			return i.Name, true
		}
	}
	return "", false
}
