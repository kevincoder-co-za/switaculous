package main

import (
	"embed"
	"strings"

	"switaculous/internal/switaculous"
)

//go:embed VERSION
var versionFile string

//go:embed web/*
var webFiles embed.FS

//go:embed examples
var demoFiles embed.FS

func main() {
	switaculous.Run(strings.TrimSpace(versionFile), webFiles, demoFiles)
}
