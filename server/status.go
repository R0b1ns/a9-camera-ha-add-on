package main

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"strings"
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
	DNSName   string   `json:"dns_name"`
	DNSIP     string   `json:"dns_ip,omitempty"`
	DNSOK     bool     `json:"dns_ok"`
	LocalIPs  []string `json:"local_ips"`
	WIFIAvail bool     `json:"wifi_available"`
	WiFiIface string   `json:"wifi_iface,omitempty"`
}

var lookupDNS = net.DefaultResolver.LookupIPAddr

func apiStatus(w http.ResponseWriter, _ *http.Request) {
	s := statusJSON{DNSName: cloudDNSNames[0], LocalIPs: localIPs()}
	if ips, err := lookupDNS(context.Background(), s.DNSName); err == nil && len(ips) > 0 {
		s.DNSIP = ips[0].IP.String()
		s.DNSOK = containsString(s.LocalIPs, s.DNSIP)
	}
	if iface, ok := wifiInterface(); ok {
		s.WIFIAvail, s.WiFiIface = true, iface
	}
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
