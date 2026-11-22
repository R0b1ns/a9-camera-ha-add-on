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
- **Vor jedem Push Version bumpen** in `a9-cam/config.yaml`,
  `custom_components/a9_cam/manifest.json` und `a9-cam/CHANGELOG.md`, sonst
  zeigt Home Assistant kein Update an.
- Kommunikation auf Deutsch.
- Keine Verbindung zur echten Naxclow-Cloud.
- Hardware-Aktionen (Kamera neu starten, AP-Mode) nur nach Absprache.

## Aktueller Stand

- Repo: `https://github.com/R0b1ns/a9-camera-ha-add-on`
- `main`: Version 3.2.8 mit Auth, Wi-Fi-Setup, Link-/UI-Fixes, Login-Fix,
  neuem AP-Setup-UI und lokalem Go-Build via `a9-cam/Dockerfile` (HA liest den
  Default-Branch `main`; vorher zog es das dbuezas-v3.1.7-Image).
  `fork/main`: 3.2.33, gepusht (Sicherheitshärtung, Wi-Fi-AP-Fix + Reboot in UI und Helper).
- `origin` ist der Upstream `dbuezas/a9-camera-ha-add-on` – **nie dorthin pushen**.
- Der Feature-Branch `feature/secured-ui-wifi-setup` war vollständig in `main`
  gemergt (`f1847d0`) und wurde lokal und auf `fork` gelöscht. Kein PR geplant.
- `a9-cam/Dockerfile` klont beim Build `main` (`A9CAM_REF=main`). **Falle:** Docker
  cachete diese Layer, sodass ein Versionsupdate den alten Code neu verpackte.
  Seit 3.2.3 geht `BUILD_VERSION` in den `git clone`-Schritt ein und bricht den
  Cache – ohne das bleiben 404 und fehlende Badges trotz „Update“ sichtbar.
  Seit 3.2.5 nutzt der Build zusätzlich BuildKit-Cache-Mounts
  (`/root/.cache/go-build`, `/go/pkg/mod`), damit nicht jedes Update alle
  Go-Pakete neu kompiliert. HA baut aber weiter lokal – der endgültige Weg wäre
  ein GHCR-Prebuilt-Image (`image:` in `config.yaml`), wie es der Upstream macht.
  Die Versionsanzeige kommt aus `config.yaml` und beweist für sich allein nicht,
  dass das Image den neuen Code enthält.
- Go ist hier nicht installiert → `go test` lokal nicht möglich. 3.2.3 scheiterte
  im HA-Build an `api.go:154 undefined: err` (Variable nur im `if`-Scope
  deklariert); in 3.2.4 mit `err :=` behoben. Vor Pushes auf Go-Scope-Prüfung
  achten oder den HA-Build als einzige Prüfung ansehen.

## Add-on-UI: Status-Badges & DNS-Alert (Code behoben, Image-Prüfung offen)

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
- **Behoben in `23fe35d`:** Badges werden jetzt auch ohne `/api/status`
  gesetzt („unavailable“); `update()` nutzt eine Ingress-Basis mit `/`.
- **Behoben in `20aaa56`:** `GET /index.html` existiert jetzt, der
  „← Back to cameras“-Link aus `setup.html` liefert nicht mehr 404.
- Offen: Der neue Code muss in HA tatsächlich ausgeliefert werden (siehe
  Dockerfile-Hinweis oben). Danach Browser-Konsole/Network und
  `/api/status` prüfen. Kein Neustart oder Hardware-Aktion ohne Absprache.

## Vergleich 3.1.7 → 3.2.x: Kamer-Links / UI

- 3.1.7 (`3ed6a3c`) lieferte die komplette UI auch auf Port 80 aus
  („old links keep working“) – daher waren `/dev/.../live` und `/snapshot`
  öffentlich im Netzwerk erreichbar. Jetzt bedient Port 80 nur noch die
  Cloud-API der Kameras; UI und Streams liegen auf 8557 hinter dem HA-Login.
- Die Link-Zeilen in `index.html` selbst sind gegenüber 3.1.7 **unverändert**
  (`rtsp://host:8556/<uid>`, `http://host:8557/dev/<uid>/live` bzw.
  `/snapshot`). Es wurde also nichts entfernt; sichtbar war nur der
  relative Pfad aus dem Ingress-/Button-Kontext.
- Fehler gefunden und behoben:
  - `setup.html` verlinkt `index.html`, `main.go` hatte aber keine Route
    dafür → „404 page not found“ (`20aaa56`).
  - Relative URLs ohne abschließenden `/` im Ingress-Pfad gingen verloren;
    `index.html` baut In-Page-URLs jetzt über `rel()` mit fester Basis
    (`23fe35d`).
  - Direkte Links standen nur als Text im `<code>`-Block; jetzt zusätzlich
    klickbare Buttons „Open MJPEG“ und „Open snapshot“ mit
    `http://<host>:8557/...` (erster Aufruf → HA-Login, `23fe35d`).
  - `DOCS.md` enthielt noch `http://127.0.0.1/dev/.../snapshot` ohne Port;
    korrigiert auf `:8557` (`9c0b5e4`).
- **Behoben in `1ea93d4` / `69a3f68` / `a45f3e1` (3.2.6):** Login-Seite ohne
  `%!s(MISSING)` (CSS `width:100%` wurde von `fmt.Sprintf` als Platzhalter
  gelesen; jetzt `strings.NewReplacer` mit `{{NEXT}}`/`{{ERR}}`, Regressionstest
  in `server/auth_test.go`). Keine „Open MJPEG“/“Open snapshot“-Buttons und
  kein „(direct link, HA login)“ mehr; die direkten URLs bleiben als Text.
  „Change Wi-Fi“ direkt auf jeder Kamera-Karte; `setup.html` nur noch für neue
  Geräte im AP-Mode („Add new camera (AP mode)“).
- **Behoben in `1029b47` / `bfd6d93` (3.2.7):** Der Kamera-AP ist **offen** –
   kein AP-Passwort mehr in UI und API. `POST /api/wifi/configure-ap` sendet
   `auth: "open"` ohne `psk` (optionales `ap_password` für gesicherte APs bleibt
   aus Kompatibilitätsgründen erhalten; Regressionstest
   `TestWiFiAPFlowJoinsOpenCameraAP`). `setup.html` neu gestaltet: integrierte
   Schritte (Modus-Taste 5 s, LAN-Voraussetzung, 2,4-GHz-WLAN, DNS-Rewrites),
   Statusmeldungen direkt an den Feldern statt weit entfernter Ergebnisbox,
   URLs über `rel()` ingress-sicher.
- **Behoben in `1c03876` / `be6cccc` (3.2.8):** Übersicht mit „Add Camera“
  statt Textlink und Leerzustands-Karte. Der „← Back to cameras“-404 kam daher,
  dass `rel()` die Basis aus der Datei-URL statt aus dem Verzeichnis baute
  (`/setup.html/index.html`). Jetzt `new URL(".", location.href)`; Regression:
  `TestHTTPIndexHTML` prüft die Route, `TestHTTPSetupPage` den neuen Text.
  **Wichtig:** `@media`-Darkmode und alle relativen URLs hängen von dieser
  Basis ab – bei weiteren Seiten nicht wieder `pathname += "/"` verwenden.
- **Geändert in `c395899` / `501e14c` (3.2.10):** Statt des Textes „HA muss
  per LAN …“ prüft `setup.html` jetzt `/api/status`. Neu in `status.go`:
  `lan_connected` (UP-NIC mit IPv4, Docker/Hassio/Bridges ausgenommen),
  `wifi_connected` (Supervisor `/network/interface/<iface>/info`, Fallback
  lokale IP) und `ap_ready = LAN && WLAN vorhanden && !WLAN verbunden`.
  Nur bei `ap_ready` wird „Connect the camera“ gezeigt, sonst der Helfer
  mit Grund. `apiWiFiAP` lehnt aktives WLAN weiterhin mit 409 ab.
- **Geändert in `d1f8a0b` (3.2.12):** Übersicht zeigt nur noch ein kompaktes
  grünes Badge „DNS-Rewrite ✓“ in der Kopfzeile (Title links, Badge, „Add
  Camera“ rechts); keine Domain/IP in der UI. Bei Fehler rotes Badge mit ✗
  plus Alert. WLAN-Badge bleibt generell entfernt. `TestHTTPIndexHTML` prüft
  das Badge.
- **Geändert in `3d794fd` / `d02a957` (3.2.13):** `setup.html` zeigt die Schritte
  jetzt in der Reihenfolge DNS-Rewrites → Heim-WLAN (2,4 GHz, max. WPA2, kein
  WPA3) → HA-Prüfung → Kamera-AP-Modus. Schritt 1 liest `dns_ok`/`dns_ip` aus
  `/api/status` und wird bei korrekter Rewrite grün mit IP; sonst bleibt der
  Hinweis. Der Helper-Block bleibt immer sichtbar (auch bei `ap_ready`), nur
  der „Connect the camera“-Teil wird bedingt angezeigt.
- **Geändert in `192302d` (3.2.14):** Die Kamera-Karten haben keinen permanenten
  Wi-Fi-Eingabeblock mehr. Der Button „Change Wi-Fi“ öffnet einen nativen
  `<dialog>` (`#wifi-modal`) mit SSID/Passwort; `POST /api/devices/<uid>/wifi`
  bleibt unverändert. Closing per Cancel, ×, Backdrop oder Escape.
- **Geändert in `feb1898` (3.2.15):** Kamera-Karten nutzen jetzt dieselben
  Design-Tokens wie `setup.html` (Farben, Radien, Schatten, Typografie).
  Struktur: `.preview` mit Bild + Live-Indikator, `.card-head` (UID + Status),
  Section „Streams“ (RTSP/MJPEG/Snapshot als Code) und Section „Controls“
  (Live/Snapshot/Befehle/Wi-Fi-Button). JS-Funktionen unverändert; `live`
  toggelt zusätzlich `.active` und `.preview.live`.
- **Geändert in `d8ecdd4` (3.2.16):** Die Zeile „N camera(s) connected“
  (`#status`) wurde entfernt, weil sie „Cameras: N“ doppelt. Kamerazahl gibt es
  nur noch unter dem Titel; Leerzustand bleibt in `#empty`.
- **Geändert in `f5062be` (3.2.17):** Live-Button durch Play/Stop-Overlay
  (`.play`) im Bild ersetzt; `.preview.live` zeigt weiter den LIVE-Badge, der
  Punkt vor der IP ist weg. IR/status LED und Flip sind jetzt Switches
  (`.toggle`), deren Zustand aus `cam.info` (`IrLed`, `instLed`, `mirrorFlip`)
  kommt; 5 s `data-pending` verhindert Zurückspringen nach dem Klick. Die
  Firmware steht als kompakter Badge `.fw` ohne „fw“-Prefix.
- **Geändert in `ab027ea` (3.2.18):** Firmware jetzt als „Firmware <version>“
  unter dem Gerätenamen (`.identity`), IP allein rechts in `.meta`.
- **Geändert in `2d6bfb4` (3.2.19):** Play-Dreieck ohne Kreis, nur bei gestopptem
  Stream sichtbar (`.preview.live .play { display:none }`); Snapshot jetzt als
  `.snap-btn`-Kamera-Icon links unten im Bild statt als Button in „Controls“.
- **Geändert in `216df84` (3.2.20):** `<h1>` jetzt nur noch „Cameras" unter dem
  Eyebrow „A9 Camera · Overview" (`<title>` bleibt „A9 Cameras").
- **Neu in `550e325` / `c9d5413` (3.2.20):** Pro-Kamera-Zugriffsschalter
  `network_access` und `password_required` (beide Default `true`), gespeichert in
  `/data/access.json` (`server/access.go`). `authMiddleware` prüft nur die Pfade
  `/dev/<uid>/live` und `/dev/<uid>/snapshot`: bei `network_access == false`
  403, bei `password_required == false` ohne Login durchgelassen. Alle anderen
  Pfade (UI, API) bleiben geschützt; Ingress/Loopback/session immer erlaubt.
  Neuer Endpunkt `POST /api/devices/{uid}/access`, Felder auch in `GET /dev/list`.
  Die Karten zeigen die Schalter unter „Streams" und blenden die HTTP-Links aus,
  wenn Network Access aus ist.
- **Geändert in `59c9f97` (3.2.21):** Klick auf das laufende Bild stoppt den
  Stream (`img.onclick`), `.preview.live img` zeigt Pointer-Cursor.
- **Geändert in 3.2.21:** Snapshot-Icon ist jetzt ein `<button>`, der das Bild
  per `fetch` holt und in `#snap-modal` zeigt (Download-Button). Grund: eine
  neue Tab mit Ingress-URL hat keine `ingress_session` → 401 von HA.
- **Fix in 3.2.21:** `authMiddleware` prüft `network_access` jetzt **vor** der
  Session (nur für nicht-trusted Sources). Vorher konnte ein alter
  `a9cam_session`-Cookie auf `:8557` die Links trotz ausgeschaltetem
  Network Access öffnen. Ingress/Loopback bleiben erlaubt, damit die UI ihre
  Vorschau laden kann.
- **Neu in `a709332` (3.2.22):** `.flip-btn` rechts oben im Bild schaltet
  `mirrorFlip` (gleiche Commands wie der Switch in „Controls"); beide bleiben
  über `flipSwitch.dataset.pending` und die 2-s-Aktualisierung synchron.
- **Geändert in `b118bc0` (3.2.23):** Nicht mehr nur `img.onclick`, sondern
  `preview.onclick` mit `toggleLive()` – Klick auf die ganze Bildfläche
  startet/stoppt; `.play/.snap-btn/.flip-btn` werden per `closest()` ausgenommen.
  `.preview img` hat jetzt immer `cursor:pointer`.
- **Geändert in `b2ebed1` (3.2.24):** Die LIVE-Anzeige kommt jetzt aus
  `cam.streaming` (`/dev/list`, `server/main.go`) statt nur aus dem lokalen
  Klick. Pro Karte gibt es ein `live`-Objekt mit `server`, `stop`, `sync()`,
  `ended()`, `toggle()` an `el._live`; `update()` setzt `server` alle 2 s.
  So leuchtet LIVE auch bei Streams von Direktlinks/anderen Clients.
- **Geändert in `0fa6348` (3.2.25):** Der „Flip image"-Switch unter „Controls"
  ist entfernt. `.flip-btn` oben rechts ist jetzt alleinige Steuerung; sein
  Zustand kommt aus `cam.info.mirrorFlip`, mit 5-s-`data-pending` gegen
  Zurückspringen. Commands unverändert (`code=216&mirrorFlip=4/0`).
- **Geändert in `0cd9ac7` (3.2.26):** Die „Password required"-Zeile wird
  ausgeblendet (`pwdRow.hidden`), solange „Network access" aus ist. `syncAccess()`
  hängt an `el._syncAccess` und läuft beim Umschalten und in `update()`. Der
  gespeicherte Wert bleibt erhalten; Backend prüft Network Access ohnehin zuerst.
- **Neu in `ef5aa2b`/`bfb85c5` (3.2.27):** `server/viewers.go` zählt aktive
  `/live`-Viewer getrennt nach UI (`?raw`) und Direktlinks. `/dev/list` liefert
  `external_streaming`. Die Karte spiegelt externe Streams automatisch
  (`live.refresh()`, `auto`/`suppress`) und stoppt sie, wenn der andere Viewer
  geht. Beim Reaktivieren von Network Access wird `password_required=true`
  erzwungen.
- **Neu in `da62abe` (3.2.28):** `a9.Settings` hat pointer-Felder `ir_led`,
  `inst_led`, `mirror_flip` (omitempty, alte Dateien bleiben gültig).
  `Device.SendCommand` speichert diese Werte aus Commands,
  `Device.ApplySettings()` sendet sie beim Verbinden erneut (`server.go`).
  So überleben LED/Flip einen Reboot der Kamera.
- **Geändert in `81bc09b`/`668be5d` (3.2.32/33):** Auch `server/web/helper/a9-wifi-setup.py` sendet
  nach der geprüften 204-Antwort `code 299` (Reboot) und meldet nur dann Erfolg. **Falle:** beim
  ersten Edit war das `sendall` des Set-Wifi-Kommandos weggefallen; lokaler End-to-End-Test gegen
  eine Fake-Kamera (204+299 im Protokoll) und der `.command`-Launcher fingen das. `.bat` nur statisch
  prüfbar (kein Windows). Helper-Test temporär in `/private/var/folders/.../opencode/a9_helper_test.py`,
  nicht im Repo (HOST/PORT sind im Helper hart codiert).
- **Geändert in `2c62dbb` (3.2.31):** `apSetWiFi` sendet nach der bestätigten `code 204` jetzt auch
  `code 299` (Reboot) über AP-TCP `6123`. Die Kamera speicherte die WLAN-Daten nur und blieb im
  AP-Mode, daher kam trotz Erfolgsmeldung kein Wechsel ins Heim-WLAN. Die 502/204-Antwort wird jetzt
  validiert (code + status 0/200), bevor Erfolg gemeldet wird. `apAddress` ist testbar var;
  Regressionstest `TestAPSetWiFiSendsReboot` mit Fake-AP. Referenz: `a9-v702/fakecloud/ap_client.py`
  ruft nach `set_wifi()` explizit `reboot()`.
- **Geändert in `4d2f8bb` (3.2.30):** `supervisorConnectWiFi` sendet jetzt `enabled: true` und
  `ipv4: {method: auto}`. Vorher blieb ein via `disconnectWiFi` deaktiviertes WLAN-Interface aus
  bzw. bekam per Supervisor-Default `static` keine Adresse vom Kamera-AP → „camera AP did not assign
  an address". `waitForCameraAP` prüft zusätzlich `interfaceIPv4(iface)`, wartet 30 s und meldet
  klarer. Regressionstests prüfen beide Felder im Join-Request.
- **Sicherheitshärtung in `970749b`, `6ce0a8b`, `d875cd5` (3.2.30):** Supervisor-Pfade auf
  `/network/interface/<validierte iface>/{info,accesspoints,update}` begrenzt, Redirects blockiert,
  AP-Flow fail-closed; `GET /dev/{uid}/cmd` → POST mit Code-Allowlist (204 nur über dedizierte APIs),
  Origin/Sec-Fetch-Site-Prüfung, Session 24 h + `SameSite=Strict` + `Secure` bei HTTPS + `/logout`,
  Query-Parameter maskiert; atomare `access.json`/`settings.json` mit 0600, UID-Validierung,
  Body-Limits, Server-Timeouts, SSE-Client-Limit, Security-Header/CSP, Revocation laufender
  Direktstreams. Go-Tests weiterhin nur über HA-Build/CI möglich (lokal kein Go).
- **Geändert in `40ea7da` (3.2.29):** Reboot-Button wird bei Klick deaktiviert,
  zeigt `.spinner` + „Rebooting…". `el._rebootSince()`/`_finishReboot()`
  setzen ihn zurück, sobald `cam.connected_at` wechselt (Reconnect); 3-min-
  Fallback und 2-s-Retry bei fehlgeschlagenem Request.
- Weiterhin bewusst: `8556/tcp` ist jetzt standardmäßig leer (RTSP aus),
  damit auch RTSP nicht ungeschützt im Netzwerk liegt. Wer go2rtc nutzt,
  muss den Port in den Add-on-Netzwerkoptionen aktivieren.

## Wi-Fi-Setup – drei Wege (Stand 21.11.2026)

1. **Kamera bereits verbunden:** Formular **direkt auf der Kamera-Karte** in
   `index.html` → `POST /api/devices/<uid>/wifi` → `code 204` über die
   bestehende TCP-Session. Kein generelles Setup-Formular mehr.
2. **Neues Gerät im AP-Mode, HA per LAN + freies WLAN (Normalfall bei HA OS auf
   dem Raspberry Pi):** nur die Seite `setup.html` („Add new camera (AP
   mode)“); `GET /api/wifi/accesspoints` nutzt Supervisor
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
