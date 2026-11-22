# Changelog

## 3.2.31

- Fix: direct Wi-Fi setup now sends a reboot command (`code 299`) after the camera accepted the new
  SSID and password. Previously the camera only stored the settings, stayed in AP mode and never
  applied them, while the UI already showed success. The set-wifi reply is now validated, so a
  rejected change no longer reports success either.

## 3.2.30

- Fix: direct Wi-Fi setup now enables the Wi-Fi interface and requests DHCP. Without `enabled: true`
  and `ipv4.method: auto`, Home Assistant could join the camera AP but never receive an address, so
  the setup stopped with "camera AP did not assign an address". The address probe also got more time
  and a clearer error message.
- Security: camera commands are POST-only and restricted to known codes; cross-origin writes,
  oversized request bodies and redirect leaks of the Supervisor token are rejected. Sessions now use
  `SameSite=Strict`, expire after 24 hours and can be cleared with `POST /logout`. Access settings are
  stored atomically with private file permissions, invalid UIDs are rejected and revoking network
  access stops running direct-link streams. The UI adds security headers and several input checks.

## 3.2.29

- Change: clicking **Reboot** now disables the button and shows a spinner. It becomes usable again
  once the camera reconnects (with a fallback timeout and a retry after a failed request).

## 3.2.28

- Fix: IR LED, status LED and image flip settings are now saved in `/data/settings.json` and sent to
  the camera again after a reboot or reconnect, instead of falling back to the camera defaults.

## 3.2.27

- New: when a stream is started through a direct link or another client, the camera card now mirrors
  it automatically. It stops again once the other viewer leaves, so the card never keeps the camera
  streaming on its own. A manual stop is respected until a new stream starts.
- Change: turning **Network access** back on restores the safe default for **Password required**
  (on).

## 3.2.26

- Change: the **Password required** switch is now hidden while **Network access** is off, since
  external requests are blocked with HTTP 403 anyway. It reappears as soon as network access is on.

## 3.2.25

- Change: removed the duplicate "Flip image" switch from **Controls**. The icon in the top right of
  the preview is now the only control for the image orientation and stays in sync with the camera.

## 3.2.24

- Change: the **LIVE** badge now follows the server's real stream state from `/dev/list`. It also
  lights up when the stream was started through a direct link or another client, not only by a click
  in the web UI.

## 3.2.23

- Change: clicking anywhere on the camera preview now starts or stops the stream. The play triangle
  stays as a visual hint; the snapshot and flip icons keep their own actions.

## 3.2.22

- New: a flip icon in the top right of the preview toggles the image orientation. It stays in sync
  with the "Flip image" switch under Controls.

## 3.2.21

- Change: clicking the preview image while a stream is running stops it and restores the snapshot.
- Fix: the camera icon now opens the snapshot in a dialog inside the page. Opening an Ingress URL in
  a new tab returned 401 because that tab had no Home Assistant ingress session.
- Fix: **Network access off** now blocks the direct links for external browsers even when they still
  have a login session for port 8557. Ingress and local requests continue to work.

## 3.2.20

- New: each camera card has a **Network access** switch (default on) that hides the direct links and
  blocks anonymous access to them when off, and a **Password required** switch (default on). With the
  password off, only that camera's MJPEG and snapshot URLs work without a login; the web UI stays
  protected. Settings are saved in `/data/access.json`.
- Change: the page heading is now just "Cameras" below the "A9 Camera · Overview" eyebrow.

## 3.2.19

- Change: the play triangle is only shown while the stream is stopped and is now a plain icon
  without a circle. Snapshot moved to a small camera icon in the lower left of the preview.

## 3.2.18

- Change: the firmware is now shown below the camera name as "Firmware <version>"; the IP address
  sits alone on the right side of the card header.

## 3.2.17

- Change: live video is started/stopped with a play/stop button directly on the camera preview; the
  LIVE badge stays in the image and the status dot before the host is gone.
- Change: IR LED, status LED and image flip are switches instead of pairs of buttons, initialized from
  the camera's reported state. The firmware version is now a compact badge without the "fw" label.

## 3.2.16

- Change: removed the redundant "N camera(s) connected" line below the header. The camera count is
  now only shown once as "Cameras: N" under the title.

## 3.2.15

- Change: the camera cards now use the same design system as the rest of the UI – shared colors,
  radii, shadows and typography, a preview area with a Live indicator, and grouped
  **Streams**/**Controls** sections. Live, Snapshot, commands and the Wi-Fi dialog still work as
  before.

## 3.2.14

- Change: the camera cards no longer show the Wi-Fi inputs permanently. A **Change Wi-Fi** button
  opens a modal dialog with the SSID/password form; sending works the same as before.

## 3.2.13

- Change: the Add Camera page now lists the steps in the order DNS rewrites, home Wi-Fi (2.4 GHz,
  WPA2 at most, no WPA3), host check, camera AP mode. Step 1 checks `/api/status`: when the rewrite is
  correct its block turns green and shows the IP; otherwise the rewrite instructions stay.
- Change: the local helper block stays visible as an alternative even when direct AP setup is
  available.

## 3.2.12

- Change: the overview header now shows a compact green **DNS-Rewrite ✓** badge next to the title
  and the Add Camera button, instead of the domain/IP line.

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
