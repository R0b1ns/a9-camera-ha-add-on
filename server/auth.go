package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const sessionCookie = "a9cam_session"

// supervisorAuthURL is a variable so tests can point it at a local server.
var supervisorAuthURL = "http://supervisor/auth"

// loadSessionSecret keeps sessions valid across add-on restarts when a data
// directory is available. Standalone runs get a fresh secret on every start.
func loadSessionSecret(path string) []byte {
	if path != "" {
		if b, err := os.ReadFile(path); err == nil && len(b) >= 32 {
			return b
		}
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		log.Fatal().Err(err).Msg("[auth] session secret")
	}
	if path != "" {
		if err := os.WriteFile(path, b, 0o600); err != nil {
			log.Warn().Err(err).Msg("[auth] cannot store session secret")
		}
	}
	return b
}

func sessionValue(secret []byte, user string, expires int64) string {
	mac := hmac.New(sha256.New, secret)
	_, _ = fmt.Fprintf(mac, "%s|%d", user, expires)
	return base64.RawURLEncoding.EncodeToString([]byte(user)) + "." +
		strconv.FormatInt(expires, 10) + "." + hex.EncodeToString(mac.Sum(nil))
}

func validSession(r *http.Request, secret []byte) bool {
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return false
	}
	parts := strings.Split(c.Value, ".")
	if len(parts) != 3 {
		return false
	}
	expires, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || expires <= time.Now().Unix() {
		return false
	}
	user, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil || len(user) == 0 {
		return false
	}
	mac := hmac.New(sha256.New, secret)
	_, _ = fmt.Fprintf(mac, "%s|%d", user, expires)
	want := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(parts[2]), []byte(want))
}

func trustedSource(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	ip, err := netip.ParseAddr(strings.Trim(host, "[]"))
	if err != nil {
		return false
	}
	for _, prefix := range defaultUIAllow {
		if prefix.Contains(ip) {
			return true
		}
	}
	return false
}

// authMiddleware protects direct links to port 8557 with a Home Assistant login.
// Ingress (Supervisor) and the local integration are already trusted. Per camera,
// the two direct links can be opened without a password or blocked for the
// network entirely; the rest of the UI always needs the login.
func authMiddleware(next http.Handler, secret []byte, enabled bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !enabled || r.URL.Path == "/login" || trustedSource(r.RemoteAddr) || validSession(r, secret) {
			next.ServeHTTP(w, r)
			return
		}
		if uid, ok := directLinkUID(r.URL.Path); ok {
			ac := access.get(uid)
			if !ac.NetworkAccess {
				http.Error(w, "network access is disabled for this camera", http.StatusForbidden)
				return
			}
			if !ac.PasswordRequired {
				next.ServeHTTP(w, r)
				return
			}
		}
		target := r.URL.RequestURI()
		http.Redirect(w, r, "/login?next="+url.QueryEscape(target), http.StatusFound)
	})
}

func loginGET(w http.ResponseWriter, r *http.Request) {
	next := r.URL.Query().Get("next")
	if !safeNext(next) {
		next = "/"
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = io.WriteString(w, renderLogin(next, ""))
}

func loginPOST(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	user := strings.TrimSpace(r.FormValue("username"))
	password := r.FormValue("password")
	next := r.FormValue("next")
	if !safeNext(next) {
		next = "/"
	}
	if !verifyHACredentials(user, password) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, renderLogin(next, "Wrong Home Assistant user or password."))
		return
	}
	expires := time.Now().Add(7 * 24 * time.Hour)
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    sessionValue(sessionSecret, user, expires.Unix()),
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Expires:  expires,
		MaxAge:   int(time.Until(expires).Seconds()),
	})
	http.Redirect(w, r, next, http.StatusSeeOther)
}

// verifyHACredentials asks the Supervisor to check the user against Home
// Assistant. X-Supervisor-Token is the app token, not the user's password.
func verifyHACredentials(user, password string) bool {
	token := os.Getenv("SUPERVISOR_TOKEN")
	if token == "" || user == "" || password == "" {
		return false
	}
	body := mustJSON(map[string]string{"username": user, "password": password})
	req, err := http.NewRequest(http.MethodPost, supervisorAuthURL, bytes.NewReader(body))
	if err != nil {
		return false
	}
	req.Header.Set("X-Supervisor-Token", token)
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 5 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		log.Warn().Err(err).Msg("[auth] supervisor auth")
		return false
	}
	defer res.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 4096))
	return res.StatusCode >= 200 && res.StatusCode < 300
}

func safeNext(next string) bool {
	return strings.HasPrefix(next, "/") && !strings.HasPrefix(next, "//") && !strings.Contains(next, "\r") && !strings.Contains(next, "\n")
}

func htmlEscape(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&#39;")
	return r.Replace(s)
}

func renderLogin(next, errMsg string) string {
	// The CSS contains "100%", so fmt.Sprintf would consume the arguments for
	// those percent signs and leave the real placeholders empty.
	return strings.NewReplacer("{{NEXT}}", htmlEscape(next), "{{ERR}}", htmlEscape(errMsg)).
		Replace(loginPage)
}

var sessionSecret []byte

const loginPage = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>A9 Camera – Home Assistant login</title>
  <style>
    :root { --bg:#f6f7f9; --card:#fff; --text:#1c1e21; --muted:#65676b;
      --line:#dfe1e5; --accent:#0b6bcb; --err:#b3261e; }
    @media (prefers-color-scheme: dark) {
      :root { --bg:#111315; --card:#1b1e21; --text:#e4e6eb; --muted:#a0a4ab;
        --line:#2f3338; --accent:#5aa7ff; --err:#ff8a80; }
    }
    * { box-sizing:border-box; }
    body { margin:0; padding:16px; background:var(--bg); color:var(--text);
      font:15px/1.45 system-ui,-apple-system,Segoe UI,Roboto,sans-serif; }
    .card { max-width:420px; margin:8vh auto; padding:24px; background:var(--card);
      border:1px solid var(--line); border-radius:12px; }
    h1 { font-size:20px; margin:0 0 8px; }
    p { color:var(--muted); margin:0 0 18px; }
    label { display:block; font-size:12px; text-transform:uppercase; letter-spacing:.04em;
      color:var(--muted); margin:12px 0 4px; }
    input { width:100%; padding:9px 10px; font:inherit; border:1px solid var(--line);
      border-radius:6px; background:var(--card); color:var(--text); }
    button { margin-top:18px; width:100%; padding:10px; font:inherit; border:0;
      border-radius:6px; background:var(--accent); color:#fff; cursor:pointer; }
    .err { color:var(--err); margin-top:12px; }
  </style>
</head>
<body>
  <div class="card">
    <h1>A9 Camera</h1>
    <p>Log in with your Home Assistant user to open the camera page.</p>
    <form method="post" action="login">
      <input type="hidden" name="next" value="{{NEXT}}">
      <label for="username">Home Assistant user</label>
      <input id="username" name="username" autocomplete="username" required>
      <label for="password">Password</label>
      <input id="password" name="password" type="password" autocomplete="current-password" required>
      <button type="submit">Log in</button>
    </form>
    <div class="err">{{ERR}}</div>
  </div>
</body>
</html>
`
