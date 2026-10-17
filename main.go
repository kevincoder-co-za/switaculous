package main

import (
	"embed"

	"switaculous/internal/switaculous"
)

//go:embed web/*
var webFiles embed.FS

//go:embed examples
var demoFiles embed.FS

func main() {
	switaculous.Run(webFiles, demoFiles)
}
