package switaculous

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"

	qrcode "github.com/skip2/go-qrcode"
)

const shareCookieName = "switaculous-share"

type shareAccess struct {
	token string
	port  int
}

func newShareAccess(port int) *shareAccess {
	raw := make([]byte, 12)
	rand.Read(raw)
	return &shareAccess{token: hex.EncodeToString(raw), port: port}
}

func isLoopbackRequest(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// Phones on the Wi-Fi only get the preview: they can look and tap, but never
// read or change project files. The token from the QR code is required, and
// is remembered in a cookie so the page's own requests work after the scan.
func (s *shareAccess) guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isLoopbackRequest(r) {
			next.ServeHTTP(w, r)
			return
		}
		if token := r.URL.Query().Get("t"); token != "" && token == s.token {
			http.SetCookie(w, &http.Cookie{Name: shareCookieName, Value: token, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode})
		} else if cookie, err := r.Cookie(shareCookieName); err != nil || cookie.Value != s.token {
			http.Error(w, "Scan the QR code in Switaculous to open the preview on this device.", http.StatusForbidden)
			return
		}
		if !s.allowedFromNetwork(r.URL.Path) {
			http.Error(w, "This action is only available on the computer running Switaculous.", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Browsers send an Origin header with a POST. Only the Switaculous page itself
// may change files, so a web page on another site can't write into the project
// or stop the program behind the user's back.
func rejectCrossSiteRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		isChange := r.Method != http.MethodGet && r.Method != http.MethodHead
		if isChange && origin != "" && origin != "http://"+r.Host {
			http.Error(w, "Requests from other websites are not allowed.", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *shareAccess) allowedFromNetwork(path string) bool {
	if !strings.HasPrefix(path, "/api/") {
		return true
	}
	switch path {
	case "/api/event", "/api/geo":
		return true
	}
	return false
}

// Home and office networks use private addresses, so those are listed first.
func lanAddresses() []string {
	var private, other []string
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok || ipNet.IP.To4() == nil || ipNet.IP.IsLinkLocalUnicast() {
				continue
			}
			if ipNet.IP.IsPrivate() {
				private = append(private, ipNet.IP.String())
			} else {
				other = append(other, ipNet.IP.String())
			}
		}
	}
	return append(private, other...)
}

func (s *shareAccess) shareURL() string {
	addresses := lanAddresses()
	if len(addresses) == 0 {
		return ""
	}
	return fmt.Sprintf("http://%s:%d/?view=device&t=%s", addresses[0], s.port, s.token)
}

func (s *shareAccess) mcpURL() string {
	return fmt.Sprintf("http://127.0.0.1:%d/mcp", s.port)
}

func (s *shareAccess) claudeCodeCommand() string {
	return "claude mcp add --transport http switaculous " + s.mcpURL()
}

func (s *shareAccess) claudeCodeJSON() string {
	config := map[string]interface{}{"mcpServers": map[string]interface{}{"switaculous": map[string]interface{}{"type": "http", "url": s.mcpURL()}}}
	encoded, _ := json.MarshalIndent(config, "", "  ")
	return string(encoded)
}

func claudeCodeStdioCommand(executable string) string {
	return fmt.Sprintf(`claude mcp add switaculous -- "%s" -mcp`, executable)
}

func (s *shareAccess) registerAPI(mux *http.ServeMux) {
	mux.HandleFunc("/api/share", func(w http.ResponseWriter, r *http.Request) {
		if !isLoopbackRequest(r) {
			http.NotFound(w, r)
			return
		}
		executable, _ := os.Executable()
		writeJSON(w, map[string]interface{}{
			"url": s.shareURL(), "addresses": lanAddresses(),
			"mcpURL":          s.mcpURL(),
			"mcpCommand":      s.claudeCodeCommand(),
			"mcpJSON":         s.claudeCodeJSON(),
			"mcpStdioCommand": claudeCodeStdioCommand(executable),
			"port":            s.port,
		})
	})
	mux.HandleFunc("/api/share/qr.png", func(w http.ResponseWriter, r *http.Request) {
		url := s.shareURL()
		if !isLoopbackRequest(r) || url == "" {
			http.NotFound(w, r)
			return
		}
		png, err := qrcode.Encode(url, qrcode.Medium, 320)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "no-store")
		w.Write(png)
	})
}
