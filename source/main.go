package main

import (
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"time"
)

//go:embed web/*
var webFS embed.FS

const version = "1.0.0"

func printStack() { debug.PrintStack() }

// Where startup messages and print() output go. The MCP mode talks JSON on
// stdout, so there everything else moves to stderr.
var consoleOutput io.Writer = os.Stdout

func main() {
	port := flag.Int("port", 8642, "port for the preview server")
	noOpen := flag.Bool("no-open", false, "don't open the browser automatically")
	check := flag.Bool("check", false, "parse and render once, print errors, then exit")
	dump := flag.Bool("dump", false, "with -check: print the rendered tree as JSON")
	localOnly := flag.Bool("local-only", false, "don't allow phones and tablets on the Wi-Fi to open the preview")
	mcp := flag.Bool("mcp", false, "run as an MCP server on stdin/stdout for Claude Code and other assistants")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "SwiftUI Tutor %s — live SwiftUI previews\n\nUsage: swiftui-tutor [flags] [project-folder]\n\n", version)
		flag.PrintDefaults()
	}
	flag.Parse()
	if *mcp {
		consoleOutput = os.Stderr
		runMCPServer(flag.Arg(0), *port, *noOpen)
		return
	}
	dir := "MyApp"
	if flag.NArg() > 0 {
		dir = flag.Arg(0)
	}
	if *check {
		checkProject(dir, *dump)
		return
	}
	serveProject(dir, *port, *noOpen, *localOnly, nil)
}

func prepareProjectFolder(dir string) string {
	if absDir, err := filepath.Abs(dir); err == nil {
		dir = absDir
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if created := ensureStarter(dir); created {
		fmt.Fprintf(consoleOutput, "Created a starter project in %s\n", dir)
	}
	return dir
}

func checkProject(dir string, dump bool) {
	dir = prepareProjectFolder(dir)
	rt := NewRuntime(dir)
	rt.mu.Lock()
	rt.files, _ = rt.scan()
	rt.reloadLocked()
	rt.mu.Unlock()
	for _, e := range rt.errs {
		fmt.Printf("%s:%d:%d: error: %s\n%s\n", e.File, e.Line, e.Col, e.Msg, e.Snippet)
	}
	for _, w := range rt.warnList {
		fmt.Println("warning:", w)
	}
	if dump && rt.lastPayload != nil {
		os.Stdout.Write(rt.lastPayload)
		fmt.Println()
	}
	if len(rt.errs) > 0 {
		os.Exit(1)
	}
	fmt.Println("OK")
}

// Runs the preview server until the process ends. When ready is given, the
// server's local URL is sent on it once it is listening (used by the MCP mode).
func serveProject(dir string, port int, noOpen, localOnly bool, ready chan<- string) {
	dir = prepareProjectFolder(dir)
	rt := NewRuntime(dir)
	saveRecentProject(dir)
	go warmUpSQLite()
	go rt.Watch()

	sub, _ := fs.Sub(webFS, "web")
	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.FS(sub)))
	mux.HandleFunc("/events", rt.serveEvents)
	mux.HandleFunc("/api/event", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID string      `json:"id"`
			V  interface{} `json:"v"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		rt.HandleEvent(req.ID, req.V)
		w.WriteHeader(204)
	})
	mux.HandleFunc("/api/settings", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Dark      *bool  `json:"dark"`
			Device    string `json:"device"`
			Preview   string `json:"preview"`
			Reset     bool   `json:"reset"`
			EraseData bool   `json:"eraseData"`
			Landscape *bool  `json:"landscape"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		rt.mu.Lock()
		if req.Dark != nil {
			rt.store.dark = *req.Dark
		}
		if req.Device != "" {
			if _, ok := devices[req.Device]; ok {
				rt.store.device = req.Device
				rt.store.geo = map[string]*GeoV{}
			}
		}
		if req.Landscape != nil {
			rt.store.landscape = *req.Landscape
			rt.store.geo = map[string]*GeoV{}
		}
		if req.Preview != "" {
			rt.pinned = req.Preview
			if req.Preview == "auto" {
				rt.lastEdited = ""
			}
		}
		if req.EraseData {
			rt.eraseModelDataLocked()
			rt.logf("info", "Erased all SwiftData data")
		}
		if req.Reset || req.EraseData {
			st := newStore()
			st.dark, st.device, st.landscape = rt.store.dark, rt.store.device, rt.store.landscape
			rt.store = st
			if rt.ip != nil {
				rt.ip.store = st
			}
			for k, t := range rt.tickers {
				close(t.stop)
				delete(rt.tickers, k)
			}
			rt.gen++
			if rt.ip != nil {
				rt.ip.gen = rt.gen
			}
		}
		rt.renderLocked()
		rt.mu.Unlock()
		w.WriteHeader(204)
	})
	mux.HandleFunc("/api/geo", func(w http.ResponseWriter, r *http.Request) {
		var req []struct {
			ID string  `json:"id"`
			W  float64 `json:"w"`
			H  float64 `json:"h"`
			X  float64 `json:"x"`
			Y  float64 `json:"y"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		rt.mu.Lock()
		changed := false
		for _, g := range req {
			old := rt.store.geo[g.ID]
			if old == nil || abs(old.W-g.W) > 0.5 || abs(old.H-g.H) > 0.5 {
				rt.store.geo[g.ID] = &GeoV{W: g.W, H: g.H, X: g.X, Y: g.Y}
				changed = true
			}
		}
		if changed {
			rt.renderLocked()
		}
		rt.mu.Unlock()
		w.WriteHeader(204)
	})
	rt.registerFileAPI(mux)
	rt.registerAssetAPI(mux)
	rt.registerAssistantAPI(mux)
	mux.HandleFunc("/blob/", rt.serveImageBlob)
	mux.HandleFunc("/asset/", func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/asset/")
		rt.mu.Lock()
		p, ok := rt.assets[name]
		rt.mu.Unlock()
		if !ok {
			http.NotFound(w, r)
			return
		}
		http.ServeFile(w, r, p)
	})

	listenHost := ""
	if localOnly {
		listenHost = "127.0.0.1"
	}
	ln, err := net.Listen("tcp", fmt.Sprintf("%s:%d", listenHost, port))
	if err != nil {
		ln, err = net.Listen("tcp", listenHost+":0")
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			time.Sleep(10 * time.Second)
			os.Exit(1)
		}
	}
	listenPort := ln.Addr().(*net.TCPAddr).Port
	url := fmt.Sprintf("http://127.0.0.1:%d/", listenPort)
	writeServerInfo(listenPort, dir)
	share := newShareAccess(listenPort)
	share.registerAPI(mux)
	var handler http.Handler = mux
	if !localOnly {
		handler = share.guard(mux)
	}
	fmt.Fprintf(consoleOutput, "\n  SwiftUI Tutor %s\n  Watching: %s\n  Preview:  %s\n\n  Edit any .swift file in that folder and the preview updates when you save.\n  Click Share in the top bar to open the preview on your phone or tablet.\n  Press Ctrl+C to stop.\n\n", version, dir, url)
	if !noOpen {
		go func() {
			time.Sleep(400 * time.Millisecond)
			openBrowser(url)
		}()
	}
	if ready != nil {
		ready <- strings.TrimSuffix(url, "/")
	}
	http.Serve(ln, handler)
}

func abs(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}

func (rt *Runtime) serveEvents(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", 500)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	ch := make(chan []byte, 16)
	rt.cmu.Lock()
	rt.clients[ch] = true
	rt.cmu.Unlock()
	defer func() {
		rt.cmu.Lock()
		delete(rt.clients, ch)
		rt.cmu.Unlock()
	}()
	rt.mu.Lock()
	first := rt.lastPayload
	rt.mu.Unlock()
	if first != nil {
		fmt.Fprintf(w, "data: %s\n\n", first)
		fl.Flush()
	}
	keep := time.NewTicker(15 * time.Second)
	defer keep.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case b := <-ch:
			// Only the newest render matters, so skip any that queued up while this browser was busy.
			for len(ch) > 0 {
				b = <-ch
			}
			fmt.Fprintf(w, "data: %s\n\n", b)
			fl.Flush()
		case <-keep.C:
			fmt.Fprintf(w, ": keepalive\n\n")
			fl.Flush()
		}
	}
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	cmd.Start()
}

func ensureStarter(dir string) bool {
	found := false
	filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && strings.HasSuffix(p, ".swift") {
			found = true
			return filepath.SkipAll
		}
		if err == nil && info.IsDir() && p != dir && skipDir(info.Name()) {
			return filepath.SkipDir
		}
		return nil
	})
	if found {
		return false
	}
	os.WriteFile(filepath.Join(dir, "MyApp.swift"), []byte(starterApp), 0644)
	os.WriteFile(filepath.Join(dir, "ContentView.swift"), []byte(starterContent), 0644)
	return true
}

const starterApp = `import SwiftUI

@main
struct MyApp: App {
    var body: some Scene {
        WindowGroup {
            ContentView()
        }
    }
}
`

const starterContent = `import SwiftUI

struct ContentView: View {
    @State private var count = 0
    @State private var name = ""

    var body: some View {
        NavigationStack {
            VStack(spacing: 20) {
                Image(systemName: "hand.wave")
                    .font(.system(size: 60))
                    .foregroundStyle(.orange)

                Text("Hello, \(name.isEmpty ? "SwiftUI" : name)!")
                    .font(.title)
                    .bold()

                TextField("Your name", text: $name)
                    .textFieldStyle(.roundedBorder)

                Text("Tapped \(count) times")
                    .foregroundStyle(.secondary)

                Button("Tap me") {
                    count += 1
                }
                .buttonStyle(.borderedProminent)
            }
            .padding()
            .navigationTitle("Welcome")
        }
    }
}

#Preview {
    ContentView()
}
`
