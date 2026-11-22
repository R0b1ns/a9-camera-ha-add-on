package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"image/jpeg"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/mjpeg"
	"github.com/AlexxIT/go2rtc/pkg/rtsp"
	"github.com/dbuezas/a9-camera-ha-add-on/server/internal/fakecam"
	"github.com/dbuezas/a9-camera-ha-add-on/server/internal/hub"
	"github.com/dbuezas/a9-camera-ha-add-on/server/pkg/a9"
	"github.com/pion/rtp"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

var (
	once    sync.Once
	camAddr string
	tsURL   string
	nextUID atomic.Int32
)

// setup starts the servers once for all tests, and a new fake camera for each test
func setup(t *testing.T) (*fakecam.Cam, string, string) {
	t.Helper()
	once.Do(func() {
		log = zerolog.Nop()
		server = a9.NewServer()
		server.OnUpdate = bus.publish
		streams = hub.New(func(name string) (core.Producer, error) { return server.Dial("a9:" + name) })

		var tcp net.Listener
		var udp *net.UDPConn
		for {
			var err error
			tcp, err = net.Listen("tcp", "127.0.0.1:0")
			require.NoError(t, err)
			if udp, err = net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: tcp.Addr().(*net.TCPAddr).Port}); err == nil {
				break
			}
			_ = tcp.Close()
		}
		go server.ServeTCP(tcp)
		go server.ServeUDP(udp)
		camAddr = tcp.Addr().String()
		tsURL = httptest.NewServer(routes()).URL
	})

	uid := fmt.Sprintf("0800c00%05d", nextUID.Add(1))
	cam, err := fakecam.Dial(camAddr, uid)
	require.NoError(t, err)
	t.Cleanup(cam.Close)
	require.Eventually(t, func() bool { return server.Get(uid) != nil }, 2*time.Second, 10*time.Millisecond)
	require.Greater(t, len(cam.Frames[0]), 3000, "frames must be split in fragments")
	return cam, uid, tsURL
}

func TestHTTPList(t *testing.T) {
	_, uid, url := setup(t)
	res, err := http.Get(url + "/dev/list")
	require.NoError(t, err)
	defer res.Body.Close()
	var list []map[string]any
	require.NoError(t, json.NewDecoder(res.Body).Decode(&list))
	var found map[string]any
	for _, item := range list {
		if item["uid"] == uid {
			found = item
		}
	}
	require.NotNil(t, found)
	require.Equal(t, "127.0.0.1", found["host"])
}

func TestHTTPHomepage(t *testing.T) {
	_, _, url := setup(t)
	res, err := http.Get(url + "/")
	require.NoError(t, err)
	b, _ := io.ReadAll(res.Body)
	res.Body.Close()
	require.Equal(t, 200, res.StatusCode)
	require.Contains(t, string(b), "<title>A9 Cameras</title>")
	require.Contains(t, string(b), "<h1>Cameras</h1>")
}

func TestHTTPSnapshot(t *testing.T) {
	cam, uid, url := setup(t)
	for range 3 { // each snapshot opens and closes the stream
		res, err := http.Get(url + "/dev/" + uid + "/snapshot")
		require.NoError(t, err)
		b, _ := io.ReadAll(res.Body)
		res.Body.Close()
		require.Equal(t, 200, res.StatusCode)
		require.Equal(t, "image/jpeg", res.Header.Get("Content-Type"))
		_, err = jpeg.Decode(bytes.NewReader(b))
		require.NoError(t, err)
	}
	require.Eventually(t, func() bool { return cam.Closes.Load() == 3 }, 2*time.Second, 10*time.Millisecond)
	require.False(t, server.Get(uid).Streaming())
}

func TestHTTPSnapshotUnknown(t *testing.T) {
	_, _, url := setup(t)
	res, err := http.Get(url + "/dev/nope/snapshot")
	require.NoError(t, err)
	res.Body.Close()
	require.Equal(t, http.StatusServiceUnavailable, res.StatusCode)
}

func TestHTTPMJPEG(t *testing.T) {
	cam, uid, url := setup(t)

	// two viewers share one camera stream
	read := func() (*http.Response, *multipart.Reader) {
		res, err := http.Get(url + "/dev/" + uid + "/live")
		require.NoError(t, err)
		require.True(t, strings.HasPrefix(res.Header.Get("Content-Type"), "multipart/x-mixed-replace"))
		return res, multipart.NewReader(res.Body, "frame")
	}
	res1, mr1 := read()
	res2, mr2 := read()

	for _, mr := range []*multipart.Reader{mr1, mr2, mr1, mr2} {
		part, err := mr.NextPart()
		require.NoError(t, err)
		b, _ := io.ReadAll(part)
		require.True(t, bytes.Equal(b, cam.Frames[0]) || bytes.Equal(b, cam.Frames[1]))
	}
	require.EqualValues(t, 1, cam.Opens.Load(), "one camera stream for two viewers")

	res1.Body.Close()
	time.Sleep(300 * time.Millisecond)
	require.True(t, server.Get(uid).Streaming(), "one viewer left")
	res2.Body.Close()
	require.Eventually(t, func() bool { return !server.Get(uid).Streaming() }, 3*time.Second, 20*time.Millisecond)
	require.Eventually(t, func() bool { return cam.Closes.Load() == 1 }, time.Second, 10*time.Millisecond)
}

// ?raw is for the web UI behind HA ingress: same parts, a content type HA does not touch
func TestHTTPMJPEGRaw(t *testing.T) {
	cam, uid, url := setup(t)

	res, err := http.Get(url + "/dev/" + uid + "/live?raw")
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, "application/octet-stream", res.Header.Get("Content-Type"))
	part, err := multipart.NewReader(res.Body, "frame").NextPart()
	require.NoError(t, err)
	b, _ := io.ReadAll(part)
	require.True(t, bytes.Equal(b, cam.Frames[0]) || bytes.Equal(b, cam.Frames[1]))
}

// The camera disconnects while a viewer watches: the viewer must be closed, not hang
func TestHTTPMJPEGCameraGone(t *testing.T) {
	cam, uid, url := setup(t)
	res, err := http.Get(url + "/dev/" + uid + "/live")
	require.NoError(t, err)
	defer res.Body.Close()
	br := bufio.NewReader(res.Body)
	_, err = br.ReadString('\n')
	require.NoError(t, err)

	cam.Close()
	done := make(chan struct{})
	go func() { _, _ = io.Copy(io.Discard, br); close(done) }()
	select {
	case <-done:
	case <-time.After(35 * time.Second): // TCP read timeout of the camera connection is 30 s
		t.Fatal("viewer still open")
	}
}

func TestHTTPCommand(t *testing.T) {
	cam, uid, url := setup(t)
	res, err := http.Post(url+"/dev/"+uid+"/cmd?code=202&IrLed=1", "text/plain", nil)
	require.NoError(t, err)
	b, _ := io.ReadAll(res.Body)
	res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode)
	require.JSONEq(t, `{"code":202,"IrLed":1}`, string(b))

	select {
	case c := <-cam.Commands:
		require.EqualValues(t, 202, c["code"])
		require.EqualValues(t, 1, c["IrLed"])
	case <-time.After(2 * time.Second):
		t.Fatal("no command")
	}

	// A GET cannot change camera state: SameSite cookies may accompany it.
	getRes, err := http.Get(url + "/dev/" + uid + "/cmd?code=299&reboot=1")
	require.NoError(t, err)
	_ = getRes.Body.Close()
	require.Equal(t, http.StatusMethodNotAllowed, getRes.StatusCode)

	// Wi-Fi configuration (204) only belongs to the dedicated APIs.
	wifiRes, err := http.Post(url+"/dev/"+uid+"/cmd?code=204&s=Evil&p=password123", "text/plain", nil)
	require.NoError(t, err)
	_ = wifiRes.Body.Close()
	require.Equal(t, http.StatusBadRequest, wifiRes.StatusCode)
}

func TestCommandJSONRejectsWiFiCode(t *testing.T) {
	_, uid, url := setup(t)
	res, err := http.Post(url+"/api/devices/"+uid+"/command", "application/json",
		strings.NewReader(`{"code":204,"s":"Evil","p":"password123"}`))
	require.NoError(t, err)
	_ = res.Body.Close()
	require.Equal(t, http.StatusBadRequest, res.StatusCode)
}

func TestRTSP(t *testing.T) {
	cam, uid, _ := setup(t)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()
	go serveRTSP(ln)

	client := rtsp.NewClient("rtsp://" + ln.Addr().String() + "/" + uid)
	require.NoError(t, client.Dial())
	require.NoError(t, client.Describe())

	var kinds []string
	frames := make(chan []byte, 10)
	audio := make(chan int, 10)
	for _, media := range client.Medias {
		kinds = append(kinds, media.Kind+":"+media.Codecs[0].Name)
		track, err := client.GetTrack(media, media.Codecs[0])
		require.NoError(t, err)
		sender := core.NewSender(media, track.Codec)
		if media.Kind == core.KindVideo {
			sender.Handler = mjpeg.RTPDepay(func(p *rtp.Packet) { frames <- p.Payload })
		} else {
			sender.Handler = func(p *rtp.Packet) { audio <- len(p.Payload) }
		}
		sender.HandleRTP(track)
	}
	require.ElementsMatch(t, []string{"video:JPEG", "audio:PCMA"}, kinds)

	require.NoError(t, client.Play())
	go func() { _ = client.Start() }()

	for range 3 {
		select {
		case b := <-frames:
			// RTP JPEG rebuilds the headers, so compare the picture
			img, err := jpeg.Decode(bytes.NewReader(b))
			require.NoError(t, err)
			require.Equal(t, 320, img.Bounds().Dx())
		case <-time.After(5 * time.Second):
			t.Fatal("no rtsp video")
		}
	}
	select {
	case n := <-audio:
		require.Equal(t, 800, n)
	case <-time.After(2 * time.Second):
		t.Fatal("no rtsp audio")
	}

	_ = client.Stop()
	require.Eventually(t, func() bool { return cam.Closes.Load() == 1 }, 3*time.Second, 20*time.Millisecond)
}

func TestRTSPUnknown(t *testing.T) {
	setup(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()
	go serveRTSP(ln)

	client := rtsp.NewClient("rtsp://" + ln.Addr().String() + "/nope")
	require.NoError(t, client.Dial())
	require.Error(t, client.Describe())
	_ = client.Close()
}

func TestAPIEventsAndCommand(t *testing.T) {
	_, uid, url := setup(t)

	res, err := http.Get(url + "/api/events")
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, "text/event-stream", res.Header.Get("Content-Type"))
	events := make(chan map[string]any, 100)
	go func() {
		sc := bufio.NewScanner(res.Body)
		sc.Buffer(make([]byte, 64*1024), 1<<20)
		for sc.Scan() {
			if data, ok := strings.CutPrefix(sc.Text(), "data: "); ok {
				var d map[string]any
				_ = json.Unmarshal([]byte(data), &d)
				events <- d
			}
		}
	}()

	// waits for an event of this camera that matches
	wait := func(match func(d map[string]any) bool) map[string]any {
		t.Helper()
		timeout := time.After(3 * time.Second)
		for {
			select {
			case d := <-events:
				if d["uid"] == uid && match(d) {
					return d
				}
			case <-timeout:
				t.Fatal("no matching event")
			}
		}
	}

	// the base info asked on connect arrives as an event
	d := wait(func(d map[string]any) bool { return d["info"].(map[string]any)["version"] != nil })
	require.Equal(t, true, d["online"])
	require.EqualValues(t, 0, d["info"].(map[string]any)["IrLed"])

	res2, err := http.Post(url+"/api/devices/"+uid+"/command", "application/json", strings.NewReader(`{"code":202,"IrLed":1}`))
	require.NoError(t, err)
	res2.Body.Close()
	require.Equal(t, http.StatusNoContent, res2.StatusCode)

	// the camera echo updates the state right away
	wait(func(d map[string]any) bool { return d["info"].(map[string]any)["IrLed"] == float64(1) })

	res3, err := http.Get(url + "/api/devices")
	require.NoError(t, err)
	var list []map[string]any
	require.NoError(t, json.NewDecoder(res3.Body).Decode(&list))
	res3.Body.Close()
	for _, d := range list {
		if d["uid"] == uid {
			require.EqualValues(t, 1, d["info"].(map[string]any)["IrLed"])
		}
	}

	res4, err := http.Post(url+"/api/devices/"+uid+"/command", "application/json", strings.NewReader(`[1]`))
	require.NoError(t, err)
	res4.Body.Close()
	require.Equal(t, http.StatusBadRequest, res4.StatusCode)
}

func TestAPIEventsOffline(t *testing.T) {
	cam, uid, url := setup(t)
	res, err := http.Get(url + "/api/events")
	require.NoError(t, err)
	defer res.Body.Close()

	cam.Close()
	sc := bufio.NewScanner(res.Body)
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	deadline := time.Now().Add(3 * time.Second)
	for sc.Scan() && time.Now().Before(deadline) {
		if data, ok := strings.CutPrefix(sc.Text(), "data: "); ok {
			var d map[string]any
			_ = json.Unmarshal([]byte(data), &d)
			if d["uid"] == uid && d["online"] == false {
				return
			}
		}
	}
	t.Fatal("no offline event")
}

func TestHTTPSnapshotMaxAge(t *testing.T) {
	cam, uid, url := setup(t)
	get := func(q string) []byte {
		res, err := http.Get(url + "/dev/" + uid + "/snapshot" + q)
		require.NoError(t, err)
		defer res.Body.Close()
		require.Equal(t, 200, res.StatusCode)
		b, _ := io.ReadAll(res.Body)
		return b
	}
	get("")
	require.EqualValues(t, 1, cam.Opens.Load())
	get("?max_age=60") // recent frame, the camera is not asked again
	require.EqualValues(t, 1, cam.Opens.Load())
	time.Sleep(1100 * time.Millisecond)
	get("?max_age=1") // too old
	require.EqualValues(t, 2, cam.Opens.Load())
}

func TestHTTPWiFiCommand(t *testing.T) {
	cam, uid, url := setup(t)
	res, err := http.Post(url+"/api/devices/"+uid+"/wifi", "application/json",
		strings.NewReader(`{"ssid":"WLAN-12358","password":"secret123"}`))
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusNoContent, res.StatusCode)

	select {
	case cmd := <-cam.Commands:
		require.EqualValues(t, a9.FwdSetWiFi, cmd["code"])
		require.Equal(t, a9.CLITarget, cmd["devTarget"])
		require.Equal(t, "WLAN-12358", cmd["s"])
		require.Equal(t, "secret123", cmd["p"])
	case <-time.After(2 * time.Second):
		t.Fatal("no wifi command")
	}
}

func TestHTTPWiFiValidation(t *testing.T) {
	cam, uid, url := setup(t)
	for _, body := range []string{
		`{"ssid":"","password":"secret123"}`,
		`{"ssid":"WLAN-12358","password":"short"}`,
		`{"ssid":"WLAN-12358"}`,
	} {
		res, err := http.Post(url+"/api/devices/"+uid+"/wifi", "application/json", strings.NewReader(body))
		require.NoError(t, err)
		_ = res.Body.Close()
		require.Equal(t, http.StatusBadRequest, res.StatusCode, body)
	}
	select {
	case cmd := <-cam.Commands:
		t.Fatalf("unexpected command: %v", cmd)
	case <-time.After(300 * time.Millisecond):
	}
}

func TestAuthMiddleware(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	secret := []byte("0123456789abcdef0123456789abcdef")
	guard := authMiddleware(next, secret, true)
	call := func(remote string, cookie *http.Cookie) *httptest.ResponseRecorder {
		req := httptest.NewRequest("GET", "/dev/list", nil)
		req.RemoteAddr = remote
		if cookie != nil {
			req.AddCookie(cookie)
		}
		rec := httptest.NewRecorder()
		guard.ServeHTTP(rec, req)
		return rec
	}
	for _, remote := range []string{"127.0.0.1:1234", "[::1]:1234", "172.30.32.2:1234"} {
		require.Equal(t, http.StatusNoContent, call(remote, nil).Code, remote)
	}
	rec := call("192.168.1.50:1234", nil)
	require.Equal(t, http.StatusFound, rec.Code)
	require.Contains(t, rec.Header().Get("Location"), "/login?next=")

	cookie := &http.Cookie{Name: sessionCookie, Value: sessionValue(secret, "jane", time.Now().Add(time.Hour).Unix())}
	require.Equal(t, http.StatusNoContent, call("192.168.1.50:1234", cookie).Code)
	cookie.Value = sessionValue(secret, "jane", time.Now().Add(-time.Hour).Unix())
	require.Equal(t, http.StatusFound, call("192.168.1.50:1234", cookie).Code)

	// Without a Supervisor token (standalone) direct access stays open.
	openRec := httptest.NewRecorder()
	authMiddleware(next, secret, false).ServeHTTP(openRec, httptest.NewRequest("GET", "/", nil))
	require.Equal(t, http.StatusNoContent, openRec.Code)
}

func TestCrossOriginWritesRejected(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	guard := authMiddleware(next, []byte("0123456789abcdef0123456789abcdef"), false)
	call := func(origin, fetchSite string) int {
		req := httptest.NewRequest("POST", "http://ha.example:8123/api/devices/cam1/access", nil)
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		if fetchSite != "" {
			req.Header.Set("Sec-Fetch-Site", fetchSite)
		}
		rec := httptest.NewRecorder()
		guard.ServeHTTP(rec, req)
		return rec.Code
	}
	require.Equal(t, http.StatusForbidden, call("https://evil.example", ""))
	require.Equal(t, http.StatusForbidden, call("", "cross-site"))
	require.Equal(t, http.StatusNoContent, call("https://ha.example:8123", "same-origin"))
	require.Equal(t, http.StatusNoContent, call("", ""))
}

func TestSafeNextAndSessionCookie(t *testing.T) {
	t.Setenv("SUPERVISOR_TOKEN", "test-token")
	for _, good := range []string{"/", "/index.html", "/dev/x/live?raw"} {
		require.True(t, safeNext(good), good)
	}
	for _, bad := range []string{"", "https://evil.example", "//evil.example",
		"/\\evil.example", "/a\r\nX: 1", "/a\tb", "/a\x7f"} {
		require.False(t, safeNext(bad), bad)
	}

	old := supervisorAuthURL
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	supervisorAuthURL = srv.URL
	defer func() { supervisorAuthURL = old; srv.Close() }()

	req := httptest.NewRequest("POST", "/login", strings.NewReader("username=jane&password=pw&next=%2F"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	loginPOST(rec, req)
	require.Equal(t, http.StatusSeeOther, rec.Code)
	cookies := rec.Result().Cookies()
	require.Len(t, cookies, 1)
	require.Equal(t, sessionCookie, cookies[0].Name)
	require.True(t, cookies[0].HttpOnly)
	require.Equal(t, http.SameSiteStrictMode, cookies[0].SameSite)
	require.Equal(t, "/", cookies[0].Path)
	require.False(t, cookies[0].Secure, "plain HTTP must not get a Secure cookie")
	require.LessOrEqual(t, time.Until(cookies[0].Expires), 24*time.Hour)
}

func TestCameraAccess(t *testing.T) {
	oldPath := access.path
	access.path = t.TempDir() + "/access.json"
	access.load()
	defer func() { access.path = oldPath; access.load() }()

	secret := []byte("0123456789abcdef0123456789abcdef")
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	guard := authMiddleware(next, secret, true)
	call := func(path, remote string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
		req := httptest.NewRequest("GET", path, nil)
		req.RemoteAddr = remote
		for _, c := range cookies {
			req.AddCookie(c)
		}
		rec := httptest.NewRecorder()
		guard.ServeHTTP(rec, req)
		return rec
	}
	const external = "192.168.1.50:1234"
	const link = "/dev/cam1/snapshot"

	// Both switches default to on: the direct link asks for the HA login.
	require.Equal(t, http.StatusFound, call(link, external).Code)
	require.NoError(t, access.set("cam1", cameraAccess{NetworkAccess: true, PasswordRequired: false}))
	require.Equal(t, http.StatusNoContent, call(link, external).Code)
	require.Equal(t, http.StatusNoContent, call("/dev/cam1/live?raw", external).Code)
	// Only the two camera links become public, never the UI or the API.
	require.Equal(t, http.StatusFound, call("/api/status", external).Code)
	require.Equal(t, http.StatusFound, call("/dev/other/snapshot", external).Code)

	// Network access off blocks the links from outside, even for a logged-in
	// browser (the session is checked afterwards).
	require.NoError(t, access.set("cam1", cameraAccess{NetworkAccess: false, PasswordRequired: false}))
	cookie := &http.Cookie{Name: sessionCookie, Value: sessionValue(secret, "jane", time.Now().Add(time.Hour).Unix())}
	require.Equal(t, http.StatusForbidden, call(link, external).Code)
	require.Equal(t, http.StatusForbidden, call(link, external, cookie).Code)

	// Ingress and loopback still work, so the UI can show its own preview.
	require.Equal(t, http.StatusNoContent, call(link, "172.30.32.2:1234").Code)
	require.Equal(t, http.StatusNoContent, call(link, "127.0.0.1:1234").Code)
}

func TestAccessEndpoint(t *testing.T) {
	_, uid, url := setup(t)
	t.Cleanup(func() { _ = access.set(uid, defaultCameraAccess) })

	body := `{"network_access":false,"password_required":false}`
	res, err := http.Post(url+"/api/devices/"+uid+"/access", "application/json", strings.NewReader(body))
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusNoContent, res.StatusCode)

	res2, err := http.Get(url + "/dev/list")
	require.NoError(t, err)
	defer res2.Body.Close()
	var list []map[string]any
	require.NoError(t, json.NewDecoder(res2.Body).Decode(&list))
	var found map[string]any
	for _, item := range list {
		if item["uid"] == uid {
			found = item
		}
	}
	require.NotNil(t, found)
	require.Equal(t, false, found["network_access"])
	require.Equal(t, false, found["password_required"])

	// Re-enabling the links restores the safe password default.
	body = `{"network_access":true,"password_required":false}`
	res3, err := http.Post(url+"/api/devices/"+uid+"/access", "application/json", strings.NewReader(body))
	require.NoError(t, err)
	defer res3.Body.Close()
	require.Equal(t, http.StatusNoContent, res3.StatusCode)

	res4, err := http.Get(url + "/dev/list")
	require.NoError(t, err)
	defer res4.Body.Close()
	var list2 []map[string]any
	require.NoError(t, json.NewDecoder(res4.Body).Decode(&list2))
	var found2 map[string]any
	for _, item := range list2 {
		if item["uid"] == uid {
			found2 = item
		}
	}
	require.NotNil(t, found2)
	require.Equal(t, true, found2["network_access"])
	require.Equal(t, true, found2["password_required"])
}

func TestAccessRejectsUnknownUID(t *testing.T) {
	_, _, url := setup(t)
	for _, uid := range []string{"bad.uid", "bad%2Fuid"} {
		res, err := http.Post(url+"/api/devices/"+uid+"/access", "application/json",
			strings.NewReader(`{"network_access":false}`))
		require.NoError(t, err)
		_ = res.Body.Close()
		require.Equal(t, http.StatusBadRequest, res.StatusCode, uid)
	}
	res, err := http.Post(url+"/api/devices/does-not-exist/access", "application/json",
		strings.NewReader(`{"network_access":false}`))
	require.NoError(t, err)
	_ = res.Body.Close()
	require.Equal(t, http.StatusNotFound, res.StatusCode)
}

func TestStopDirectViewers(t *testing.T) {
	viewers = liveViewers{raw: map[string]int{}, link: map[string]int{},
		stops: map[string]map[*viewerStop]struct{}{}}
	var stopped []bool
	u1 := viewers.register("cam1", false, func() { stopped = append(stopped, false) })
	u2 := viewers.register("cam1", true, func() { stopped = append(stopped, true) })
	viewers.stopDirect("cam1")
	require.Equal(t, []bool{false}, stopped, "only direct links must be stopped")
	u1()
	u2()
}

func TestLiveViewers(t *testing.T) {
	viewers = liveViewers{raw: map[string]int{}, link: map[string]int{}}
	viewers.add("cam1", false, 1)
	require.Equal(t, 1, viewers.count("cam1", false))
	require.Equal(t, 0, viewers.count("cam1", true))
	viewers.add("cam1", true, 1)
	require.Equal(t, 1, viewers.count("cam1", true))
	viewers.add("cam1", false, -1)
	require.Equal(t, 0, viewers.count("cam1", false))
	viewers.add("cam1", true, -1)
	require.Equal(t, 0, viewers.count("cam1", true))
}

func TestLoginPage(t *testing.T) {
	req := httptest.NewRequest("GET", "/login?next=%2Fdev%2Fuid%2Fsnapshot", nil)
	rec := httptest.NewRecorder()
	loginGET(rec, req)
	require.Equal(t, 200, rec.Code)
	require.Contains(t, rec.Body.String(), "Home Assistant user")
	require.Contains(t, rec.Body.String(), `value="/dev/uid/snapshot"`)
}

func TestVerifyHACredentials(t *testing.T) {
	auth := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "test-token", r.Header.Get("X-Supervisor-Token"))
		var body map[string]string
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		if body["username"] == "jane" && body["password"] == "secret" {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusUnauthorized)
		}
	}))
	defer auth.Close()
	oldURL, oldToken := supervisorAuthURL, os.Getenv("SUPERVISOR_TOKEN")
	supervisorAuthURL = auth.URL
	require.NoError(t, os.Setenv("SUPERVISOR_TOKEN", "test-token"))
	defer func() {
		supervisorAuthURL = oldURL
		_ = os.Setenv("SUPERVISOR_TOKEN", oldToken)
	}()

	require.True(t, verifyHACredentials("jane", "secret"))
	require.False(t, verifyHACredentials("jane", "wrong"))
}

func TestStatusEndpoint(t *testing.T) {
	_, _, url := setup(t)
	res, err := http.Get(url + "/api/status")
	require.NoError(t, err)
	defer res.Body.Close()
	b, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	var st statusJSON
	require.NoError(t, json.Unmarshal(b, &st))
	require.Equal(t, "v720.naxclow.com", st.DNSName)
	require.NotEmpty(t, st.LocalIPs)
	var raw map[string]any
	require.NoError(t, json.Unmarshal(b, &raw))
	for _, key := range []string{"lan_connected", "wifi_connected", "ap_ready"} {
		require.Contains(t, raw, key)
	}
}

func TestAccessPointsEndpoint(t *testing.T) {
	t.Setenv("SUPERVISOR_TOKEN", "test-token")
	_, _, url := setup(t)
	old := supervisorURL
	apSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/network/interface/wlan0/accesspoints", r.URL.Path)
		_, _ = io.WriteString(w, `{"result":"ok","data":{"accesspoints":[{"ssid":"NAXCLOW-123","signal":-40}]}}`)
	}))
	supervisorURL = apSrv.URL
	defer func() { supervisorURL = old; apSrv.Close() }()

	res, err := http.Get(url + "/api/wifi/accesspoints?iface=wlan0")
	require.NoError(t, err)
	defer res.Body.Close()
	var aps []accessPoint
	require.NoError(t, json.NewDecoder(res.Body).Decode(&aps))
	require.Len(t, aps, 1)
	require.Equal(t, "NAXCLOW-123", aps[0].SSID)
}

func TestWiFiAPIRejectsInvalidInterface(t *testing.T) {
	t.Setenv("SUPERVISOR_TOKEN", "test-token")
	_, _, url := setup(t)
	old := supervisorURL
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	supervisorURL = srv.URL
	defer func() { supervisorURL = old; srv.Close() }()

	res, err := http.Get(url + "/api/wifi/accesspoints?iface=../../core")
	require.NoError(t, err)
	_ = res.Body.Close()
	require.Equal(t, http.StatusBadRequest, res.StatusCode)

	body := `{"iface":"../../core","ap_ssid":"NAXCLOW-1","ssid":"CameraNet","password":"newpassword"}`
	res2, err := http.Post(url+"/api/wifi/configure-ap", "application/json", strings.NewReader(body))
	require.NoError(t, err)
	_ = res2.Body.Close()
	require.Equal(t, http.StatusBadRequest, res2.StatusCode)
	require.Zero(t, calls, "an invalid interface must never reach the Supervisor")
}

func TestWiFiAPFailsClosedWhenStateUnknown(t *testing.T) {
	t.Setenv("SUPERVISOR_TOKEN", "test-token")
	_, _, url := setup(t)
	old := supervisorURL
	var posts int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			posts++
		}
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, `{"result":"error","message":"psk=hunter2 secret-config"}`)
	}))
	supervisorURL = srv.URL
	defer func() { supervisorURL = old; srv.Close() }()

	body := `{"iface":"wlan0","ap_ssid":"NAXCLOW-1","ssid":"CameraNet","password":"newpassword"}`
	res, err := http.Post(url+"/api/wifi/configure-ap", "application/json", strings.NewReader(body))
	require.NoError(t, err)
	b, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	require.Equal(t, http.StatusBadGateway, res.StatusCode)
	require.Zero(t, posts, "an unknown Wi-Fi state must not change the interface")
	require.NotContains(t, string(b), "secret-config")
	require.NotContains(t, string(b), "hunter2")
}

func TestHelperDownloads(t *testing.T) {
	_, _, url := setup(t)
	for _, name := range []string{"a9-wifi-setup.py", "a9-wifi-setup.bat", "a9-wifi-setup.command"} {
		res, err := http.Get(url + "/helper/" + name)
		require.NoError(t, err)
		b, _ := io.ReadAll(res.Body)
		_ = res.Body.Close()
		require.Equal(t, 200, res.StatusCode, name)
		require.NotEmpty(t, b, name)
	}
}

func TestWiFiAPRejectsActiveWiFi(t *testing.T) {
	t.Setenv("SUPERVISOR_TOKEN", "test-token")
	_, _, url := setup(t)
	old := supervisorURL
	var posts int
	wifiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			posts++
		}
		_, _ = io.WriteString(w, `{"result":"ok","data":{"connected":true}}`)
	}))
	supervisorURL = wifiSrv.URL
	defer func() { supervisorURL = old; wifiSrv.Close() }()

	body := `{"iface":"wlan0","ap_ssid":"NAXCLOW-1","ap_password":"apsecret","ssid":"CameraNet","password":"newpassword"}`
	res, err := http.Post(url+"/api/wifi/configure-ap", "application/json", strings.NewReader(body))
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusConflict, res.StatusCode)
	require.Zero(t, posts, "an active home Wi-Fi must not be changed")
}

func TestWiFiAPFlowDisconnectsWiFi(t *testing.T) {
	t.Setenv("SUPERVISOR_TOKEN", "test-token")
	_, _, url := setup(t)
	old := supervisorURL
	var mu sync.Mutex
	var posts []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/network/interface/wlan0/info":
			_, _ = io.WriteString(w, `{"result":"ok","data":{"connected":false}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/network/interface/wlan0/update":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			mu.Lock()
			posts = append(posts, body)
			mu.Unlock()
			_, _ = io.WriteString(w, `{"result":"ok","data":{}}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	supervisorURL = srv.URL
	defer func() { supervisorURL = old; srv.Close() }()

	oldSet, oldWait := apSetWiFiFunc, waitForCameraAPFunc
	apSetWiFiFunc = func(ssid, password string) error {
		require.Equal(t, "CameraNet", ssid)
		require.Equal(t, "newpassword", password)
		return nil
	}
	waitForCameraAPFunc = func(time.Duration) bool { return true }
	defer func() { apSetWiFiFunc, waitForCameraAPFunc = oldSet, oldWait }()

	body := `{"iface":"wlan0","ap_ssid":"NAXCLOW-1","ap_password":"apsecret",` +
		`"ssid":"CameraNet","password":"newpassword"}`
	res, err := http.Post(url+"/api/wifi/configure-ap", "application/json", strings.NewReader(body))
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusNoContent, res.StatusCode)
	mu.Lock()
	defer mu.Unlock()
	require.Len(t, posts, 2)

	join, ok := posts[0]["wifi"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "NAXCLOW-1", join["ssid"])
	require.Equal(t, "apsecret", join["psk"])

	// Afterwards Wi-Fi is switched off again; LAN keeps HA reachable.
	require.Equal(t, false, posts[1]["enabled"])
}

func TestWiFiAPFlowJoinsOpenCameraAP(t *testing.T) {
	t.Setenv("SUPERVISOR_TOKEN", "test-token")
	_, _, url := setup(t)
	old := supervisorURL
	var mu sync.Mutex
	var posts []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/network/interface/wlan0/info":
			_, _ = io.WriteString(w, `{"result":"ok","data":{"connected":false}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/network/interface/wlan0/update":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			mu.Lock()
			posts = append(posts, body)
			mu.Unlock()
			_, _ = io.WriteString(w, `{"result":"ok","data":{}}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	supervisorURL = srv.URL
	defer func() { supervisorURL = old; srv.Close() }()

	oldSet, oldWait := apSetWiFiFunc, waitForCameraAPFunc
	apSetWiFiFunc = func(ssid, password string) error {
		require.Equal(t, "CameraNet", ssid)
		require.Equal(t, "newpassword", password)
		return nil
	}
	waitForCameraAPFunc = func(time.Duration) bool { return true }
	defer func() { apSetWiFiFunc, waitForCameraAPFunc = oldSet, oldWait }()

	// The camera AP is open: the request has no ap_password.
	body := `{"iface":"wlan0","ap_ssid":"NAXCLOW-1","ssid":"CameraNet","password":"newpassword"}`
	res, err := http.Post(url+"/api/wifi/configure-ap", "application/json", strings.NewReader(body))
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusNoContent, res.StatusCode)
	mu.Lock()
	defer mu.Unlock()
	require.Len(t, posts, 2)
	join, ok := posts[0]["wifi"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "NAXCLOW-1", join["ssid"])
	require.Equal(t, "open", join["auth"])
	require.NotContains(t, join, "psk")
	require.Equal(t, false, posts[1]["enabled"])
}

func TestCameraPortDoesNotServeUI(t *testing.T) {
	req := httptest.NewRequest("GET", "/dev/list", nil)
	rec := httptest.NewRecorder()
	cameraRoutes().ServeHTTP(rec, req)
	require.Equal(t, http.StatusNotFound, rec.Code)
}

func TestHTTPSetupPage(t *testing.T) {
	_, _, url := setup(t)
	res, err := http.Get(url + "/setup.html")
	require.NoError(t, err)
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	require.Equal(t, 200, res.StatusCode)
	require.Contains(t, string(b), "Add Camera")
	require.Contains(t, string(b), "Keep the DNS rewrites in place")
	require.Contains(t, string(b), "WPA2")
	require.Contains(t, string(b), `id="helpercard"`)
}

func TestHTTPIndexHTML(t *testing.T) {
	_, _, url := setup(t)
	res, err := http.Get(url + "/index.html")
	require.NoError(t, err)
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	require.Equal(t, 200, res.StatusCode)
	require.Contains(t, string(b), "Add Camera")
	require.Contains(t, string(b), `id="dns-badge"`)
	require.Contains(t, string(b), `id="wifi-modal"`)
	require.Contains(t, string(b), `class="preview"`)
	require.Contains(t, string(b), `class="play"`)
	require.Contains(t, string(b), `class="snap-btn"`)
	require.Contains(t, string(b), `id="snap-modal"`)
	require.Contains(t, string(b), `class="flip-btn"`)
	require.Contains(t, string(b), "cam.streaming")
	require.Contains(t, string(b), "cam.external_streaming")
	require.Contains(t, string(b), "Rebooting")
	require.NotContains(t, string(b), `data-key="mirrorFlip"`)
	require.Contains(t, string(b), "Streams")
	require.Contains(t, string(b), "Controls")
	require.Contains(t, string(b), "Flip image")
	require.Contains(t, string(b), "Network access")
	require.Contains(t, string(b), "Password required")
}
