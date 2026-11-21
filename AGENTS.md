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
- `feature/secured-ui-wifi-setup`: `f228668` (PR-Branch, sauber)
- `main`: `67ce41b` (zusätzlich lokaler Go-Build via `a9-cam/Dockerfile`,
  weil HA den Default-Branch `main` liest und vorher das dbuezas-v3.1.7-Image
  zog). Version 3.2.0. **Noch kein PR.**
- Go ist hier nicht installiert → `go test` lokal nicht möglich, PR-CI muss prüfen.
- `gh` ist nicht installiert.

## Wichtig: offene Entscheidung Wi-Fi-Setup

Die jetzige `setup.html` hängt an der HA-Oberfläche. Das ist unbrauchbar, sobald
das Gerät für den Kamera-AP das Heimnetz verlässt. Gewünscht ist **ein Link**,
der einmal im Browser geöffnet bleibt, während WLAN getrennt und der Kamera-AP
verbunden wird. Danach soll die Seite die Kamera erkennen und den Wi-Fi-Wechsel
auslösen.

**Technische Realität (geprüft):** Das geht mit **reinem HTML/JS nicht**.
Die Kamera im AP-Mode (`192.168.169.1`) spricht nur **rohes TCP auf Port 6123**
mit eigenem Binär-Format (`prot_ap` im Alt-Projekt `a9-v720/src/v720_ap.py`,
`set_wifi()` = `code 204`). Browser können kein TCP, weder `fetch()` noch
WebSocket. In JADX-Quellen und `docs/uart.log` gibt es keinen HTTP-Endpunkt
auf der Kamera. Ein Chrome-Experiment (`TCPSocket`/Direct Sockets) existiert,
ist aber nicht standardisiert und auf iOS nicht verfügbar.

→ **Geklärt (User, 21.11.2026):** Das Szenario war ein iPhone. Sobald das
Heimnetz-WLAN weg ist, ist HA und jeder Helfer im Heimnetz für das iPhone
unereichbar. Mit nur HTML/JS geht es damit **gar nicht**. Die AP-Variante ist
hiermit endgültig verworfen. Es bleibt ausschließlich der Wi-Fi-Wechsel über
die bestehende TCP-Session, solange die Kamera noch im Heimnetz online ist
(`apiWiFi()` / code 204). Kein Helfer, kein Chrome-Experiment, keine
Diskussion mehr darüber.

## Dateien

- `server/main.go` – Muxes, Ports, `authMiddleware`-Einbindung
- `server/auth.go` – Login-Seite, Supervisor `POST /auth`, Session-Cookie
- `server/api.go` – `apiWiFi()` (code 204 über bestehende TCP-Session)
- `server/web/index.html` – Karten mit absoluten Direktlinks auf `:8557`
- `server/web/setup.html` – jetzige Setup-Seite (noch HA-gebunden)
- `a9-cam/config.yaml` – `auth_api: true`, `8557/tcp: 8557`, lokaler Build
- `a9-cam/DOCS.md`, `CHANGELOG.md` – Doku zu 3.2.0
