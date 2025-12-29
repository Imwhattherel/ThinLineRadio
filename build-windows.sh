#!/bin/bash

# Build script for Windows deployment (cross-compile from macOS)
# This script builds the ThinLine Radio server for Windows amd64

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

echo -e "${GREEN}Building ThinLine Radio v${VERSION} for Windows amd64 (cross-compile from macOS)${NC}"
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

# Build the Go server (cross-compile for Windows)
echo ""
echo -e "${YELLOW}Cross-compiling Go server for Windows amd64...${NC}"

cd server

# Set build variables for Windows cross-compilation
export GOOS=windows
export GOARCH=amd64
export CGO_ENABLED=0  # Disable CGO for static binary (required for cross-compilation)

# Build the binary
BINARY_NAME="thinline-radio-windows-amd64-v${VERSION}.exe"
echo "  Building binary: $BINARY_NAME"
go build -ldflags="-s -w" -o "../$BINARY_NAME" .

if [ ! -f "../$BINARY_NAME" ]; then
    echo -e "${RED}ERROR: Server build failed${NC}"
    exit 1
fi

echo -e "${GREEN}  ✓ Server built successfully${NC}"
cd ..

# Create distribution directory
DIST_DIR="dist-windows"
echo ""
echo -e "${YELLOW}Creating distribution package...${NC}"

# Clean and create dist directory
rm -rf "$DIST_DIR"
mkdir -p "$DIST_DIR"

# Copy binary
cp "$BINARY_NAME" "$DIST_DIR/thinline-radio.exe"

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

# Copy config template
echo "  Creating config template..."
cat > "$DIST_DIR/thinline-radio.ini.template" << 'EOF'
# ThinLine Radio Configuration
# Rename this file to thinline-radio.ini and update with your settings

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
# ssl_cert_file = C:\path\to\cert.pem
# ssl_key_file = C:\path\to\key.pem
# ssl_auto_cert = yourdomain.com

# Base directory for data storage
# base_dir = C:\ProgramData\thinline-radio
EOF

# Create README for deployment
cat > "$DIST_DIR/README.md" << EOF
# ThinLine Radio - Windows Deployment

## Version ${VERSION}

This distribution is built for Windows amd64 (64-bit) and should work on Windows 10/11 and Windows Server 2016+.

## Quick Start

1. **Extract the files** to a directory (e.g., \`C:\\Program Files\\ThinLine Radio\`)

2. **Set up PostgreSQL:**
   
   You have two options for PostgreSQL setup:
   
   ### Option A: Interactive Setup Wizard (Recommended)
   
   Simply run the server and it will guide you through setup:
   \`\`\`cmd
   thinline-radio.exe
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
   
   - **Download and Install PostgreSQL:**
     - Download from: https://www.postgresql.org/download/windows/
     - Run installer and set a password for postgres user
   
   - **Create Database and User:**
     - Open Command Prompt as Administrator:
     \`\`\`cmd
     cd "C:\\Program Files\\PostgreSQL\\16\\bin"
     psql -U postgres
     \`\`\`
     
     In PostgreSQL prompt:
     \`\`\`sql
     CREATE DATABASE thinline_radio;
     CREATE USER thinline_user WITH PASSWORD 'your_secure_password';
     GRANT ALL PRIVILEGES ON DATABASE thinline_radio TO thinline_user;
     \\q
     \`\`\`

3. **Configure the server (if using manual setup):**
   - Copy \`thinline-radio.ini.template\` to \`thinline-radio.ini\`
   - Edit \`thinline-radio.ini\` with your database and server settings

4. **Run the server:**
   \`\`\`cmd
   thinline-radio.exe -config thinline-radio.ini
   \`\`\`

5. **Access the admin dashboard:**
   - Open your browser and navigate to \`http://localhost:3000/admin\`
   - Default password: \`admin\`
   - **Important**: Change the default password immediately after first login

## Windows Service Setup (Optional)

To run ThinLine Radio as a Windows Service, you can use a service wrapper like NSSM (Non-Sucking Service Manager):

1. **Download NSSM:** https://nssm.cc/download

2. **Install the service:**
   \`\`\`cmd
   nssm install ThinLineRadio "C:\\Program Files\\ThinLine Radio\\thinline-radio.exe"
   nssm set ThinLineRadio AppParameters "-config C:\\Program Files\\ThinLine Radio\\thinline-radio.ini"
   nssm set ThinLineRadio AppDirectory "C:\\Program Files\\ThinLine Radio"
   nssm set ThinLineRadio DisplayName "ThinLine Radio Server"
   nssm set ThinLineRadio Description "ThinLine Radio Scanner Server"
   nssm set ThinLineRadio Start SERVICE_AUTO_START
   \`\`\`

3. **Start the service:**
   \`\`\`cmd
   nssm start ThinLineRadio
   \`\`\`

4. **Check status:**
   \`\`\`cmd
   nssm status ThinLineRadio
   \`\`\`

## Requirements

- Windows 10/11 or Windows Server 2016+
- PostgreSQL 12+
- FFmpeg (for audio processing) - download from https://ffmpeg.org/download.html
- Sufficient disk space for audio files

## Installation Location

Recommended installation path: \`C:\\Program Files\\ThinLine Radio\`

Or for user-specific installation: \`C:\\Users\\YourUsername\\ThinLine Radio\`

## Firewall Configuration

If using Windows Firewall, allow the configured ports:

1. Open Windows Defender Firewall with Advanced Security
2. Click "Inbound Rules" → "New Rule"
3. Select "Port" → Next
4. Select "TCP" and enter port \`3000\` (and \`3443\` if using SSL)
5. Allow the connection → Next
6. Select profiles (Domain, Private, Public as needed) → Next
7. Name it "ThinLine Radio HTTP" (and "ThinLine Radio HTTPS")
8. Finish

## Logs

Logs are written to stdout/stderr. When running as a service, check the service logs or run from command prompt to see output.

## Troubleshooting

- **"Port already in use":** Another application is using the configured port. Change the port in \`thinline-radio.ini\`
- **Database connection failed:** Verify database is running and credentials in \`thinline-radio.ini\` are correct
- **Permission denied:** Run as Administrator or check file/directory permissions

## Documentation

- **SETUP.md** - Comprehensive setup and administration guide (transcription, system admin, troubleshooting)
- **README.md** - Quick start guide (this file)

For more information, see the project repository: https://github.com/Thinline-Dynamic-Solutions/ThinLineRadio
EOF

# Create ZIP archive
ARCHIVE_NAME="thinline-radio-windows-amd64-v${VERSION}.zip"
echo "  Creating archive: $ARCHIVE_NAME"
cd "$DIST_DIR"
zip -r "../$ARCHIVE_NAME" . > /dev/null
cd ..

# Clean up temporary binary
rm -f "$BINARY_NAME"

echo ""
echo -e "${GREEN}✓ Build complete!${NC}"
echo ""
echo "Distribution package: $ARCHIVE_NAME"
echo "Distribution directory: $DIST_DIR/"
echo ""
echo "To deploy:"
echo "  1. Transfer the ZIP file to your Windows server"
echo "  2. Extract the archive"
echo "  3. Follow the instructions in $DIST_DIR/README.md"
echo ""

