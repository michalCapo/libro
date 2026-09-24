SHELL := /usr/bin/bash
.DEFAULT_GOAL := help
.ONESHELL:
.PHONY: help dev check install release release-dry-run

help: ## Show available commands
	@awk 'BEGIN {FS = ":.*## "} /^[a-zA-Z0-9_-]+:.*## / {printf "  %-18s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

dev: ## Run an isolated development instance on port 8101
	go run . --dev

check: ## Run all code checks
	@set -uo pipefail
	
	ROOT_DIR="$(CURDIR)"
	cd "$$ROOT_DIR"
	
	status=0
	GO_FILES=()
	while IFS= read -r -d '' file; do
	  GO_FILES+=("$$file")
	done < <(find . -name '*.go' -not -path './third_party/*' -print0)
	
	run() {
	  echo "==> $$*"
	  "$$@"
	  local code=$$?
	  if [[ $$code -ne 0 ]]; then
	    status=1
	    echo "!! failed: $$*" >&2
	  fi
	  echo
	}
	
	# Keep checks non-mutating and fail when go fix would change sources.
	run go fix -diff ./...
	
	echo "==> gofmt"
	unformatted="$$(gofmt -l "$${GO_FILES[@]}")"
	if [[ -n "$$unformatted" ]]; then
	  status=1
	  echo "The following Go files are not formatted:" >&2
	  echo "$$unformatted" >&2
	fi
	echo
	
	run go vet ./...
	run staticcheck ./...
	
	echo "==> gopls check"
	if command -v gopls >/dev/null 2>&1; then
	  gopls check -severity=hint "$${GO_FILES[@]}"
	  code=$$?
	  if [[ $$code -ne 0 ]]; then
	    status=1
	    echo "!! failed: gopls check" >&2
	  fi
	else
	  echo "gopls not found; skipping"
	fi
	echo
	
	echo "==> golangci-lint"
	if command -v golangci-lint >/dev/null 2>&1; then
	  golangci-lint run --max-issues-per-linter=0 --max-same-issues=0 ./...
	  code=$$?
	  if [[ $$code -ne 0 ]]; then
	    status=1
	    echo "!! failed: golangci-lint run" >&2
	  fi
	  echo
	
	  echo "==> golangci-lint revive lsp-style diagnostics"
	  revive_config="$$(mktemp --suffix=.yml)"
	  cat > "$$revive_config" <<'YAML'
	version: "2"
	linters:
	  default: none
	  enable: [revive]
	  settings:
	    revive:
	      rules: [{name: unused-parameter}, {name: package-comments}]
	YAML
	  golangci-lint run --config "$$revive_config" --max-issues-per-linter=0 --max-same-issues=0 ./...
	  code=$$?
	  rm -f "$$revive_config"
	  if [[ $$code -ne 0 ]]; then
	    status=1
	    echo "!! failed: golangci-lint revive lsp-style diagnostics" >&2
	  fi
	else
	  echo "golangci-lint not found; skipping"
	fi
	echo
	
	run deadcode -test ./...
	run go build ./...
	run go test ./...
	
	if [[ $$status -eq 0 ]]; then
	  echo "All checks passed."
	else
	  echo "Checks finished with errors/warnings." >&2
	fi
	exit "$$status"

install: ## Build and install Libro locally
	@set -euo pipefail
	
	# ---------------------------------------------------------------------------
	# install — Build Libro from source and install it locally.
	#
	# This path is for repo-based installs. It installs the Go binary plus the
	# Electron app files under ~/.local/share/libro and links the launcher into
	# ~/.local/bin. Release downloads are self-contained and do not use this flow.
	# ---------------------------------------------------------------------------
	
	SCRIPT_DIR="$(CURDIR)"
	VERSION_FILE="$$SCRIPT_DIR/VERSION"
	MODULE="libro"
	
	# ---------------------------------------------------------------------------
	# Helpers
	# ---------------------------------------------------------------------------
	
	info() { echo -e "\033[1;34m==>\033[0m $$*"; }
	ok() { echo -e "\033[1;32m==>\033[0m $$*"; }
	err() { echo -e "\033[1;31m==>\033[0m $$*" >&2; }
	
	copy_if_changed() {
	    local source="$$1"
	    local target="$$2"
	    mkdir -p "$$(dirname "$$target")"
	    if [[ -f "$$target" ]] && cmp -s "$$source" "$$target"; then
	        return 1
	    fi
	    cp "$$source" "$$target"
	    return 0
	}
	
	# ---------------------------------------------------------------------------
	# Detect OS and set install directory + binary name
	# ---------------------------------------------------------------------------
	
	OS="$$(uname -s)"
	ARCH="$$(uname -m)"
	
	case "$$ARCH" in
	x86_64) GOARCH="amd64" ;;
	aarch64) GOARCH="arm64" ;;
	arm64) GOARCH="arm64" ;;
	*) GOARCH="$$ARCH" ;;
	esac
	
	BIN_NAME="libro"
	INSTALL_DIR="$$HOME/.local/bin"
	
	case "$$OS" in
	Linux)
	    GOOS="linux"
	    ;;
	Darwin)
	    GOOS="darwin"
	    ;;
	MINGW* | MSYS* | CYGWIN*)
	    GOOS="windows"
	    BIN_NAME="libro.exe"
	    INSTALL_DIR="$$USERPROFILE/.local/bin"
	    # Fallback if USERPROFILE is not set
	    if [[ -z "$$INSTALL_DIR" || "$$INSTALL_DIR" == "/.local/bin" ]]; then
	        INSTALL_DIR="$$HOME/.local/bin"
	    fi
	    ;;
	*)
	    err "Unsupported OS: $$OS"
	    exit 1
	    ;;
	esac
	
	info "OS: $$GOOS/$$GOARCH"
	info "Install directory: $$INSTALL_DIR"
	
	# ---------------------------------------------------------------------------
	# Read version
	# ---------------------------------------------------------------------------
	
	VERSION="dev"
	if [[ -f "$$VERSION_FILE" ]]; then
	    VERSION=$$(cat "$$VERSION_FILE" | tr -d '[:space:]')
	fi
	
	LDFLAGS="-X $${MODULE}/internal/version.Version=$${VERSION}"
	
	# ---------------------------------------------------------------------------
	# Cleanup
	# ---------------------------------------------------------------------------
	
	info "Cleaning up..."
	rm -f "$$SCRIPT_DIR/libro" "$$SCRIPT_DIR/libro.exe"
	go clean -cache -testcache 2>/dev/null || true
	go mod tidy
	ok "Clean"
	
	# ---------------------------------------------------------------------------
	# Embed Windows resources (icon + version info)
	# ---------------------------------------------------------------------------
	
	if [[ "$$GOOS" == "windows" ]]; then
	    if command -v go-winres &>/dev/null; then
	        info "Embedding Windows resources..."
	        (cd "$$SCRIPT_DIR" && go-winres make --product-version="$${VERSION}" --file-version="$${VERSION}")
	        ok "Windows resources generated"
	    else
	        info "go-winres not found — skipping icon embedding (install: go install github.com/tc-hib/go-winres@latest)"
	    fi
	fi
	
	# ---------------------------------------------------------------------------
	# Build from the current repo checkout
	# ---------------------------------------------------------------------------
	
	info "Building libro v$${VERSION}..."
	GOOS="$$GOOS" GOARCH="$$GOARCH" go build -ldflags "$$LDFLAGS" -o "$$SCRIPT_DIR/$$BIN_NAME" .
	ok "Built $$BIN_NAME"
	
	# Cleanup .syso files from go-winres
	rm -f "$$SCRIPT_DIR/"*.syso
	
	# ---------------------------------------------------------------------------
	# Install the source-built app bundle
	# ---------------------------------------------------------------------------
	
	LIBRO_DIR="$$HOME/.local/share/libro"
	
	mkdir -p "$$INSTALL_DIR" "$$LIBRO_DIR/electron" "$$LIBRO_DIR/winres"
	
	# Copy Go binary
	mv "$$SCRIPT_DIR/$$BIN_NAME" "$$LIBRO_DIR/$$BIN_NAME"
	chmod +x "$$LIBRO_DIR/$$BIN_NAME"
	
	# Copy Electron app files used by repo-based installs
	APP_FILES_CHANGED=false
	if copy_if_changed "$$SCRIPT_DIR/package.json" "$$LIBRO_DIR/package.json"; then APP_FILES_CHANGED=true; fi
	if copy_if_changed "$$SCRIPT_DIR/package-lock.json" "$$LIBRO_DIR/package-lock.json"; then APP_FILES_CHANGED=true; fi
	if copy_if_changed "$$SCRIPT_DIR/electron/main.js" "$$LIBRO_DIR/electron/main.js"; then APP_FILES_CHANGED=true; fi
	if copy_if_changed "$$SCRIPT_DIR/electron/linux-theme.js" "$$LIBRO_DIR/electron/linux-theme.js"; then APP_FILES_CHANGED=true; fi
	if copy_if_changed "$$SCRIPT_DIR/electron/agent-control.js" "$$LIBRO_DIR/electron/agent-control.js"; then APP_FILES_CHANGED=true; fi
	if copy_if_changed "$$SCRIPT_DIR/electron/page-area.js" "$$LIBRO_DIR/electron/page-area.js"; then APP_FILES_CHANGED=true; fi
	if copy_if_changed "$$SCRIPT_DIR/electron/preload.js" "$$LIBRO_DIR/electron/preload.js"; then APP_FILES_CHANGED=true; fi
	if copy_if_changed "$$SCRIPT_DIR/electron/webview-preload.js" "$$LIBRO_DIR/electron/webview-preload.js"; then APP_FILES_CHANGED=true; fi
	if copy_if_changed "$$SCRIPT_DIR/winres/icon.png" "$$LIBRO_DIR/winres/icon.png"; then APP_FILES_CHANGED=true; fi
	
	# Install or refresh the local Electron runtime only when needed
	if [[ ! -d "$$LIBRO_DIR/node_modules/electron" || "$$APP_FILES_CHANGED" == true ]]; then
	    info "Installing Electron..."
	    (cd "$$LIBRO_DIR" && npm install --no-fund --no-audit --omit=dev 2>&1 | tail -1)
	    ok "Electron installed"
	else
	    ok "Electron already installed"
	fi
	
	# Create launcher symlink in bin dir
	ln -sf "$$LIBRO_DIR/$$BIN_NAME" "$$INSTALL_DIR/$$BIN_NAME"
	ok "Installed to $$LIBRO_DIR (symlink at $$INSTALL_DIR/$$BIN_NAME)"

	# Preload offline dictation. A failed download is retried automatically on launch.
	info "Preparing offline voice typing..."
	if "$$LIBRO_DIR/$$BIN_NAME" voice install; then
	    ok "Voice typing ready"
	else
	    info "Voice setup will retry when Libro opens."
	fi
	
	# ---------------------------------------------------------------------------
	# Desktop integration (Linux)
	# ---------------------------------------------------------------------------
	
	if [[ "$$GOOS" == "linux" ]]; then
	    ICON_DIR="$$HOME/.local/share/icons"
	    DESKTOP_DIR="$$HOME/.local/share/applications"
	
	    mkdir -p "$$ICON_DIR" "$$DESKTOP_DIR"
	    cp "$$SCRIPT_DIR/assets/logo.svg" "$$ICON_DIR/libro.svg"
	
	    # Keep the backend and Electron helpers alive when the app launcher restarts.
	    DESKTOP_EXEC="\"$$LIBRO_DIR/libro\""
	    if command -v systemd-run >/dev/null && systemctl --user is-active --quiet default.target; then
	        DESKTOP_EXEC="systemd-run --user --scope --collect --quiet $$DESKTOP_EXEC"
	    fi

	    cat >"$$DESKTOP_DIR/libro.desktop" <<DESKTOP
	[Desktop Entry]
	Name=Libro
	Comment=Application Manager
	Exec=$$DESKTOP_EXEC
	Icon=$$ICON_DIR/libro.svg
	Type=Application
	Categories=Utility;Development;
	DESKTOP
	
	    # Set custom icon on the binary itself (shown in file managers)
	    gio set "$$LIBRO_DIR/$$BIN_NAME" metadata::custom-icon "file://$$ICON_DIR/libro.svg" 2>/dev/null || true
	
	    ok "Desktop entry installed"
	fi
	
	# ---------------------------------------------------------------------------
	# Check PATH
	# ---------------------------------------------------------------------------
	
	if ! echo "$$PATH" | tr ':' '\n' | grep -qx "$$INSTALL_DIR"; then
	    echo ""
	    echo "  NOTE: $$INSTALL_DIR is not in your PATH."
	    echo "  Add this to your shell profile:"
	    echo ""
	    echo "    export PATH=\"\$$HOME/.local/bin:\$$PATH\""
	    echo ""
	fi
	
	echo ""
	echo "  Installed source build. Run 'libro' to start,"
	echo "  or 'libro --no-desktop' for server-only mode."
	echo ""

release-dry-run: RELEASE_ARGS := --dry-run
release-dry-run: release ## Preview a release without changes

release: ## Bump, build, tag, push, and publish a release
	@set -- $(RELEASE_ARGS)
	set -euo pipefail
	
	# ---------------------------------------------------------------------------
	# release — Version-bump, build self-contained cross-platform release binaries,
	#           tag, push, and create a GitHub Release with attached assets
	#
	# Usage:
	#   make release           # bump patch, build, tag, push, create release
	#   make release-dry-run   # preview without making any changes
	# ---------------------------------------------------------------------------
	
	SCRIPT_DIR="$(CURDIR)"
	VERSION_FILE="$$SCRIPT_DIR/VERSION"
	MODULE="libro"
	BUILD_DIR="$$SCRIPT_DIR/dist"
	BUNDLE_DIR="$$SCRIPT_DIR/internal/desktopbundledata"
	BUNDLE_APP_DIR="$$BUNDLE_DIR/electron-app"
	BUNDLE_RUNTIME_DIR="$$BUNDLE_DIR/electron-runtime"
	DOWNLOAD_CACHE_DIR="$$SCRIPT_DIR/.cache/electron"
	
	DRY_RUN=false
	
	# Parse flags
	for arg in "$$@"; do
	    case "$$arg" in
	    "--dry-run")
	            DRY_RUN=true
	            ;;
	        *)
	            echo "Unknown flag: $$arg"
	            echo "Usage: make release or make release-dry-run"
	            exit 1
	            ;;
	    esac
	done
	
	# ---------------------------------------------------------------------------
	# Helpers
	# ---------------------------------------------------------------------------
	
	info()  { echo -e "\033[1;34m==>\033[0m $$*"; }
	ok()    { echo -e "\033[1;32m==>\033[0m $$*"; }
	warn()  { echo -e "\033[1;33m==>\033[0m $$*"; }
	err()   { echo -e "\033[1;31m==>\033[0m $$*" >&2; }
	
	electron_version() {
	    sed -nE 's/.*"electron"[[:space:]]*:[[:space:]]*"[^0-9]*([0-9][^"]*)".*/\1/p' "$$SCRIPT_DIR/package.json" | head -n 1
	}
	
	electron_platform_id() {
	    local os="$$1"
	    local arch="$$2"
	    case "$${os}/$${arch}" in
	        linux/amd64) echo "linux-x64" ;;
	        linux/arm64) echo "linux-arm64" ;;
	        darwin/amd64) echo "darwin-x64" ;;
	        darwin/arm64) echo "darwin-arm64" ;;
	        windows/amd64) echo "win32-x64" ;;
	        *)
	            err "Unsupported Electron platform: $${os}/$${arch}"
	            exit 1
	            ;;
	    esac
	}
	
	reset_embedded_bundle_dirs() {
	    rm -rf "$$BUNDLE_APP_DIR" "$$BUNDLE_RUNTIME_DIR"
	    mkdir -p "$$BUNDLE_APP_DIR/electron" "$$BUNDLE_APP_DIR/winres" "$$BUNDLE_RUNTIME_DIR"
	    printf '%s\n' "Generated release-time desktop app files live here." > "$$BUNDLE_APP_DIR/README.md"
	    printf '%s\n' "Generated release-time Electron runtime zips live here." > "$$BUNDLE_RUNTIME_DIR/README.md"
	}
	
	stage_embedded_app_files() {
	    mkdir -p "$$BUNDLE_APP_DIR/electron" "$$BUNDLE_APP_DIR/winres"
	    rm -f "$$BUNDLE_APP_DIR/README.md"
	    cp "$$SCRIPT_DIR/package.json" "$$BUNDLE_APP_DIR/package.json"
	    cp "$$SCRIPT_DIR/package-lock.json" "$$BUNDLE_APP_DIR/package-lock.json"
	    cp "$$SCRIPT_DIR/electron/main.js" "$$BUNDLE_APP_DIR/electron/main.js"
	    cp "$$SCRIPT_DIR/electron/linux-theme.js" "$$BUNDLE_APP_DIR/electron/linux-theme.js"
	    cp "$$SCRIPT_DIR/electron/agent-control.js" "$$BUNDLE_APP_DIR/electron/agent-control.js"
	    cp "$$SCRIPT_DIR/electron/page-area.js" "$$BUNDLE_APP_DIR/electron/page-area.js"
	    cp "$$SCRIPT_DIR/electron/preload.js" "$$BUNDLE_APP_DIR/electron/preload.js"
	    cp "$$SCRIPT_DIR/electron/webview-preload.js" "$$BUNDLE_APP_DIR/electron/webview-preload.js"
	    cp "$$SCRIPT_DIR/winres/icon.png" "$$BUNDLE_APP_DIR/winres/icon.png"
	}
	
	cache_electron_zip() {
	    local zip_name="$$1"
	    local url="$$2"
	    local cached_zip="$$DOWNLOAD_CACHE_DIR/$$zip_name"
	
	    mkdir -p "$$DOWNLOAD_CACHE_DIR"
	    if [[ -s "$$cached_zip" ]]; then
	        info "Using cached Electron runtime $$zip_name" >&2
	        printf '%s\n' "$$cached_zip"
	        return 0
	    fi
	
	    info "Downloading Electron runtime $$zip_name..." >&2
	    curl -L "$$url" -o "$$cached_zip"
	    printf '%s\n' "$$cached_zip"
	}
	
	prepare_embedded_runtime() {
	    local os="$$1"
	    local arch="$$2"
	    local platform_id
	    local zip_name
	    local url
	    local cached_zip
	
	    platform_id="$$(electron_platform_id "$$os" "$$arch")"
	    zip_name="electron-v$${ELECTRON_VERSION}-$${platform_id}.zip"
	    url="https://github.com/electron/electron/releases/download/v$${ELECTRON_VERSION}/$${zip_name}"
	
	    reset_embedded_bundle_dirs
	    stage_embedded_app_files
	
	    cached_zip="$$(cache_electron_zip "$$zip_name" "$$url")"
	    cp "$$cached_zip" "$$BUNDLE_RUNTIME_DIR/$$zip_name"
	    ok "Embedded desktop payload staged for $${os}/$${arch}"
	}
	
	cleanup_generated_bundle() {
	    reset_embedded_bundle_dirs
	    rm -f "$$SCRIPT_DIR/"*.syso
	}
	
	cleanup_old_releases() {
	    local current_tag="$$1"
	    local old_tag
	    local deleted=0
	
	    info "Removing older GitHub releases and matching remote tags..."
	
	    while IFS=$$'\t' read -r old_tag _; do
	        [[ -z "$$old_tag" || "$$old_tag" == "$$current_tag" ]] && continue
	
	        info "Deleting $${old_tag}..."
	        gh release delete "$$old_tag" --yes --cleanup-tag
	        deleted=$$((deleted + 1))
	    done < <(gh release list --limit 1000)
	
	    if [[ "$$deleted" -eq 0 ]]; then
	        ok "No older releases found"
	    else
	        ok "Removed $${deleted} older release(s)"
	    fi
	}
	
	# ---------------------------------------------------------------------------
	# Pre-flight checks
	# ---------------------------------------------------------------------------
	
	if [[ ! -f "$$VERSION_FILE" ]]; then
	    err "VERSION file not found at $$VERSION_FILE"
	    exit 1
	fi
	
	if ! command -v gh &>/dev/null; then
	    err "GitHub CLI (gh) is required. Install: https://cli.github.com"
	    exit 1
	fi
	
	if ! command -v curl &>/dev/null; then
	    err "curl is required to download Electron runtimes"
	    exit 1
	fi
	
	if [[ ! -f "$$SCRIPT_DIR/package.json" ]]; then
	    err "package.json not found at $$SCRIPT_DIR/package.json"
	    exit 1
	fi
	
	ELECTRON_VERSION="$$(electron_version)"
	if [[ -z "$$ELECTRON_VERSION" ]]; then
	    err "Unable to determine Electron version from package.json"
	    exit 1
	fi
	
	trap cleanup_generated_bundle EXIT
	
	# ---------------------------------------------------------------------------
	# Read current version and auto-bump patch
	# ---------------------------------------------------------------------------
	
	CURRENT_VERSION=$$(cat "$$VERSION_FILE" | tr -d '[:space:]')
	info "Current version: $$CURRENT_VERSION"
	info "Electron version: $$ELECTRON_VERSION"
	
	IFS='.' read -r MAJOR MINOR PATCH <<< "$$CURRENT_VERSION"
	PATCH=$$((PATCH + 1))
	NEW_VERSION="$${MAJOR}.$${MINOR}.$${PATCH}"
	TAG="v$${NEW_VERSION}"
	
	info "New version:     $$NEW_VERSION"
	
	# ---------------------------------------------------------------------------
	# Build matrix
	# ---------------------------------------------------------------------------
	
	PLATFORMS=(
	    "linux/amd64"
	    "linux/arm64"
	    "darwin/amd64"
	    "darwin/arm64"
	    "windows/amd64"
	)
	
	LDFLAGS="-X $${MODULE}/internal/version.Version=$${NEW_VERSION}"
	
	# ---------------------------------------------------------------------------
	# Dry-run mode
	# ---------------------------------------------------------------------------
	
	if [[ "$$DRY_RUN" == true ]]; then
	    warn "DRY RUN — no changes will be made"
	    echo ""
	    echo "  Version bump:   $$CURRENT_VERSION → $$NEW_VERSION"
	    echo "  Electron:       $$ELECTRON_VERSION"
	    echo "  VERSION file:   $$VERSION_FILE"
	    echo ""
	    echo "  Builds:"
	    for platform in "$${PLATFORMS[@]}"; do
	        OS="$${platform%/*}"
	        ARCH="$${platform#*/}"
	        EXT=""
	        [[ "$$OS" == "windows" ]] && EXT=".exe"
	        echo "    $${MODULE}-$${OS}-$${ARCH}$${EXT} (self-contained desktop runtime)"
	    done
	    echo ""
	    echo "  Git commit:     git add VERSION && git commit -m \"$$TAG\""
	    echo "  Git tag:        git tag $$TAG"
	    echo "  Git push:       git push && git push --tags"
	    echo ""
	    echo "  GitHub release: gh release create $$TAG --generate-notes <binaries...>"
	    echo "  Cleanup:        delete older GitHub releases and matching remote tags"
	    echo ""
	    exit 0
	fi
	
	# ---------------------------------------------------------------------------
	# Bump version (first real action)
	# ---------------------------------------------------------------------------
	
	echo "$$NEW_VERSION" > "$$VERSION_FILE"
	ok "VERSION file updated to $$NEW_VERSION"
	
	# ---------------------------------------------------------------------------
	# Cleanup & tidy
	# ---------------------------------------------------------------------------
	
	info "Cleaning up..."
	rm -f "$$SCRIPT_DIR/libro"
	go mod tidy
	ok "Clean"
	
	# ---------------------------------------------------------------------------
	# Build all release targets. Each binary embeds its own Electron runtime zip.
	# ---------------------------------------------------------------------------
	
	rm -rf "$$BUILD_DIR"
	mkdir -p "$$BUILD_DIR"
	
	for platform in "$${PLATFORMS[@]}"; do
	    OS="$${platform%/*}"
	    ARCH="$${platform#*/}"
	    EXT=""
	    [[ "$$OS" == "windows" ]] && EXT=".exe"
	
	    OUTPUT="$${BUILD_DIR}/$${MODULE}-$${OS}-$${ARCH}$${EXT}"
	    info "Building $${OS}/$${ARCH}..."
	
	    prepare_embedded_runtime "$$OS" "$$ARCH"
	
	    # Embed Windows resources if building for Windows and go-winres is available
	    if [[ "$$OS" == "windows" ]] && command -v go-winres &>/dev/null; then
	        (cd "$$SCRIPT_DIR" && go-winres make --product-version="$${NEW_VERSION}" --file-version="$${NEW_VERSION}")
	    fi
	
	    CGO_ENABLED=0 GOOS="$$OS" GOARCH="$$ARCH" \
	        go build -ldflags "$$LDFLAGS" -o "$$OUTPUT" .
	
	    # Cleanup .syso files after Windows build
	    [[ "$$OS" == "windows" ]] && rm -f "$$SCRIPT_DIR/"*.syso
	done
	
	reset_embedded_bundle_dirs
	ok "All self-contained binaries built in $$BUILD_DIR"
	
	# ---------------------------------------------------------------------------
	# Git commit, tag, and push
	# ---------------------------------------------------------------------------
	
	info "Committing version bump..."
	git add "$$VERSION_FILE"
	git commit -m "$${TAG}"
	git tag "$${TAG}"
	ok "Committed and tagged $${TAG}"
	
	info "Pushing to remote..."
	git push
	git push --tags
	ok "Pushed"
	
	# ---------------------------------------------------------------------------
	# Create GitHub Release
	# ---------------------------------------------------------------------------
	
	info "Creating GitHub release $${TAG}..."
	
	ASSETS=()
	for f in "$$BUILD_DIR"/*; do
	    ASSETS+=("$$f")
	done
	
	gh release create "$${TAG}" "$${ASSETS[@]}" \
	    "--title" "$${TAG}" \
	    "--generate-notes"
	
	ok "GitHub release created: $${TAG}"
	
	cleanup_old_releases "$${TAG}"
	
	# ---------------------------------------------------------------------------
	# Cleanup
	# ---------------------------------------------------------------------------
	
	rm -rf "$$BUILD_DIR"
	ok "Build artifacts cleaned up"
	
	echo ""
	echo "============================================"
	echo "  Release complete"
	echo "  $$CURRENT_VERSION → $$NEW_VERSION"
	echo "  https://github.com/michalCapo/libro/releases/tag/$${TAG}"
	echo "============================================"
	echo ""
