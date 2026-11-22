# Changelog

## 3.2.11

- Change: the overview shows "Cameras: N" as muted text below the title and
  `DNS-Rewrite <domain> => <ip> ✓` instead of status badges. The HA Wi-Fi badge is gone.

## 3.2.10

- Change: the new-camera page now checks the host itself (`/api/status`: `lan_connected`,
  `wifi_connected`, `ap_ready`). It shows the direct "Connect the camera" form only when HA is on LAN
  and its Wi-Fi is free; otherwise it shows the local helper with the reason.

## 3.2.9

- Change: removed the redundant note about the camera AP having no password from the setup page.
- Fix: emphasized words inside the steps (e.g. "5 seconds") no longer break onto their own line.

## 3.2.8

- Fix: "Back to cameras" no longer returns 404. In-page URLs were resolved against the page file
  (`/setup.html/index.html`); they now resolve against its directory.
- Change: the overview has an "Add Camera" button instead of a plain text link, and an empty-state
  card when no camera is connected.

## 3.2.7

- Fix: the camera's AP-mode network is open, so the AP password field is gone. The add-on now joins it
  with `auth: open` and no PSK (a supplied `ap_password` is still accepted for protected APs).
- Change: redesigned the new-camera setup page with numbered steps (hold Mode for 5 s, LAN requirement,
  2.4 GHz Wi-Fi, DNS rewrites) and status messages next to the relevant fields.

## 3.2.6

- Fix: the login page no longer shows `%!s(MISSING)`. The CSS `width:100%` was parsed by
  `fmt.Sprintf` as a placeholder; fixed placeholders are now replaced with `strings.NewReplacer`
  (regression test in `server/auth_test.go`).
- Change: the Wi-Fi form is now directly on each connected camera's card. The setup page is only
  for new cameras in AP mode; the generic camera list is gone.
- Change: removed the "Open MJPEG" / "Open snapshot" buttons and the extra label text from the
  camera cards; the direct URLs remain as plain text.

## 3.2.5

- Change: the add-on image build now uses BuildKit cache mounts for Go modules and compiled
  packages. Updates reuse the cache instead of recompiling all of go2rtc every time.

## 3.2.4

- Fix: Go compile error in the Wi-Fi AP endpoint (`undefined: err`) broke the 3.2.3 image build.

## 3.2.3

- Fix: the add-on image now really contains the current `main` code. Docker had cached the
  `git clone` layer, so version bumps rebuilt an old binary without the link/UI fixes.

## 3.2.2

- Fix: „Back to cameras“ on the Wi-Fi setup page no longer returns 404 (`GET /index.html`).
- Fix: direct MJPEG and snapshot links are shown as clickable URLs again; the first open asks for the
  Home Assistant login.
- Fix: Ingress URLs work with and without a trailing slash; status badges also appear when
  `/api/status` is unavailable.
- Docs: the Generic Camera still-image URL now uses port 8557.

## 3.2.1

- New: the add-on UI shows badges for connected cameras, the cloud DNS rewrite and the HA host's
  Wi-Fi, plus an alert when `v720.naxclow.com` does not point at this host.
- New: a camera that is still in its own `NAXCLOW` AP mode can be configured from the UI. With HA on
  LAN and Wi-Fi unused, the add-on scans for access points, joins the camera AP, sends the Wi-Fi
  credentials over TCP 6123 and disconnects Wi-Fi again (needs `hassio_api` with `manager`). An active
  home Wi-Fi is left untouched.
- New: downloadable standalone helper (`a9-wifi-setup.py` with `.bat`/`.command` launchers) for hosts
  without Wi-Fi. Join the camera AP with that computer, run the helper and answer the prompts.

## 3.2.0

- **Security:** direct links to port 8557 (`/dev/.../snapshot`, `/live`, UI) now require a Home
  Assistant login. The add-on validates the user through Supervisor's auth API and sets a session
  cookie; Ingress and the local integration remain automatically trusted. Port 80 serves only the
  cameras' cloud API, so links without a port no longer expose video. RTSP (8556) stays off by default.
- New: Wi-Fi setup page in the add-on UI. It sends code `204` to a connected camera, so the network
  can be changed without reconnecting a phone to the camera's access point. Includes a short guide.

## 3.1.7

- Fix: the Live button in the add-on page showed nothing (#17). Home Assistant's add-on panel
  compresses the MJPEG stream and removes its boundary, so the browser got no pictures. The page
  now reads the stream itself and shows each picture.

## 3.1.6

- Fix: WebRTC (go2rtc) often froze on the first frame until a reconnect. FFmpeg guesses the frame
  rate from the first frames, and with irregular times it sometimes guessed 90000 fps and sent
  hundreds of copies of each frame. Video times are now whole frames (10 fps), so the guess is right.
- Times start at the first video frame. Audio before it is dropped, so FFmpeg does not fill the
  start with copies of the first frame.

## 3.1.5

- Video and audio times follow the clock. (This did not fix the WebRTC freeze, 3.1.6 does.)

## 3.1.4

- Faster stream start: when the camera answers "busy" (it is still closing the previous stream),
  the add-on asks again every second instead of waiting 5 seconds.
- Frames without the JPEG end marker are dropped, so a broken frame does not show up as a grey image.
- Status LED switch: also sends code 220 (`ledEI`), which newer firmware uses instead of 210.
- Docs: also redirect `*.naxclow.com.cn`, and links for other cameras sold as "A9".

## 3.1.3

- Newer firmware (2024 and later) first asks `getDevInfo`. The add-on now gives the answer that
  works (thanks to 0x3dlux in intx82/a9-v720#51), so these cameras can go on to connect.

## 3.1.2

- Fix: with Audio off, the camera sent no video. Cameras do not start a stream in "video only" mode,
  so the stream now starts with audio and switches to video only after the first frame.

## 3.1.1

- Integration: an existing setup follows the add-on to its new port (8557) instead of showing
  "A9 Camera discovered" again. Only one setup is allowed.

## 3.1.0

- New: Audio switch per camera. Off asks the camera for video only, so no audio leaves it (#13).
- New: web UI, MJPEG, snapshots and the integration API also on port 8557. Option `camera_http_port`
  (default 80, 0 = off) for systems where another add-on uses port 80 (#1).
- Experimental: answer `getDevInfo` from newer camera firmware (#16).

## 3.0.0

- Rewrite in Go. No Python and no FFmpeg in the image anymore.
- New: RTSP on port 8556 with MJPEG video and PCMA audio, for go2rtc and the Generic Camera integration.
- One camera stream is shared by all viewers. The camera stops sending when nobody watches.
- The data channel restarts by itself when the video stops.
- New web UI with live view and buttons for IR, flip, LED and reboot.
- Option `log_level`.
- Removed: `/dev/CAMERA_ID/stream`, `/go2rtc-stream`, `/browser-stream` and `/audio`. Use the RTSP URL.

## 2.4

- Last Python version.
