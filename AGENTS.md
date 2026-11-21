# AGENTS.md – A9 Camera Home Assistant Add-on (Fork)

> Verbindliche Projektanweisung für dieses Repository. Alles aus dem alten
> `a9-v702`-Fake-Server-Projekt ist nur noch Historie und hier nicht relevant.

## Ziel

Fork von `dbuezas/a9-camera-ha-add-on` weiterentwickeln. Zwei Änderungen:

1. **Auth:** Direktlinks (z. B. `http://<ha>:8557/dev/<uid>/snapshot`) bleiben
   erhalten, erfordern aber einen Home-Assistant-Login.
2. **Wi-Fi-Setup:** Ein Weg, die Kamera ins WLAN zu bringen.

## Harte Regeln

- **Nichts installieren** (kein brew/pip/apt/go-Download).
- **Commits in jedem Zwischenschritt** (User-Vorgabe 21.11.2026). Schrittweise
  arbeiten, Zwischenstand zeigen.
- Kommunikation auf Deutsch.
- Keine Verbindung zur echten Naxclow-Cloud.
- Hardware-Aktionen (Kamera neu starten, AP-Mode) nur nach Absprache.

## Aktueller Stand

- Repo: `https://github.com/R0b1ns/a9-camera-ha-add-on`
- `main`: Version 3.2.1 mit Auth, Wi-Fi-Setup und lokalem Go-Build via
  `a9-cam/Dockerfile` (HA liest den Default-Branch `main`; vorher zog es das
  dbuezas-v3.1.7-Image). Letzter Stand `c2bb75b`, auf `fork/main` gepusht.
- `origin` ist der Upstream `dbuezas/a9-camera-ha-add-on` – **nie dorthin pushen**.
- Der Feature-Branch `feature/secured-ui-wifi-setup` war vollständig in `main`
  gemergt (`f1847d0`) und wurde lokal und auf `fork` gelöscht. Kein PR geplant.
- `a9-cam/Dockerfile` klont beim Build `main` (`A9CAM_REF=main`): Code-Änderungen
  wirken in HA erst nach einem Add-on-Update/Rebuild. Die Versionsanzeige 3.2.1
  kommt aus `config.yaml` und beweist nicht, dass das Image den neuen Code enthält.
- Go ist hier nicht installiert → `go test` lokal nicht möglich.

## Add-on-UI: Status-Badges & DNS-Alert (offen)

- Nutzer meldet: Add-on zeigt Version 3.2.1, auf der Kamera-Übersicht
  („A9 Cameras“, `GET /{$}`) sind aber **keine Badges** sichtbar – auch nicht
  die Platzhalter „Cameras: …“ / „Cloud DNS: …“. Auf `Wi-Fi setup` gibt es
  bewusst keine Badges, nur den Text „Checking Home Assistant's Wi-Fi…“.
- Implementierung: `server/web/index.html` Zeilen 61–66 (`badge-cams`,
  `badge-dns`, `badge-wifi`, `<div id="alert">`), befüllt in `update()`
  (Zeilen 166–189). Datenquelle: `GET /api/status` (`server/status.go`).
- Der Alert erscheint **nur bei `dns_ok == false`**; bei korrekten Rewrites
  bleibt er leer. `dns_ok` = `v720.naxclow.com` löst (2 s Timeout,
  `net.DefaultResolver`) auf eine der lokalen IPv4-Adressen auf.
- `wifi_available` liefert `wifiInterface()`: UP-Interface mit `wl*`,
  `wireless` oder `wifi` im Namen. Sonst zeigt das Badge „HA WLAN: none“.
- Fällt `fetch("dev/list")` aus, bricht `update()` vor dem Setzen der Badges
  ab und schreibt „Server unreachable“; die Badges bleiben auf „…“.
  `api/status` hat zwar einen `.catch(() => null)`, aber die Badges werden
  nur innerhalb von `if (st)` gesetzt.
- Alle relativen URLs (`dev/list`, `api/status`) setzen einen Ingress-Pfad mit
  abschließendem `/` voraus; ohne Slash gehen die Requests unter
  `/api/hassio_ingress/...` verloren.
- Bekannter Nebenfehler: `server/main.go` kennt kein `GET /index.html`; der
  Link „← Back to cameras“ in `setup.html` (`href="index.html"`) läuft in 404.
- Nächste Prüfschritte: Browser-Konsole/Network ansehen, Antwort von
  `/api/status` und `dev/list` prüfen, `view-source` auf `badge-dns` prüfen,
  ggf. Add-on neu bauen/installieren. Kein Neustart oder Hardware-Aktion ohne
  Absprache.

## Wi-Fi-Setup – drei Wege (Stand 21.11.2026)

1. **Kamera online:** `POST /api/devices/<uid>/wifi` → `code 204` über die
   bestehende TCP-Session.
2. **Kamera im AP-Mode, HA per LAN + freies WLAN (Normalfall bei HA OS auf dem
   Raspberry Pi):** `GET /api/wifi/accesspoints` nutzt Supervisor
   `/network/interface/<iface>/accesspoints`, `POST /api/wifi/configure-ap`
   prüft `connected == false`, verbindet HA kurz mit dem Kamera-AP, sendet
   `LIVE_MOTION` → `code 501` → `code 502{content:{code:204}}` an
   `192.168.169.1:6123` und schaltet WLAN danach wieder ab (`enabled: false`).
   Ein aktives Heimnetz-WLAN wird mit HTTP 409 abgelehnt, weil Supervisor das
   Passwort nie zurückgibt und ein Restore sonst das Kamera-Passwort übernähme.
   `hassio_api: true` + `hassio_role: manager` sind nötig, `host_network: true`
   ist bereits gesetzt.
3. **HA hat kein WLAN:** Download-Helfer `server/web/helper/a9-wifi-setup.py`
   (stdlib-only, keine Parameter) mit `.bat`/`.command`-Launchern. Der Nutzer
   verbindet seinen Rechner mit dem Kamera-AP, startet den Helfer und gibt SSID
   und Passwort ein.

**Reines HTML/JS direkt gegen den Kamera-AP geht nicht:** Browser können kein
rohes TCP auf `192.168.169.1:6123`; das war der Ursprung der Helfer-Idee.
Ein iPhone ohne Heimnetz erreicht weder HA noch den Helfer – das ist der Grund,
warum Weg 3 einen separaten Rechner voraussetzt.

## Dateien

- `server/main.go` – Muxes, Ports, `authMiddleware`-Einbindung
- `server/auth.go` – Login-Seite, Supervisor `POST /auth`, Session-Cookie
- `server/api.go` – `apiWiFi()` (code 204 über bestehende TCP-Session)
- `server/web/index.html` – Karten mit absoluten Direktlinks auf `:8557`
- `server/web/setup.html` – jetzige Setup-Seite (noch HA-gebunden)
- `a9-cam/config.yaml` – `auth_api: true`, `8557/tcp: 8557`, lokaler Build
- `a9-cam/DOCS.md`, `CHANGELOG.md` – Doku zu 3.2.1
- `server/status.go` – `/api/status` (DNS-Check, WLAN-Interface)
- `server/supervisor.go` – Supervisor-Client (Scan, Connect, Restore)
- `server/wifi_ap.go` – AP-Client (`code 502` → `code 204`)
- `server/web/helper/*` – downloadbarer Standalone-Helfer
