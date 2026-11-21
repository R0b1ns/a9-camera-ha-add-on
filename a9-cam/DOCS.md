# A9 Camera fake server

This add-on pretends to be the cloud server of the cheap A9 / V720 "Naxclow" mini cameras.
The cameras connect to it instead of China, and it serves their video and audio on your network.

## Setup

1. Connect the camera to your WiFi (with the V720 app, or see https://github.com/intx82/a9-v720).
2. Point `*.naxclow.com` to the IP of your Home Assistant machine.
   With the AdGuard add-on: Filters → DNS rewrites → `*.naxclow.com` → your HA IP.
   Do the same for `*.naxclow.com.cn`: newer firmware uses it, for example for firmware updates.
   Your router must give AdGuard as the DNS server to the cameras.
3. Install and start this add-on.
4. Restart the cameras, or wait. After a server restart a camera can take up to about 10 minutes to
   come back.
5. Open the add-on web UI from the add-on panel in Home Assistant (Ingress). Each connected camera
   shows up with its ID and URLs. You can also open the URLs directly on port 8557 after a one-time
   Home Assistant login (see **Access control**).
6. Optional but recommended: install the **A9 Camera** integration from HACS (same repository) and
   restart Home Assistant. It finds this add-on by itself and adds each camera as a device with a live
   camera, switches and sensors.

## Access control

Direct links such as `http://HA_IP:8557/dev/CAMERA_ID/snapshot` still work, but now ask for a
**Home Assistant user name and password** on first use. The add-on checks them against the Supervisor's
auth API (`auth_api: true`) and then sets an HttpOnly session cookie valid for seven days. The password
is never stored in the add-on. Requests coming through Home Assistant Ingress (the Supervisor at
`172.30.32.2`) and the local integration (`127.0.0.1`) are already authenticated by Home Assistant and
skip the login form.

Port 80 serves only the cameras' cloud API, so old links without a port (for example
`http://HA_IP/dev/.../snapshot`) no longer expose video to the whole network. Always use port 8557.
The add-on page shows the full direct URLs again, with buttons to open MJPEG and snapshot; the first
open asks for the Home Assistant login.

If your setup sends ingress traffic from a different address (for example a reverse proxy), add that
network to the `ui_allowed_cidrs` option, e.g. `192.168.1.0/24`. Those addresses are treated as already
authenticated. The option extends the built-in allows; it never removes them.

## Streams

Replace `CAMERA_ID` with the ID shown in the web UI (for example `0800c0035826`).

| What | URL |
| --- | --- |
| RTSP, MJPEG video + PCMA audio | `rtsp://HA_IP:8556/CAMERA_ID` |
| MJPEG over HTTP | `http://HA_IP:8557/dev/CAMERA_ID/live` |
| JPEG snapshot | `http://HA_IP:8557/dev/CAMERA_ID/snapshot` (add `?max_age=60` to reuse a recent frame) |
| Camera list (JSON) | `http://HA_IP:8557/dev/list` |

The camera only sends video while someone watches. All viewers of one camera share one stream.
These URLs ask for a Home Assistant login on first use (see **Access control**). RTSP is a separate
protocol without a login, so its port is off by default; enable it in the add-on's **Network** section
only if you trust the network and need it (e.g. for go2rtc).

### go2rtc / WebRTC card

Browsers need H.264 for WebRTC, so let go2rtc convert the video:

```yaml
streams:
  a9_camera: ffmpeg:rtsp://127.0.0.1:8556/CAMERA_ID#video=h264#audio=copy
```

### Generic Camera integration

- Stream source: `rtsp://127.0.0.1:8556/CAMERA_ID`
- Still image: `http://127.0.0.1:8557/dev/CAMERA_ID/snapshot`

## Commands

`http://HA_IP:8557/dev/CAMERA_ID/cmd?code=...` sends a command to the camera (only reachable through
Ingress or localhost). The web UI has buttons for these:

| Command | URL query |
| --- | --- |
| Infrared on / off | `code=202&IrLed=1` / `code=202&IrLed=0` |
| Flip image | `code=216&mirrorFlip=4` / `code=216&mirrorFlip=0` |
| Status LED on / off | `code=210&instLed=1` / `code=210&instLed=0` |
| Reboot | `code=299&reboot=1` |

## Changing the camera's Wi-Fi

For a camera that is **already connected**, use the **Change Wi-Fi** form directly on its card on
the camera overview page. Enter the new SSID and password there; the add-on sends code `204` over
the existing connection and the camera reconnects on the new network.

For a **new camera in its own AP mode (`NAXCLOW`, not configured yet)**, power it on and hold the
**Mode** button for about **5 seconds**. Then open **Add new camera (AP mode)** in the add-on web page
(or go to `setup.html` in the same Ingress URL). That page is only for new devices. The HA host must be
connected by **LAN** with its Wi-Fi unused (the normal Home Assistant OS setup). Click **Scan Wi-Fi**,
pick the camera's access point (**no AP password – the network is open**), then enter your 2.4 GHz
home network and its password. The add-on joins the camera AP via the Supervisor network API, sends
code `204` over TCP
`6123`, and disconnects Wi-Fi again afterwards. HA stays reachable over LAN throughout. It needs the
`hassio_api` permission with the `manager` role; if Wi-Fi is already in use, the request is refused.
If the host has no usable Wi-Fi, download the standalone helper from the setup page:
`a9-wifi-setup.py` plus a `.bat` (Windows) or `.command` (macOS) launcher. Join the camera AP with that
computer, run the helper, wait for the camera and enter the new SSID and password when asked. No
parameters and no Python installation are needed (Python 3 ships with macOS and most Linux
distributions; the Windows launcher finds it). If macOS refuses to open the downloaded `.command`
file, run `chmod +x a9-wifi-setup.command` once in Terminal.

The add-on UI also shows badges for the number of connected cameras, the state of the cloud DNS
rewrite and whether the HA host has Wi-Fi. If `v720.naxclow.com` does not resolve to this host, a red
warning explains which DNS rewrites are missing.

Notes:

- Only **2.4 GHz** networks are supported by these cameras.
- Keep the DNS rewrites for `*.naxclow.com` and `*.naxclow.com.cn` in the new network's DHCP DNS.
- The AP method requires LAN and a free Wi-Fi interface; it never changes an active home Wi-Fi.
- Wi-Fi passwords are sent to the camera and are never written to the add-on log.

## Upgrading from version 2.x

Version 3 is a rewrite in Go. It does not use Python or FFmpeg anymore.

- `/dev/CAMERA_ID/live`, `/snapshot`, `/cmd` and `/dev/list` work as before.
- `/dev/CAMERA_ID/go2rtc-stream`, `/stream`, `/browser-stream` and `/audio` are gone (they needed FFmpeg).
  Use the RTSP URL instead, see the go2rtc example above.

## Options

| Option | Default | What it does |
| --- | --- | --- |
| `log_level` | `info` | `debug` shows every camera message and stream statistics |
| `camera_http_port` | `80` | Port for the cameras' cloud calls. They always call port 80, so only change it if another add-on uses port 80 and forwards `/app/api/` to this add-on (port 8557). `0` turns it off. |
| `ui_allowed_cidrs` | empty | Comma-separated extra networks allowed to use the web UI and API, e.g. `192.168.1.0/24`. Loopback and the Supervisor are always allowed. |

## Audio

Each camera has an **Audio** switch in the integration. Off asks the camera for video only, so its
microphone audio never leaves the camera. The setting is kept when the add-on restarts.

## Ports

The add-on uses the host network:

| Port | What |
| --- | --- |
| 80 | Cloud calls of the cameras only (see `camera_http_port`) |
| 6123 TCP + UDP | Camera connection |
| 8556 | RTSP, off by default |
| 8557 | Web UI, MJPEG, snapshots, integration API, off by default (use Ingress) |

## Reconnecting

After the add-on restarts, cameras come back by themselves. Some firmware versions wait about 8 minutes
before they retry. Restarting the camera brings it back at once.

## Credits

Camera protocol reverse engineered by https://github.com/intx82/a9-v720.
The stream code uses parts of [go2rtc](https://github.com/AlexxIT/go2rtc).
