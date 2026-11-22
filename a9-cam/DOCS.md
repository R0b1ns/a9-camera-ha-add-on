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
   shows up as a card with a live preview; clicking anywhere on the image starts the stream and
   clicking again stops it. The **LIVE** badge uses the server's real stream state. When a stream is
   started through a direct link or another client, the card mirrors it automatically and stops again
   when the other viewer leaves, so it never keeps the camera streaming on its own. The play triangle
   is only a hint while stopped. The camera icon in the lower left opens the current snapshot in a
   dialog (and lets you download it) without leaving the page – opening an Ingress URL in a new tab
   would get a 401 because that tab has no Home Assistant ingress session. Its URLs are
   grouped under **Streams**, while **Controls** has switches for IR LED and status LED plus Reboot
   and Change Wi-Fi. Image flip is the icon in the top right of the preview. You can also open the
   URLs directly on
   port 8557 after a one-time Home Assistant login (see **Access control**).
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
The add-on page shows the full direct URLs as text; the first open asks for the Home Assistant login.

Each camera card has two switches under **Streams**:

- **Network access** (default on): when off, the direct links are hidden in the UI and requests
  from outside the host are rejected with HTTP 403 – even from a browser that logged in earlier.
  Requests through Ingress or from the host itself still work, so the UI can show its preview.
- **Password required** (default on): when off, only that camera's `/dev/CAMERA_ID/live` and
  `/dev/CAMERA_ID/snapshot` work without a login. The web UI and the integration API always require
  the Home Assistant login. Ingress and local requests are unaffected by both switches. This switch
  is hidden while network access is off, because external requests are blocked anyway. Turning
  network access back on restores the password default (on).

The switches are stored per camera in `/data/access.json`.

The Home Assistant login session lasts 24 hours. `POST /logout` clears the browser cookie; the
signed token itself cannot be revoked before it expires. When the UI is reached over HTTPS, the
cookie is marked `Secure`.

If your setup sends ingress traffic from a different address (for example a reverse proxy), add that
network to the `ui_allowed_cidrs` option, e.g. `192.168.1.0/24`. Those addresses are treated as already
authenticated. The option extends the built-in allows; it never removes them. **Do not use broad
ranges such as `0.0.0.0/0`**: every address listed here skips the login *and* both per-camera
switches, including the Wi-Fi and command APIs.

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

`POST http://HA_IP:8557/dev/CAMERA_ID/cmd?code=...` sends a command to the camera. A GET is rejected,
so an external link cannot change camera state through a logged-in browser. Only the codes below are
accepted; changing Wi-Fi (`code=204`) is reserved for the dedicated APIs. The web UI has buttons for
these:

| Command | URL query |
| --- | --- |
| Infrared on / off | `code=202&IrLed=1` / `code=202&IrLed=0` |
| Flip image | `code=216&mirrorFlip=4` / `code=216&mirrorFlip=0` |
| Status LED on / off | `code=210&instLed=1` / `code=210&instLed=0` |
| Reboot | `code=299&reboot=1` |

The infrared, status LED and flip settings are saved by the add-on in `/data/settings.json` and sent
to the camera again whenever it connects, so a reboot (or a camera that reboots itself) does not reset
them. After clicking **Reboot**, the button shows a spinner and stays disabled until the camera
reconnects.

## Changing the camera's Wi-Fi

For a camera that is **already connected**, click **Change Wi-Fi** on its card on the camera
overview page and enter the new SSID and password in the dialog that opens. The add-on sends code
`204` over the existing connection and the camera reconnects on the new network.

For a **new camera in its own AP mode (`NAXCLOW`, not configured yet)**, power it on and hold the
**Mode** button for about **5 seconds**. Then click **Add Camera** in the add-on web page
(or go to `setup.html` in the same Ingress URL). That page is only for new devices and lists four
steps: keep the DNS rewrites (checked via `/api/status`: when correct the block turns green and shows
the IP, otherwise the rewrite instructions stay), choose a **2.4 GHz** network with **WPA2** at most
(8–63 characters, no 5 GHz/WPA3), check this Home Assistant, and put the camera into AP mode.
The host check uses `/api/status`: when HA is connected by **LAN** and has a Wi-Fi interface that is
**not** connected, it offers the direct **Connect the camera** form; otherwise only that form is
hidden. The local helper stays available as an alternative in both cases. In the direct flow, click
**Scan Wi-Fi**, pick the camera's access point, then enter your home network and password. The add-on
joins the camera AP via the Supervisor network API, sends
code `204` over TCP
`6123`, and disconnects Wi-Fi again afterwards. HA stays reachable over LAN throughout. It needs the
`hassio_api` permission with the `manager` role; a Wi-Fi connection that appears while the page is
open is still refused by the API.
If direct setup is not available, download the standalone helper from the setup page:
`a9-wifi-setup.py` plus a `.bat` (Windows) or `.command` (macOS) launcher. Join the camera AP with that
computer, run the helper, wait for the camera and enter the new SSID and password when asked. No
parameters and no Python installation are needed (Python 3 ships with macOS and most Linux
distributions; the Windows launcher finds it). If macOS refuses to open the downloaded `.command`
file, run `chmod +x a9-wifi-setup.command` once in Terminal.

The add-on UI shows the number of connected cameras under its title and a green **DNS-Rewrite ✓**
badge in the header. If `v720.naxclow.com` does not resolve to this host, the badge turns red with
`✗` and a warning explains which DNS rewrites are missing.

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
| 8557 | Web UI, MJPEG, snapshots, integration API. Published on the host and protected by the HA login; prefer Ingress. |

Port 8556 has no authentication at all. RTSP is off until you map that port, and anyone who can reach
it can view every camera. Port 6123 (camera protocol) also has no authentication; keep it inside a
trusted network.

## Reconnecting

After the add-on restarts, cameras come back by themselves. Some firmware versions wait about 8 minutes
before they retry. Restarting the camera brings it back at once.

## Credits

Camera protocol reverse engineered by https://github.com/intx82/a9-v720.
The stream code uses parts of [go2rtc](https://github.com/AlexxIT/go2rtc).
