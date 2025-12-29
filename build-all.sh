#!/bin/bash

# Unified build script for ThinLine Radio
# This script builds all platforms and architectures
# Usage: ./build-all.sh [platform]
#   If platform is specified, only build that platform
#   Platforms: linux, freebsd, openbsd, netbsd, solaris, darwin, windows, all
#   If no platform specified, build all platforms

set -e  # Exit on any error

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Get version from server/version.go
VERSION=$(grep -E '^const Version =' server/version.go | awk -F'"' '{print $2}')
if [ -z "$VERSION" ]; then
    VERSION="7.0.0"
fi

# Parse command line arguments
BUILD_PLATFORM="${1:-all}"

echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}ThinLine Radio Unified Build Script${NC}"
echo -e "${GREEN}Version: ${VERSION}${NC}"
echo -e "${GREEN}========================================${NC}"
echo ""

# Check prerequisites
echo -e "${YELLOW}Checking prerequisites...${NC}"

# Check Node.js
if ! command -v node &> /dev/null; then
    echo -e "${RED}ERROR: Node.js is not installed. Please install Node.js 16+ from https://nodejs.org/${NC}"
    exit 1
fi
NODE_VERSION=$(node -v)
echo "  ✓ Node.js: $NODE_VERSION"

# Check npm
if ! command -v npm &> /dev/null; then
    echo -e "${RED}ERROR: npm is not installed${NC}"
    exit 1
fi
NPM_VERSION=$(npm -v)
echo "  ✓ npm: $NPM_VERSION"

# Check Go
if ! command -v go &> /dev/null; then
    echo -e "${RED}ERROR: Go is not installed. Please install Go 1.23+ from https://go.dev/dl/${NC}"
    exit 1
fi
GO_VERSION=$(go version | awk '{print $3}')
echo "  ✓ Go: $GO_VERSION"

echo ""

# Build the Angular client (only once, shared across all platforms)
CLIENT_BUILT=false
if [ ! -d "server/webapp" ] || [ -z "$(ls -A server/webapp)" ]; then
    echo -e "${YELLOW}Building Angular client...${NC}"
    cd client
    
    # Install dependencies if node_modules doesn't exist
    if [ ! -d "node_modules" ]; then
        echo "  Installing npm dependencies..."
        npm install
    fi
    
    # Build the client
    echo "  Building production bundle..."
    npm run build
    
    if [ ! -d "../server/webapp" ] || [ -z "$(ls -A ../server/webapp)" ]; then
        echo -e "${RED}ERROR: Client build failed or webapp directory is empty${NC}"
        exit 1
    fi
    
    echo -e "${GREEN}  ✓ Client built successfully${NC}"
    cd ..
    CLIENT_BUILT=true
else
    echo -e "${GREEN}✓ Client already built (using existing webapp)${NC}"
    CLIENT_BUILT=true
fi

echo ""

# Function to build a single platform/architecture
build_platform_arch() {
    local PLATFORM=$1
    local ARCH=$2
    local ARCH_NAME=$3
    
    # Capitalize platform name for display
    local PLATFORM_DISPLAY=$(echo "$PLATFORM" | awk '{print toupper(substr($0,1,1)) tolower(substr($0,2))}')
    
    echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${BLUE}Building: ${PLATFORM} ${ARCH_NAME} (${ARCH})${NC}"
    echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    
    cd server
    
    # Set build variables
    export GOOS=$PLATFORM
    export GOARCH=$ARCH
    export CGO_ENABLED=0  # Disable CGO for static binary
    
    # Determine binary extension
    if [ "$PLATFORM" = "windows" ]; then
        BINARY_EXT=".exe"
        ARCHIVE_EXT=".zip"
    else
        BINARY_EXT=""
        ARCHIVE_EXT=".tar.gz"
    fi
    
    # Build the binary
    BINARY_NAME="thinline-radio-${PLATFORM}-${ARCH}-v${VERSION}${BINARY_EXT}"
    echo "  Building binary: $BINARY_NAME"
    
    go build -ldflags="-s -w" -o "../$BINARY_NAME" .
    
    if [ ! -f "../$BINARY_NAME" ]; then
        echo -e "${RED}ERROR: Server build failed for ${PLATFORM} ${ARCH}${NC}"
        cd ..
        return 1
    fi
    
    echo -e "${GREEN}  ✓ Server built successfully${NC}"
    cd ..
    
    # Create distribution directory
    DIST_DIR="dist-${PLATFORM}-${ARCH}"
    echo "  Creating distribution package..."
    
    # Clean and create dist directory
    rm -rf "$DIST_DIR"
    mkdir -p "$DIST_DIR"
    
    # Copy binary with appropriate name
    if [ "$PLATFORM" = "windows" ]; then
        cp "$BINARY_NAME" "$DIST_DIR/thinline-radio.exe"
    else
        cp "$BINARY_NAME" "$DIST_DIR/thinline-radio"
        chmod +x "$DIST_DIR/thinline-radio"
    fi
    
    # Copy LICENSE file (required for GPL v3 compliance)
    cp LICENSE "$DIST_DIR/"
    
    # Copy setup and administration guide
    if [ -f "docs/setup-and-administration.md" ]; then
        cp docs/setup-and-administration.md "$DIST_DIR/SETUP.md"
    fi
    
    # Copy examples directory (FDMA table, keywords, tone samples)
    if [ -d "docs/examples" ]; then
        mkdir -p "$DIST_DIR/examples"
        cp docs/examples/*.csv "$DIST_DIR/examples/" 2>/dev/null || true
        cp docs/examples/*.json "$DIST_DIR/examples/" 2>/dev/null || true
        cp docs/examples/*.PNG "$DIST_DIR/examples/" 2>/dev/null || true
        cp docs/examples/*.png "$DIST_DIR/examples/" 2>/dev/null || true
    fi
    
    # Create config template
    cat > "$DIST_DIR/thinline-radio.ini.template" << 'EOF'
# ThinLine Radio Configuration
# Copy this file to thinline-radio.ini and update with your settings

db_type = postgresql
db_host = localhost
db_port = 5432
db_name = thinline_radio
db_user = your_db_user
db_pass = your_db_password

# Server settings
listen = 0.0.0.0:3000
ssl_listen = 0.0.0.0:3443

# Optional SSL settings (uncomment to enable)
# ssl_cert_file = /path/to/cert.pem
# ssl_key_file = /path/to/key.pem
# ssl_auto_cert = yourdomain.com

# Base directory for data storage
# base_dir = /var/lib/thinline-radio
EOF
    
    # Create README for deployment
    if [ "$PLATFORM" = "windows" ]; then
        cat > "$DIST_DIR/README.md" << EOF
# ThinLine Radio - Windows Deployment (${ARCH_NAME})

## Version ${VERSION}

This distribution is built for Windows ${ARCH_NAME} (${ARCH}) and should work on Windows 10/11 and Windows Server 2016+.

## Quick Start

1. **Extract the files** to a directory (e.g., \`C:\\Program Files\\ThinLine Radio\`)

2. **Configure the server:**
   - Copy \`thinline-radio.ini.template\` to \`thinline-radio.ini\`
   - Edit \`thinline-radio.ini\` with your database and server settings

3. **Set up the database:**
   - Ensure PostgreSQL is installed and running
   - Create a database for ThinLine Radio
   - Update the database credentials in \`thinline-radio.ini\`

4. **Run the server:**
   \`\`\`cmd
   thinline-radio.exe -config thinline-radio.ini
   \`\`\`

5. **Access the admin dashboard:**
   - Open your browser and navigate to \`http://localhost:3000/admin\`
   - Default password: \`admin\`
   - **Important**: Change the default password immediately after first login

## Requirements

- Windows 10/11 or Windows Server 2016+
- PostgreSQL 12+
- FFmpeg (for audio processing) - download from https://ffmpeg.org/download.html
- Sufficient disk space for audio files

## Documentation

- **SETUP.md** - Comprehensive setup and administration guide (transcription, system admin, troubleshooting)
- **README.md** - Quick start guide (this file)

For more information, see the project repository: https://github.com/Thinline-Dynamic-Solutions/ThinLineRadio
EOF
    else
        # Unix-like systems (Linux, FreeBSD, OpenBSD, NetBSD, Solaris, macOS)
        cat > "$DIST_DIR/README.md" << EOF
# ThinLine Radio - ${PLATFORM_DISPLAY} Deployment (${ARCH_NAME})

## Version ${VERSION}

This distribution is built for ${PLATFORM_DISPLAY} ${ARCH_NAME} (${ARCH}).

## Quick Start

1. **Make the binary executable:**
   \`\`\`bash
   chmod +x thinline-radio
   \`\`\`

2. **Configure the server:**
   \`\`\`bash
   cp thinline-radio.ini.template thinline-radio.ini
   nano thinline-radio.ini  # Edit with your database and server settings
   \`\`\`

3. **Set up the database:**
   - Ensure PostgreSQL is installed and running
   - Create a database for ThinLine Radio
   - Update the database credentials in \`thinline-radio.ini\`

4. **Run the server:**
   \`\`\`bash
   ./thinline-radio -config thinline-radio.ini
   \`\`\`

5. **Access the admin dashboard:**
   - Open your browser and navigate to \`http://localhost:3000/admin\`
   - Default password: \`admin\`
   - **Important**: Change the default password immediately after first login

## Requirements

- ${PLATFORM_DISPLAY} with PostgreSQL 12+
- FFmpeg (for audio processing)
- Sufficient disk space for audio files

## Documentation

- **SETUP.md** - Comprehensive setup and administration guide (transcription, system admin, troubleshooting)
- **README.md** - Quick start guide (this file)

For more information, see the project repository: https://github.com/Thinline-Dynamic-Solutions/ThinLineRadio
EOF
    fi
    
    # Create archive
    ARCHIVE_NAME="thinline-radio-${PLATFORM}-${ARCH}-v${VERSION}${ARCHIVE_EXT}"
    echo "  Creating archive: $ARCHIVE_NAME"
    
    if [ "$PLATFORM" = "windows" ]; then
        cd "$DIST_DIR"
        zip -r "../$ARCHIVE_NAME" . > /dev/null
        cd ..
    else
        tar -czf "$ARCHIVE_NAME" -C "$DIST_DIR" .
    fi
    
    # Clean up temporary binary
    rm -f "$BINARY_NAME"
    
    echo -e "${GREEN}  ✓ Package created: $ARCHIVE_NAME${NC}"
    echo ""
    
    return 0
}

# Build Linux architectures
build_linux() {
    echo -e "${YELLOW}════════════════════════════════════════${NC}"
    echo -e "${YELLOW}Building Linux Packages${NC}"
    echo -e "${YELLOW}════════════════════════════════════════${NC}"
    echo ""
    
    # Core architectures (most common) - 32-bit support removed
    local ARCHITECTURES=("amd64" "arm64" "ppc64le" "s390x" "riscv64")
    local ARCH_NAMES=("64-bit" "ARM64" "PowerPC64LE" "IBM Z" "RISC-V 64")
    
    for i in "${!ARCHITECTURES[@]}"; do
        build_platform_arch "linux" "${ARCHITECTURES[$i]}" "${ARCH_NAMES[$i]}"
    done
}

# Build FreeBSD
build_freebsd() {
    echo -e "${YELLOW}════════════════════════════════════════${NC}"
    echo -e "${YELLOW}Building FreeBSD Packages${NC}"
    echo -e "${YELLOW}════════════════════════════════════════${NC}"
    echo ""
    
    local ARCHITECTURES=("amd64" "arm64")
    local ARCH_NAMES=("64-bit" "ARM64")
    
    for i in "${!ARCHITECTURES[@]}"; do
        build_platform_arch "freebsd" "${ARCHITECTURES[$i]}" "${ARCH_NAMES[$i]}"
    done
}

# Build OpenBSD
build_openbsd() {
    echo -e "${YELLOW}════════════════════════════════════════${NC}"
    echo -e "${YELLOW}Building OpenBSD Packages${NC}"
    echo -e "${YELLOW}════════════════════════════════════════${NC}"
    echo ""
    
    local ARCHITECTURES=("amd64" "arm64")
    local ARCH_NAMES=("64-bit" "ARM64")
    
    for i in "${!ARCHITECTURES[@]}"; do
        build_platform_arch "openbsd" "${ARCHITECTURES[$i]}" "${ARCH_NAMES[$i]}"
    done
}

# Build NetBSD
build_netbsd() {
    echo -e "${YELLOW}════════════════════════════════════════${NC}"
    echo -e "${YELLOW}Building NetBSD Packages${NC}"
    echo -e "${YELLOW}════════════════════════════════════════${NC}"
    echo ""
    
    local ARCHITECTURES=("amd64" "arm64")
    local ARCH_NAMES=("64-bit" "ARM64")
    
    for i in "${!ARCHITECTURES[@]}"; do
        build_platform_arch "netbsd" "${ARCHITECTURES[$i]}" "${ARCH_NAMES[$i]}"
    done
}

# Build Solaris/Illumos
build_solaris() {
    echo -e "${YELLOW}════════════════════════════════════════${NC}"
    echo -e "${YELLOW}Building Solaris/Illumos Packages${NC}"
    echo -e "${YELLOW}════════════════════════════════════════${NC}"
    echo ""
    
    build_platform_arch "solaris" "amd64" "64-bit"
}

# Build macOS (Darwin)
build_darwin() {
    echo -e "${YELLOW}════════════════════════════════════════${NC}"
    echo -e "${YELLOW}Building macOS Packages${NC}"
    echo -e "${YELLOW}════════════════════════════════════════${NC}"
    echo ""
    
    local ARCHITECTURES=("amd64" "arm64")
    local ARCH_NAMES=("Intel" "Apple Silicon")
    
    for i in "${!ARCHITECTURES[@]}"; do
        build_platform_arch "darwin" "${ARCHITECTURES[$i]}" "${ARCH_NAMES[$i]}"
    done
    
    # Create universal binary if on macOS and lipo is available
    if [[ "$OSTYPE" == "darwin"* ]] && command -v lipo &> /dev/null && [ -d "dist-darwin-amd64" ] && [ -d "dist-darwin-arm64" ]; then
        echo -e "${YELLOW}Creating universal binary...${NC}"
        DIST_DIR_UNIVERSAL="dist-darwin-universal"
        rm -rf "$DIST_DIR_UNIVERSAL"
        mkdir -p "$DIST_DIR_UNIVERSAL"
        
        lipo -create \
            "dist-darwin-amd64/thinline-radio" \
            "dist-darwin-arm64/thinline-radio" \
            -output "$DIST_DIR_UNIVERSAL/thinline-radio"
        
        chmod +x "$DIST_DIR_UNIVERSAL/thinline-radio"
        
        cp dist-darwin-amd64/thinline-radio.ini.template "$DIST_DIR_UNIVERSAL/"
        
        ARCHIVE_NAME_UNIVERSAL="thinline-radio-darwin-universal-v${VERSION}.tar.gz"
        tar -czf "$ARCHIVE_NAME_UNIVERSAL" -C "$DIST_DIR_UNIVERSAL" .
        echo -e "${GREEN}  ✓ Universal binary created: $ARCHIVE_NAME_UNIVERSAL${NC}"
        echo ""
    fi
}

# Build Windows
build_windows() {
    echo -e "${YELLOW}════════════════════════════════════════${NC}"
    echo -e "${YELLOW}Building Windows Packages${NC}"
    echo -e "${YELLOW}════════════════════════════════════════${NC}"
    echo ""
    
    local ARCHITECTURES=("amd64" "arm64")
    local ARCH_NAMES=("64-bit" "ARM64")
    
    for i in "${!ARCHITECTURES[@]}"; do
        build_platform_arch "windows" "${ARCHITECTURES[$i]}" "${ARCH_NAMES[$i]}"
    done
}

# Main build logic
case "$BUILD_PLATFORM" in
    linux)
        build_linux
        ;;
    freebsd)
        build_freebsd
        ;;
    openbsd)
        build_openbsd
        ;;
    netbsd)
        build_netbsd
        ;;
    solaris)
        build_solaris
        ;;
    darwin|macos)
        build_darwin
        ;;
    windows)
        build_windows
        ;;
    all)
        build_linux
        build_freebsd
        build_openbsd
        build_netbsd
        build_solaris
        build_darwin
        build_windows
        ;;
    *)
        echo -e "${RED}ERROR: Unknown platform: $BUILD_PLATFORM${NC}"
        echo "Usage: $0 [platform]"
        echo "Platforms: linux, freebsd, openbsd, netbsd, solaris, darwin, windows, all"
        exit 1
        ;;
esac

# Summary
echo ""
echo -e "${GREEN}════════════════════════════════════════${NC}"
echo -e "${GREEN}Build Complete!${NC}"
echo -e "${GREEN}════════════════════════════════════════${NC}"
echo ""
echo "Built packages:"
ls -1 thinline-radio-*-v${VERSION}.* 2>/dev/null | while read file; do
    echo "  ✓ $file"
done
echo ""
echo "Distribution directories:"
ls -d dist-* 2>/dev/null | while read dir; do
    echo "  ✓ $dir/"
done
echo ""

