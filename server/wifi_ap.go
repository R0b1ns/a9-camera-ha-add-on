package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/dbuezas/a9-camera-ha-add-on/server/pkg/a9"
)

// AP-mode client. The camera is a Wi-Fi access point (192.168.169.1) and
// listens on TCP 6123. Ported from a9-v720/src/v720_ap.py:
//
//	LIVE_MOTION (cmd 115) -> {"code":501,...} -> {"code":502,"content":{"code":204,...}}
//
// Unlike the STA-mode cloud protocol, this does not need a bootstrap or MQTT.
const (
	apDevTarget   = "deadbeef"
	apToken       = "This is TEST token"
	cmdLiveMotion = 115
	codeAPConnect = 501
	codeAPCommand = 502
	// The camera only stores the new credentials on code 204. It switches from
	// AP to STA mode when it reboots (code 299), as in the original a9-v720 flow.
	fwdReboot = 299
)

// apAddress is a variable so tests can point at a local fake camera AP.
var apAddress = "192.168.169.1:6123"

// apClient sends one AP command and closes the connection. The camera sends
// heartbeats between frames; they are skipped while looking for the reply.
func apSetWiFi(ssid, password string) error {
	conn, err := net.DialTimeout("tcp", apAddress, 4*time.Second)
	if err != nil {
		return fmt.Errorf("camera AP not reachable: %w", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(8 * time.Second))

	reader := apReader{conn: conn}
	// handshake
	if err := apSend(conn, &a9.Packet{Cmd: cmdLiveMotion, ForwardID: a9DefaultForwardID()}); err != nil {
		return err
	}
	if _, err := reader.next(); err != nil {
		return fmt.Errorf("camera AP: no LIVE_MOTION reply: %w", err)
	}
	connect := map[string]any{
		"code": codeAPConnect, "target": apDevTarget, "token": apToken,
		"unixTimer": time.Now().Unix(),
	}
	if err := apSendJSON(conn, connect); err != nil {
		return err
	}
	if _, err := reader.next(); err != nil {
		return fmt.Errorf("camera AP: no connect reply: %w", err)
	}
	command := map[string]any{
		"code": codeAPCommand,
		"content": map[string]any{
			"code": a9.FwdSetWiFi, "devTarget": apDevTarget, "s": ssid, "p": password,
		},
	}
	if err := apSendJSON(conn, command); err != nil {
		return err
	}
	reply, err := reader.next()
	if err != nil {
		return fmt.Errorf("camera AP: no set-wifi reply: %w", err)
	}
	var ack struct {
		Code    int `json:"code"`
		Content struct {
			Code   int `json:"code"`
			Status int `json:"status"`
		} `json:"content"`
	}
	if err := json.Unmarshal(reply.Payload, &ack); err != nil {
		return fmt.Errorf("camera AP: invalid set-wifi reply: %w", err)
	}
	if ack.Code != 0 && ack.Code != codeAPCommand && ack.Code != a9.CodeForward {
		return fmt.Errorf("camera AP: unexpected set-wifi reply code %d", ack.Code)
	}
	if ack.Content.Code != 0 && ack.Content.Code != a9.FwdSetWiFi {
		return fmt.Errorf("camera AP: unexpected set-wifi content code %d", ack.Content.Code)
	}
	if ack.Content.Status != 0 && ack.Content.Status != 200 {
		return fmt.Errorf("camera rejected Wi-Fi change: status %d", ack.Content.Status)
	}

	// Without this the camera keeps broadcasting its AP and the new settings
	// never take effect. The Python reference sends the reboot as the last AP
	// command and does not wait for a reply.
	reboot := map[string]any{
		"code": codeAPCommand,
		"content": map[string]any{
			"code": fwdReboot, "devTarget": apDevTarget,
		},
	}
	if err := apSendJSON(conn, reboot); err != nil {
		return fmt.Errorf("camera AP: cannot reboot after Wi-Fi change: %w", err)
	}
	return nil
}

func a9DefaultForwardID() [8]byte {
	return [8]byte{'0', '0', '0', '0', '0', '0', '0', '0'}
}

func apSend(conn net.Conn, p *a9.Packet) error {
	_, err := conn.Write(p.Marshal())
	return err
}

func apSendJSON(conn net.Conn, obj any) error {
	b, err := json.Marshal(obj)
	if err != nil {
		return err
	}
	return apSend(conn, &a9.Packet{Cmd: a9.CmdJSON, ForwardID: a9DefaultForwardID(), Payload: b})
}

// apReader buffers TCP bytes until a full 20-byte-headered packet is present.
type apReader struct {
	conn net.Conn
	buf  []byte
}

func (r *apReader) next() (*a9.Packet, error) {
	tmp := make([]byte, 4096)
	for {
		if len(r.buf) >= 20 {
			want := int(uint32(r.buf[0]) | uint32(r.buf[1])<<8 |
				uint32(r.buf[2])<<16 | uint32(r.buf[3])<<24) + 20
			if want > 1<<20 {
				return nil, fmt.Errorf("invalid packet size %d", want)
			}
			if len(r.buf) >= want {
				p, err := a9.Unmarshal(r.buf[:want])
				r.buf = r.buf[want:]
				if err != nil {
					return nil, err
				}
				if p.Cmd == 100 { // heartbeat, ignore
					continue
				}
				return p, nil
			}
		}
		n, err := r.conn.Read(tmp)
		if n > 0 {
			r.buf = append(r.buf, tmp[:n]...)
		}
		if err != nil {
			if err == io.EOF && len(r.buf) > 0 {
				return nil, io.ErrUnexpectedEOF
			}
			return nil, err
		}
	}
}
