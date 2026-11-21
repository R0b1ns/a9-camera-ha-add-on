# AGENTS.md – A9 Camera Home Assistant Add-on (Fork)

> Verbindliche Projektanweisung für dieses Repository. Alles aus dem alten
> `a9-v702`-Fake-Server-Projekt ist nur noch Historie und hier nicht relevant.

## Ziel

Fork von `dbuezas/a9-camera-ha-add-on` weiterentwickeln und später einen PR
stellen. Zwei Änderungen:

1. **Auth:** Direktlinks (z. B. `http://<ha>:8557/dev/<uid>/snapshot`) bleiben
   erhalten, erfordern aber einen Home-Assistant-Login.
2. **Wi-Fi-Setup:** Ein Weg, die Kamera ins WLAN zu bringen.

## Harte Regeln

- **Nichts installieren** (kein brew/pip/apt/go-Download).
- **Commits in jedem Zwischenschritt** (User-Vorgabe 21.11.2026). **Pushes nur
  nach ausdrücklicher Freigabe.** Schrittweise arbeiten, Zwischenstand zeigen.
- Kommunikation auf Deutsch.
- Keine Verbindung zur echten Naxclow-Cloud.
- Hardware-Aktionen (Kamera neu starten, AP-Mode) nur nach Absprache.

## Aktueller Stand

- Repo: `https://github.com/R0b1ns/a9-camera-ha-add-on`
- `feature/secured-ui-wifi-setup`: `f228668` → `2b2a9ea` (UI-Badges/DNS-Alert)
  → `8be81e0` (Supervisor-Scan) → aktuell (AP-Client + Helper). PR-Branch.
- `main`: `67ce41b` (zusätzlich lokaler Go-Build via `a9-cam/Dockerfile`,
  weil HA den Default-Branch `main` liest und vorher das dbuezas-v3.1.7-Image
  zog). Version 3.2.0. **Noch kein PR.**
- Feature-Branch Version 3.2.1. Pushes nur nach Freigabe.
- Go ist hier nicht installiert → `go test` lokal nicht möglich, PR-CI muss prüfen.
- `gh` ist nicht installiert.

## Wi-Fi-Setup – drei Wege (Stand 21.11.2026)

1. **Kamera online:** `POST /api/devices/<uid>/wifi` → `code 204` über die
   bestehende TCP-Session.
2. **Kamera im AP-Mode, HA hat WLAN:** `GET /api/wifi/accesspoints` nutzt
   Supervisor `/network/interface/<iface>/accesspoints`, `POST
   /api/wifi/configure-ap` verbindet HA kurz mit dem Kamera-AP, sendet
   `LIVE_MOTION` → `code 501` → `code 502{content:{code:204}}` an
   `192.168.169.1:6123` und verbindet danach zurück. **Die Supervisor-API gibt
   kein gespeichertes WLAN-Passwort zurück** (`wifi_struct()` liefert nur
   mode/auth/ssid/signal); ohne `home_password` würde der Restore das
   Kamera-AP-Passwort im Heimnetz-Profil lassen. Deshalb muss die UI es abfragen.
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
