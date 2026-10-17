// Builds the Linux packages for a release: a .deb through dpkg-deb and an .rpm
// through rpmpack, so neither needs the distribution's own tools installed.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/rpmpack"
)

type packageFiles struct {
	binary  string
	desktop string
	icon    string
	iconSVG string
	readme  string
	license string
	notices string
}

func main() {
	version := flag.String("version", "", "package version (default: the VERSION file)")
	arch := flag.String("arch", "amd64", "amd64 or arm64")
	binary := flag.String("binary", "", "the switaculous binary for this architecture")
	outDir := flag.String("out", "dist", "where to write the packages")
	formats := flag.String("formats", "deb,rpm", "which packages to build: deb, rpm or both")
	flag.Parse()
	if *binary == "" {
		fmt.Fprintln(os.Stderr, "-binary is required")
		os.Exit(1)
	}
	if *version == "" {
		content, err := os.ReadFile("VERSION")
		if err != nil {
			fmt.Fprintln(os.Stderr, "no -version given and no VERSION file found:", err)
			os.Exit(1)
		}
		*version = strings.TrimSpace(string(content))
	}
	files := packageFiles{
		binary:  *binary,
		desktop: "packaging/linux/switaculous.desktop",
		icon:    "packaging/linux/switaculous-256.png",
		iconSVG: "packaging/icon/switaculous.svg",
		readme:  "README.md",
		license: "LICENSE",
		notices: "THIRD_PARTY_NOTICES.md",
	}
	os.MkdirAll(*outDir, 0755)
	if strings.Contains(*formats, "deb") {
		if err := buildDeb(files, *version, *arch, *outDir); err != nil {
			fmt.Fprintln(os.Stderr, "deb:", err)
			os.Exit(1)
		}
	}
	if strings.Contains(*formats, "rpm") {
		if err := buildRPM(files, *version, *arch, *outDir); err != nil {
			fmt.Fprintln(os.Stderr, "rpm:", err)
			os.Exit(1)
		}
	}
}

const description = "Swift & SwiftUI Learning Editor\n Write Swift and SwiftUI code on Linux and see it run in a live iPhone or iPad preview in the browser."

func buildDeb(files packageFiles, version, arch, outDir string) error {
	root, err := os.MkdirTemp("", "switaculous-deb")
	if err != nil {
		return err
	}
	defer os.RemoveAll(root)
	os.Chmod(root, 0755)
	copies := map[string]string{
		"usr/bin/switaculous":                                   files.binary,
		"usr/share/applications/switaculous.desktop":            files.desktop,
		"usr/share/icons/hicolor/256x256/apps/switaculous.png":  files.icon,
		"usr/share/icons/hicolor/scalable/apps/switaculous.svg": files.iconSVG,
		"usr/share/doc/switaculous/README.md":                   files.readme,
		"usr/share/doc/switaculous/copyright":                   files.license,
		"usr/share/doc/switaculous/THIRD_PARTY_NOTICES.md":      files.notices,
	}
	for target, source := range copies {
		if err := copyFile(source, filepath.Join(root, target), target == "usr/bin/switaculous"); err != nil {
			return err
		}
	}
	control := fmt.Sprintf(`Package: switaculous
Version: %s
Section: devel
Priority: optional
Architecture: %s
Maintainer: Switaculous contributors
Homepage: https://github.com/
Description: %s
`, version, arch, description)
	os.MkdirAll(filepath.Join(root, "DEBIAN"), 0755)
	if err := os.WriteFile(filepath.Join(root, "DEBIAN", "control"), []byte(control), 0644); err != nil {
		return err
	}
	out := filepath.Join(outDir, fmt.Sprintf("switaculous_%s_%s.deb", version, arch))
	// xz keeps the package installable on older Debian and Ubuntu releases.
	cmd := exec.Command("dpkg-deb", "--build", "--root-owner-group", "-Zxz", root, out)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}
	fmt.Println("wrote", out)
	return nil
}

func buildRPM(files packageFiles, version, arch, outDir string) error {
	rpmArch := map[string]string{"amd64": "x86_64", "arm64": "aarch64"}[arch]
	if rpmArch == "" {
		return fmt.Errorf("unknown architecture %s", arch)
	}
	pkg, err := rpmpack.NewRPM(rpmpack.RPMMetaData{
		Name:        "switaculous",
		Version:     version,
		Release:     "1",
		Arch:        rpmArch,
		Summary:     "Swift & SwiftUI Learning Editor",
		Description: description,
		Licence:     "MIT",
		Group:       "Development/Tools",
		BuildTime:   time.Now(),
	})
	if err != nil {
		return err
	}
	add := func(target, source string, mode uint) error {
		content, err := os.ReadFile(source)
		if err != nil {
			return err
		}
		pkg.AddFile(rpmpack.RPMFile{Name: target, Body: content, Mode: mode, Owner: "root", Group: "root"})
		return nil
	}
	entries := []struct {
		target, source string
		mode           uint
	}{
		{"/usr/bin/switaculous", files.binary, 0755},
		{"/usr/share/applications/switaculous.desktop", files.desktop, 0644},
		{"/usr/share/icons/hicolor/256x256/apps/switaculous.png", files.icon, 0644},
		{"/usr/share/icons/hicolor/scalable/apps/switaculous.svg", files.iconSVG, 0644},
		{"/usr/share/doc/switaculous/README.md", files.readme, 0644},
		{"/usr/share/doc/switaculous/LICENSE", files.license, 0644},
		{"/usr/share/doc/switaculous/THIRD_PARTY_NOTICES.md", files.notices, 0644},
	}
	for _, entry := range entries {
		if err := add(entry.target, entry.source, entry.mode); err != nil {
			return err
		}
	}
	out := filepath.Join(outDir, fmt.Sprintf("switaculous-%s-1.%s.rpm", version, rpmArch))
	file, err := os.Create(out)
	if err != nil {
		return err
	}
	defer file.Close()
	if err := pkg.Write(file); err != nil {
		return err
	}
	fmt.Println("wrote", out)
	return nil
}

func copyFile(source, target string, executable bool) error {
	content, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return err
	}
	mode := os.FileMode(0644)
	if executable {
		mode = 0755
	}
	return os.WriteFile(target, content, mode)
}
