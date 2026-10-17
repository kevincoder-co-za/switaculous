package switaculous

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"time"
)

// The version number doesn't change between builds, so each executable is
// identified by a hash of its own file. Reinstalling replaces the exe on disk
// but leaves an already-running server alive; comparing builds lets the new
// copy take over instead of handing the user back to the old code.
var currentBuildID = sync.OnceValue(func() string {
	path, err := os.Executable()
	if err != nil {
		return ""
	}
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return ""
	}
	return hex.EncodeToString(hash.Sum(nil))[:16]
})

// Asks a running server from a different build to quit and waits for it to
// go away. Returns true when the old server stopped and this one should start.
func stopOutdatedInstance(info runningServerInfo) bool {
	if info.Build == currentBuildID() {
		return false
	}
	url := fmt.Sprintf("http://127.0.0.1:%d/api/quit", info.Port)
	response, err := http.Post(url, "application/json", nil)
	if err != nil {
		return false
	}
	response.Body.Close()
	fmt.Fprintln(consoleOutput, "Stopping the Switaculous that was already running (an older build) and starting this one.")
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, stillRunning := readServerInfo(); !stillRunning {
			return true
		}
		time.Sleep(150 * time.Millisecond)
	}
	return false
}
