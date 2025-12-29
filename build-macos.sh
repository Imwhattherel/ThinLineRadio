#!/bin/bash

# Build script for macOS deployment
# This script builds the ThinLine Radio server for macOS (Intel and Apple Silicon)

set -e  # Exit on any error

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Get version from server/version.go
VERSION=$(grep -E '^const Version =' server/version.go | awk -F'"' '{print $2}')
if [ -z "$VERSION" ]; then
    VERSION="7.0.0"
fi

echo -e "${GREEN}Building ThinLine Radio v${VERSION} for macOS${NC}"
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

# Build the Angular client
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

# Create macOS icon if source exists
echo ""
echo -e "${YELLOW}Creating macOS icon...${NC}"
if [ -f "ThinlineRadio-Mobile/assets/icons/icon.png" ]; then
    # Create temporary iconset directory
    ICONSET_DIR="server/ThinLineRadio.iconset"
    mkdir -p "$ICONSET_DIR"
    
    # Check if sips (built-in macOS tool) is available
    if command -v sips &> /dev/null; then
        echo "  Using sips to create icon sizes..."
        # Create all required icon sizes for .icns
        sips -z 16 16     ThinlineRadio-Mobile/assets/icons/icon.png --out "$ICONSET_DIR/icon_16x16.png" > /dev/null 2>&1
        sips -z 32 32     ThinlineRadio-Mobile/assets/icons/icon.png --out "$ICONSET_DIR/icon_16x16@2x.png" > /dev/null 2>&1
        sips -z 32 32     ThinlineRadio-Mobile/assets/icons/icon.png --out "$ICONSET_DIR/icon_32x32.png" > /dev/null 2>&1
        sips -z 64 64     ThinlineRadio-Mobile/assets/icons/icon.png --out "$ICONSET_DIR/icon_32x32@2x.png" > /dev/null 2>&1
        sips -z 128 128   ThinlineRadio-Mobile/assets/icons/icon.png --out "$ICONSET_DIR/icon_128x128.png" > /dev/null 2>&1
        sips -z 256 256   ThinlineRadio-Mobile/assets/icons/icon.png --out "$ICONSET_DIR/icon_128x128@2x.png" > /dev/null 2>&1
        sips -z 256 256   ThinlineRadio-Mobile/assets/icons/icon.png --out "$ICONSET_DIR/icon_256x256.png" > /dev/null 2>&1
        sips -z 512 512   ThinlineRadio-Mobile/assets/icons/icon.png --out "$ICONSET_DIR/icon_256x256@2x.png" > /dev/null 2>&1
        sips -z 512 512   ThinlineRadio-Mobile/assets/icons/icon.png --out "$ICONSET_DIR/icon_512x512.png" > /dev/null 2>&1
        sips -z 1024 1024 ThinlineRadio-Mobile/assets/icons/icon.png --out "$ICONSET_DIR/icon_512x512@2x.png" > /dev/null 2>&1
        
        # Create .icns file
        if command -v iconutil &> /dev/null; then
            iconutil -c icns "$ICONSET_DIR" -o "server/icon.icns"
            echo -e "${GREEN}  ✓ macOS icon created (icon.icns)${NC}"
        else
            echo -e "${YELLOW}  ⚠ iconutil not found, skipping .icns creation${NC}"
        fi
        
        # Clean up iconset directory
        rm -rf "$ICONSET_DIR"
    else
        echo -e "${YELLOW}  ⚠ sips not available, skipping icon creation${NC}"
    fi
else
    echo -e "${YELLOW}  ⚠ Source icon not found, building without icon${NC}"
fi

# Build for both architectures
ARCHITECTURES=("amd64" "arm64")
ARCH_NAMES=("Intel" "Apple Silicon")

for i in "${!ARCHITECTURES[@]}"; do
    ARCH="${ARCHITECTURES[$i]}"
    ARCH_NAME="${ARCH_NAMES[$i]}"
    
    echo ""
    echo -e "${YELLOW}Building Go server for macOS ${ARCH_NAME} (${ARCH})...${NC}"
    
    cd server
    
    # Set build variables
    export GOOS=darwin
    export GOARCH=$ARCH
    export CGO_ENABLED=0  # Disable CGO for static binary
    
    # Build the binary
    BINARY_NAME="thinline-radio-darwin-${ARCH}-v${VERSION}"
    echo "  Building binary: $BINARY_NAME"
    go build -ldflags="-s -w" -o "../$BINARY_NAME" .
    
    if [ ! -f "../$BINARY_NAME" ]; then
        echo -e "${RED}ERROR: Server build failed for ${ARCH}${NC}"
        exit 1
    fi
    
    echo -e "${GREEN}  ✓ Server built successfully for ${ARCH_NAME}${NC}"
    cd ..
    
    # Create distribution directory for this architecture
    DIST_DIR="dist-macos-${ARCH}"
    echo "  Creating distribution package..."
    
    # Clean and create dist directory
    rm -rf "$DIST_DIR"
    mkdir -p "$DIST_DIR"
    
    # Copy binary
    cp "$BINARY_NAME" "$DIST_DIR/thinline-radio"
    chmod +x "$DIST_DIR/thinline-radio"
    
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
    
    # Copy icon if it exists
    if [ -f "server/icon.icns" ]; then
        cp "server/icon.icns" "$DIST_DIR/"
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
# base_dir = /usr/local/var/thinline-radio
EOF
    
    # Create README
    cat > "$DIST_DIR/README.md" << EOF
# ThinLine Radio - macOS Deployment (${ARCH_NAME})

## Version ${VERSION}

This distribution is built for macOS ${ARCH_NAME} (${ARCH}).

## Quick Start

1. **Make the binary executable:**
   \`\`\`bash
   chmod +x thinline-radio
   \`\`\`

2. **Set up PostgreSQL:**
   
   You have two options for PostgreSQL setup:
   
   ### Option A: Interactive Setup Wizard (Recommended)
   
   Simply run the server and it will guide you through setup:
   \`\`\`bash
   ./thinline-radio
   \`\`\`
   
   The wizard will:
   - Detect if PostgreSQL is installed locally
   - Offer to guide you through local PostgreSQL installation
   - Or help you connect to a remote PostgreSQL server
   - Automatically create the database and user (local)
   - Generate the configuration file
   
   **For Local PostgreSQL:**
   - The wizard will prompt for your PostgreSQL superuser password
   - It will create the database and user automatically
   - Configuration file will be created for you
   
   **For Remote PostgreSQL:**
   - The wizard will prompt for your remote server details
   - You'll need existing database credentials
   - Make sure the remote PostgreSQL server allows connections
   
   ### Option B: Manual Setup
   
   Install PostgreSQL and configure manually:
   
   - **Install PostgreSQL via Homebrew:**
     \`\`\`bash
     brew install postgresql@16
     brew services start postgresql@16
     \`\`\`
   
   - **Create Database and User:**
     \`\`\`bash
     psql postgres
     \`\`\`
     
     In PostgreSQL prompt:
     \`\`\`sql
     CREATE DATABASE thinline_radio;
     CREATE USER thinline_user WITH PASSWORD 'your_secure_password';
     GRANT ALL PRIVILEGES ON DATABASE thinline_radio TO thinline_user;
     \\q
     \`\`\`

3. **Configure the server (if using manual setup):**
   \`\`\`bash
   cp thinline-radio.ini.template thinline-radio.ini
   nano thinline-radio.ini  # Edit with your database and server settings
   \`\`\`

4. **Run the server:**
   \`\`\`bash
   ./thinline-radio -config thinline-radio.ini
   \`\`\`

5. **Access the admin dashboard:**
   - Open your browser and navigate to \`http://localhost:3000/admin\`
   - Default password: \`admin\`
   - **Important**: Change the default password immediately after first login

## Launch Agent Setup (Optional)

To run ThinLine Radio as a background service:

1. **Create launch agent plist:**
   \`\`\`bash
   mkdir -p ~/Library/LaunchAgents
   cat > ~/Library/LaunchAgents/com.thinlineds.radio.plist << 'PLIST'
   <?xml version="1.0" encoding="UTF-8"?>
   <!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
   <plist version="1.0">
   <dict>
       <key>Label</key>
       <string>com.thinlineds.radio</string>
       <key>ProgramArguments</key>
       <array>
           <string>/usr/local/opt/thinline-radio/thinline-radio</string>
           <string>-config</string>
           <string>/usr/local/opt/thinline-radio/thinline-radio.ini</string>
       </array>
       <key>RunAtLoad</key>
       <true/>
       <key>KeepAlive</key>
       <true/>
       <key>StandardOutPath</key>
       <string>/usr/local/var/log/thinline-radio.log</string>
       <key>StandardErrorPath</key>
       <string>/usr/local/var/log/thinline-radio.error.log</string>
       <key>WorkingDirectory</key>
       <string>/usr/local/opt/thinline-radio</string>
   </dict>
   </plist>
   PLIST
   \`\`\`

2. **Update paths in the plist** to match your installation location

3. **Load and start the service:**
   \`\`\`bash
   launchctl load ~/Library/LaunchAgents/com.thinlineds.radio.plist
   launchctl start com.thinlineds.radio
   \`\`\`

4. **Check status:**
   \`\`\`bash
   launchctl list | grep thinlineds
   tail -f /usr/local/var/log/thinline-radio.log
   \`\`\`

## Requirements

- macOS 10.15 Catalina or later
- PostgreSQL 12+ (install via Homebrew: \`brew install postgresql\`)
- FFmpeg (for audio processing: \`brew install ffmpeg\`)
- Sufficient disk space for audio files

## Installation Location

Recommended installation path: \`/usr/local/opt/thinline-radio\`

\`\`\`bash
sudo mkdir -p /usr/local/opt/thinline-radio
sudo cp thinline-radio /usr/local/opt/thinline-radio/
sudo cp thinline-radio.ini.template /usr/local/opt/thinline-radio/
sudo chown -R \$(whoami) /usr/local/opt/thinline-radio
\`\`\`

## Firewall Configuration

If you have the macOS firewall enabled:

1. Go to System Settings → Network → Firewall
2. Click "Options"
3. Add \`thinline-radio\` to allowed applications
4. Or allow specific ports via command line:
   \`\`\`bash
   sudo /usr/libexec/ApplicationFirewall/socketfilterfw --add /usr/local/opt/thinline-radio/thinline-radio
   sudo /usr/libexec/ApplicationFirewall/socketfilterfw --unblockapp /usr/local/opt/thinline-radio/thinline-radio
   \`\`\`

## Logs

When running manually, logs are written to stdout/stderr.
When running as a Launch Agent, check:
- \`/usr/local/var/log/thinline-radio.log\`
- \`/usr/local/var/log/thinline-radio.error.log\`

## Troubleshooting

- **"Port already in use":** Another application is using the configured port. Change the port in \`thinline-radio.ini\`
- **Database connection failed:** Verify PostgreSQL is running (\`brew services list\`) and credentials are correct
- **Permission denied:** Check file/directory permissions or run with appropriate user

## Documentation

- **SETUP.md** - Comprehensive setup and administration guide (transcription, system admin, troubleshooting)
- **README.md** - Quick start guide (this file)

For more information, see the project repository: https://github.com/Thinline-Dynamic-Solutions/ThinLineRadio
EOF

    # Create installation script
    cat > "$DIST_DIR/install.sh" << 'INSTALLEOF'
#!/bin/bash

# Installation script for ThinLine Radio on macOS

set -e

INSTALL_DIR="/usr/local/opt/thinline-radio"
LOG_DIR="/usr/local/var/log"

echo "ThinLine Radio Installation Script"
echo "==================================="
echo ""

# Check if running on macOS
if [[ "$OSTYPE" != "darwin"* ]]; then
    echo "ERROR: This script is for macOS only"
    exit 1
fi

# Create directories
echo "Creating directories..."
sudo mkdir -p "$INSTALL_DIR"
sudo mkdir -p "$LOG_DIR"

# Copy files
echo "Installing files..."
sudo cp thinline-radio "$INSTALL_DIR/"
sudo cp thinline-radio.ini.template "$INSTALL_DIR/"
if [ -f "icon.icns" ]; then
    sudo cp icon.icns "$INSTALL_DIR/"
fi

# Set permissions
echo "Setting permissions..."
sudo chown -R $(whoami) "$INSTALL_DIR"
sudo chmod +x "$INSTALL_DIR/thinline-radio"

echo ""
echo "✓ Installation complete!"
echo ""
echo "Installation directory: $INSTALL_DIR"
echo ""
echo "Next steps:"
echo "  1. Configure: cp $INSTALL_DIR/thinline-radio.ini.template $INSTALL_DIR/thinline-radio.ini"
echo "  2. Edit config: nano $INSTALL_DIR/thinline-radio.ini"
echo "  3. Set up PostgreSQL: brew install postgresql@16"
echo "  4. Run: $INSTALL_DIR/thinline-radio -config $INSTALL_DIR/thinline-radio.ini"
echo ""
echo "To run as a service, see README.md for Launch Agent setup"
echo ""
INSTALLEOF

    chmod +x "$DIST_DIR/install.sh"
    
    # Create archive
    ARCHIVE_NAME="thinline-radio-darwin-${ARCH}-v${VERSION}.tar.gz"
    echo "  Creating archive: $ARCHIVE_NAME"
    tar -czf "$ARCHIVE_NAME" -C "$DIST_DIR" .
    
    # Clean up temporary binary
    rm -f "$BINARY_NAME"
    
    echo -e "${GREEN}  ✓ Package created for ${ARCH_NAME}${NC}"
done

# Create universal binary (optional, if both architectures were built successfully)
echo ""
echo -e "${YELLOW}Creating universal binary...${NC}"
if command -v lipo &> /dev/null; then
    DIST_DIR_UNIVERSAL="dist-macos-universal"
    rm -rf "$DIST_DIR_UNIVERSAL"
    mkdir -p "$DIST_DIR_UNIVERSAL"
    
    # Combine binaries
    lipo -create \
        "dist-macos-amd64/thinline-radio" \
        "dist-macos-arm64/thinline-radio" \
        -output "$DIST_DIR_UNIVERSAL/thinline-radio"
    
    chmod +x "$DIST_DIR_UNIVERSAL/thinline-radio"
    
    # Copy other files from amd64 build (they're identical)
    cp dist-macos-amd64/LICENSE "$DIST_DIR_UNIVERSAL/"
    if [ -f "dist-macos-amd64/SETUP.md" ]; then
        cp dist-macos-amd64/SETUP.md "$DIST_DIR_UNIVERSAL/"
    fi
    if [ -d "dist-macos-amd64/examples" ]; then
        cp -r dist-macos-amd64/examples "$DIST_DIR_UNIVERSAL/"
    fi
    cp dist-macos-amd64/thinline-radio.ini.template "$DIST_DIR_UNIVERSAL/"
    cp dist-macos-amd64/README.md "$DIST_DIR_UNIVERSAL/"
    cp dist-macos-amd64/install.sh "$DIST_DIR_UNIVERSAL/"
    if [ -f "dist-macos-amd64/icon.icns" ]; then
        cp dist-macos-amd64/icon.icns "$DIST_DIR_UNIVERSAL/"
    fi
    
    # Create universal archive
    ARCHIVE_NAME_UNIVERSAL="thinline-radio-darwin-universal-v${VERSION}.tar.gz"
    echo "  Creating universal archive: $ARCHIVE_NAME_UNIVERSAL"
    tar -czf "$ARCHIVE_NAME_UNIVERSAL" -C "$DIST_DIR_UNIVERSAL" .
    
    echo -e "${GREEN}  ✓ Universal binary created (runs on both Intel and Apple Silicon)${NC}"
else
    echo -e "${YELLOW}  ⚠ lipo not available, skipping universal binary${NC}"
fi

echo ""
echo -e "${GREEN}✓ Build complete!${NC}"
echo ""
echo "Distribution packages:"
echo "  - thinline-radio-darwin-amd64-v${VERSION}.tar.gz (Intel Macs)"
echo "  - thinline-radio-darwin-arm64-v${VERSION}.tar.gz (Apple Silicon)"
if [ -f "thinline-radio-darwin-universal-v${VERSION}.tar.gz" ]; then
    echo "  - thinline-radio-darwin-universal-v${VERSION}.tar.gz (Universal - recommended)"
fi
echo ""
echo "Distribution directories:"
echo "  - dist-macos-amd64/"
echo "  - dist-macos-arm64/"
if [ -d "dist-macos-universal" ]; then
    echo "  - dist-macos-universal/"
fi
echo ""
echo "To deploy:"
echo "  1. Transfer the appropriate archive to your Mac"
echo "  2. Extract: tar -xzf thinline-radio-darwin-*.tar.gz"
echo "  3. Run: ./install.sh (or follow README.md for manual installation)"
echo ""

