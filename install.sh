#!/usr/bin/env bash
# Switaculous - Swift & SwiftUI Learning Editor
#
# Builds Switaculous and installs it like any other app:
#   1. installs Go if it is missing (in ~/.local/go, no root needed)
#   2. Linux: builds a .deb or .rpm package and installs it with your package manager
#      macOS: builds Switaculous.app and copies it to Applications
#   3. offers to add Switaculous to your dock
#   4. starts Switaculous
#
# Run it in the Switaculous folder:  bash install.sh
set -euo pipefail

VERSION="1.0.0"
MINIMUM_GO_MINOR_VERSION=24

ask() {
    local answer=""
    read -r -p "$1 [Y/n] " answer </dev/tty || true
    [[ -z "$answer" || "$answer" =~ ^[Yy] ]]
}

step() {
    printf '\n\033[36m==> %s\033[0m\n' "$1"
}

go_minor_version() {
    if ! command -v go >/dev/null 2>&1; then
        echo -1
        return
    fi
    local version
    version="$(go env GOVERSION)"
    if [[ "$version" =~ ^go1\.([0-9]+) ]]; then
        echo "${BASH_REMATCH[1]}"
    else
        echo -1
    fi
}

cpu_architecture() {
    case "$(uname -m)" in
        x86_64 | amd64) echo amd64 ;;
        aarch64 | arm64) echo arm64 ;;
        *) echo "unsupported" ;;
    esac
}

install_go() {
    local os latest archive
    os="$(uname -s | tr '[:upper:]' '[:lower:]')"
    latest="$(curl -fsSL 'https://go.dev/VERSION?m=text' | head -n 1)"
    archive="$latest.$os-$(cpu_architecture).tar.gz"
    step "Downloading $latest"
    curl -fL --progress-bar -o "/tmp/$archive" "https://go.dev/dl/$archive"
    rm -rf "$HOME/.local/go"
    mkdir -p "$HOME/.local"
    tar -C "$HOME/.local" -xzf "/tmp/$archive"
    rm "/tmp/$archive"
    export PATH="$HOME/.local/go/bin:$PATH"
    echo "Installed $latest in ~/.local/go"

    local profile="$HOME/.profile"
    [[ "$os" == "darwin" ]] && profile="$HOME/.zprofile"
    if ! grep -qs '.local/go/bin' "$profile" && ask "Add Go to your PATH in $profile so you can use it later?"; then
        printf '\nexport PATH="$HOME/.local/go/bin:$PATH"\n' >>"$profile"
    fi
}

start_built_copy() {
    step "Starting Switaculous"
    echo "You can start it again any time with: $1"
    nohup "$1" >/dev/null 2>&1 &
}

# ---------- Linux ----------

linux_package_format() {
    if command -v apt-get >/dev/null 2>&1 || command -v dpkg >/dev/null 2>&1; then
        echo deb
    elif command -v dnf >/dev/null 2>&1 || command -v yum >/dev/null 2>&1 || command -v zypper >/dev/null 2>&1; then
        echo rpm
    else
        echo none
    fi
}

install_linux_package() {
    local package="$1"
    if [[ "$package" == *.deb ]]; then
        if command -v apt-get >/dev/null 2>&1; then
            sudo apt-get install -y "./$package"
        else
            sudo dpkg -i "$package"
        fi
    elif command -v dnf >/dev/null 2>&1; then
        sudo dnf install -y "./$package"
    elif command -v zypper >/dev/null 2>&1; then
        sudo zypper --non-interactive install --allow-unsigned-rpm "./$package"
    else
        sudo yum install -y "./$package"
    fi
}

# GNOME (Ubuntu, Fedora) keeps its dock in the favorite-apps setting. Other desktops
# don't let scripts change their panels, so there we explain how to pin it.
add_to_linux_dock() {
    if command -v gsettings >/dev/null 2>&1 && gsettings get org.gnome.shell favorite-apps >/dev/null 2>&1; then
        local favorites
        favorites="$(gsettings get org.gnome.shell favorite-apps)"
        if [[ "$favorites" == *"switaculous.desktop"* ]]; then
            return
        fi
        if [[ "$favorites" == "@as []" || "$favorites" == "[]" ]]; then
            favorites="['switaculous.desktop']"
        else
            favorites="${favorites%]}, 'switaculous.desktop']"
        fi
        gsettings set org.gnome.shell favorite-apps "$favorites"
        echo "Added Switaculous to your dock."
    else
        echo "Your desktop doesn't let scripts change the panel. Open your applications menu,"
        echo "right-click Switaculous and choose the option to pin or add it to the panel."
    fi
}

install_on_linux() {
    local arch="$1" format package
    format="$(linux_package_format)"
    step "Building Switaculous"
    go build -trimpath -ldflags "-s -w" -o dist/switaculous .
    if [[ "$format" == "none" ]]; then
        echo "No apt, dnf, yum or zypper found, so Switaculous runs from this folder instead."
        start_built_copy "$PWD/dist/switaculous"
        return
    fi

    step "Building the .$format package"
    go run ./tools/package -version "$VERSION" -arch "$arch" -binary dist/switaculous -out dist -formats "$format"
    if [[ "$format" == "deb" ]]; then
        package="dist/switaculous_${VERSION}_${arch}.deb"
    else
        local rpm_arch="x86_64"
        [[ "$arch" == "arm64" ]] && rpm_arch="aarch64"
        package="dist/switaculous-${VERSION}-1.${rpm_arch}.rpm"
    fi

    if ! ask "Install Switaculous now? It will be added to your applications menu (asks for your password)"; then
        echo "The package is ready whenever you want it: $package"
        start_built_copy "$PWD/dist/switaculous"
        return
    fi
    install_linux_package "$package"
    if ask "Add Switaculous to your dock?"; then
        add_to_linux_dock
    fi
    start_built_copy "$(command -v switaculous)"
}

# ---------- macOS ----------

write_info_plist() {
    cat >"$1" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>CFBundleName</key><string>Switaculous</string>
    <key>CFBundleDisplayName</key><string>Switaculous</string>
    <key>CFBundleIdentifier</key><string>io.github.switaculous</string>
    <key>CFBundleVersion</key><string>$VERSION</string>
    <key>CFBundleShortVersionString</key><string>$VERSION</string>
    <key>CFBundlePackageType</key><string>APPL</string>
    <key>CFBundleExecutable</key><string>switaculous</string>
    <key>CFBundleIconFile</key><string>switaculous</string>
    <key>LSMinimumSystemVersion</key><string>11.0</string>
    <!-- Switaculous opens in the browser, so it has no window or menu bar of its own. -->
    <key>LSUIElement</key><true/>
</dict>
</plist>
PLIST
}

install_on_mac() {
    local app="dist/Switaculous.app"
    step "Building Switaculous.app"
    rm -rf "$app"
    mkdir -p "$app/Contents/MacOS" "$app/Contents/Resources"
    go build -trimpath -ldflags "-s -w" -o "$app/Contents/MacOS/switaculous" .
    cp packaging/macos/switaculous.icns "$app/Contents/Resources/switaculous.icns"
    write_info_plist "$app/Contents/Info.plist"

    if ! ask "Install Switaculous in your Applications folder?"; then
        start_built_copy "$PWD/$app/Contents/MacOS/switaculous"
        return
    fi
    local applications="/Applications"
    [[ -w "$applications" ]] || applications="$HOME/Applications"
    mkdir -p "$applications"
    rm -rf "$applications/Switaculous.app"
    cp -R "$app" "$applications/"
    echo "Installed $applications/Switaculous.app"

    if ask "Add Switaculous to the Dock?"; then
        defaults write com.apple.dock persistent-apps -array-add "<dict><key>tile-data</key><dict><key>file-data</key><dict><key>_CFURLString</key><string>$applications/Switaculous.app</string><key>_CFURLStringType</key><integer>0</integer></dict></dict></dict>"
        killall Dock
    fi
    step "Starting Switaculous"
    open "$applications/Switaculous.app"
}

# ---------- main ----------

cd "$(dirname "$0")"

echo "Switaculous - Swift & SwiftUI Learning Editor"
arch="$(cpu_architecture)"
if [[ "$arch" == "unsupported" ]]; then
    echo "Switaculous runs on 64-bit Intel/AMD and ARM computers; this one is $(uname -m)."
    exit 1
fi

if (($(go_minor_version) < MINIMUM_GO_MINOR_VERSION)); then
    echo "Switaculous is built with Go 1.$MINIMUM_GO_MINOR_VERSION or newer, which isn't installed yet."
    if ! ask "Download and install Go now?"; then
        echo "Install Go from https://go.dev/doc/install and run this script again."
        exit 1
    fi
    install_go
fi
echo "Using $(go env GOVERSION)"

case "$(uname -s)" in
    Linux) install_on_linux "$arch" ;;
    Darwin) install_on_mac ;;
    *) echo "Use install.ps1 on Windows." && exit 1 ;;
esac
