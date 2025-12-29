#!/bin/bash

# Build script for Linux deployment
# This script builds the ThinLine Radio server for all Linux architectures (386, amd64, arm, arm64)

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

echo -e "${GREEN}Building ThinLine Radio v${VERSION} for all Linux architectures${NC}"
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

# Build for all Linux architectures - 32-bit support removed
ARCHITECTURES=("amd64" "arm64")
ARCH_NAMES=("64-bit" "ARM64")

for i in "${!ARCHITECTURES[@]}"; do
    ARCH="${ARCHITECTURES[$i]}"
    ARCH_NAME="${ARCH_NAMES[$i]}"
    
    echo ""
    echo -e "${YELLOW}Building Go server for Linux ${ARCH_NAME} (${ARCH})...${NC}"
    
    cd server
    
    # Set build variables
    export GOOS=linux
    export GOARCH=$ARCH
    export CGO_ENABLED=0  # Disable CGO for static binary
    
    # Build the binary
    BINARY_NAME="thinline-radio-linux-${ARCH}-v${VERSION}"
    echo "  Building binary: $BINARY_NAME"
    go build -ldflags="-s -w" -o "../$BINARY_NAME" .
    
    if [ ! -f "../$BINARY_NAME" ]; then
        echo -e "${RED}ERROR: Server build failed for ${ARCH}${NC}"
        exit 1
    fi
    
    echo -e "${GREEN}  ✓ Server built successfully for ${ARCH_NAME}${NC}"
    cd ..
    
    # Create distribution directory for this architecture
    DIST_DIR="dist-linux-${ARCH}"
    echo "  Creating distribution package..."
    
    # Clean and create dist directory
    rm -rf "$DIST_DIR"
    mkdir -p "$DIST_DIR"
    
    # Copy binary
    cp "$BINARY_NAME" "$DIST_DIR/thinline-radio"
    
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

# Copy config template (remove sensitive data)
echo "  Creating config template..."
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
cat > "$DIST_DIR/README.md" << EOF
# ThinLine Radio - Linux Deployment

## Version ${VERSION}

This distribution is built for Linux ${ARCH_NAME} and should work on most Linux distributions including:
- Ubuntu/Debian
- Fedora/CentOS/RHEL
- Arch Linux
- And other systemd-based distributions

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
   
   - **Install PostgreSQL:**
     \`\`\`bash
     sudo apt update
     sudo apt install postgresql postgresql-contrib
     \`\`\`
   
   - **Create Database and User:**
     \`\`\`bash
     sudo -u postgres psql
     \`\`\`
     In the PostgreSQL prompt:
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

## System Service Setup (Optional)

To run ThinLine Radio as a systemd service:

1. **Create service file:**
   \`\`\`bash
   sudo nano /etc/systemd/system/thinline-radio.service
   \`\`\`

2. **Add the following content:**
   \`\`\`
   [Unit]
   Description=ThinLine Radio Server
   After=network.target postgresql.service

   [Service]
   Type=simple
   User=your-user
   WorkingDirectory=/opt/thinline-radio
   ExecStart=/opt/thinline-radio/thinline-radio -config /opt/thinline-radio/thinline-radio.ini
   Restart=always
   RestartSec=10

   [Install]
   WantedBy=multi-user.target
   \`\`\`

3. **Enable and start the service:**
   \`\`\`bash
   sudo systemctl daemon-reload
   sudo systemctl enable thinline-radio
   sudo systemctl start thinline-radio
   \`\`\`

4. **Check status:**
   \`\`\`bash
   sudo systemctl status thinline-radio
   \`\`\`

## Requirements

- Linux distribution with systemd (most modern distributions)
- PostgreSQL 12+
- FFmpeg (for audio processing)
- Sufficient disk space for audio files

## Installation Location

Recommended installation path: \`/opt/thinline-radio\`

\`\`\`bash
sudo mkdir -p /opt/thinline-radio
sudo cp thinline-radio /opt/thinline-radio/
sudo cp thinline-radio.ini.template /opt/thinline-radio/
sudo chown -R your-user:your-user /opt/thinline-radio
\`\`\`

## Firewall Configuration

If using a firewall, allow the configured ports:

**For UFW (Ubuntu/Debian):**
\`\`\`bash
sudo ufw allow 3000/tcp  # HTTP port
sudo ufw allow 3443/tcp  # HTTPS port (if configured)
\`\`\`

**For firewalld (Fedora/CentOS/RHEL):**
\`\`\`bash
sudo firewall-cmd --permanent --add-port=3000/tcp
sudo firewall-cmd --permanent --add-port=3443/tcp
sudo firewall-cmd --reload
\`\`\`

## Logs

Logs are written to stdout/stderr. When running as a service, view logs with:

\`\`\`bash
sudo journalctl -u thinline-radio -f
\`\`\`

## Documentation

- **SETUP.md** - Comprehensive setup and administration guide (transcription, system admin, troubleshooting)
- **README.md** - Quick start guide (this file)

For more information, see the project repository: https://github.com/Thinline-Dynamic-Solutions/ThinLineRadio
EOF

# Create a simple deployment script
cat > "$DIST_DIR/deploy.sh" << 'EOF'
#!/bin/bash

# Simple deployment script for ThinLine Radio

set -e

INSTALL_DIR="/opt/thinline-radio"
SERVICE_USER="${SUDO_USER:-$USER}"

echo "ThinLine Radio Deployment Script"
echo "================================="
echo ""

# Check if running as root or with sudo
if [ "$EUID" -ne 0 ]; then 
    echo "Please run with sudo: sudo ./deploy.sh"
    exit 1
fi

# Create installation directory
echo "Creating installation directory..."
mkdir -p "$INSTALL_DIR"

# Copy files
echo "Copying files..."
cp thinline-radio "$INSTALL_DIR/"
cp thinline-radio.ini.template "$INSTALL_DIR/"

# Set permissions
echo "Setting permissions..."
chown -R "$SERVICE_USER:$SERVICE_USER" "$INSTALL_DIR"
chmod +x "$INSTALL_DIR/thinline-radio"

# Create data directory
mkdir -p "$INSTALL_DIR/data"
chown -R "$SERVICE_USER:$SERVICE_USER" "$INSTALL_DIR/data"

echo ""
echo "Installation complete!"
echo ""
echo "Next steps:"
echo "1. Edit configuration: sudo nano $INSTALL_DIR/thinline-radio.ini.template"
echo "2. Rename config: sudo mv $INSTALL_DIR/thinline-radio.ini.template $INSTALL_DIR/thinline-radio.ini"
echo "3. Set up systemd service (see README.md)"
echo "4. Start the service: sudo systemctl start thinline-radio"
EOF

chmod +x "$DIST_DIR/deploy.sh"

    # Create archive
    ARCHIVE_NAME="thinline-radio-linux-${ARCH}-v${VERSION}.tar.gz"
    echo "  Creating archive: $ARCHIVE_NAME"
    tar -czf "$ARCHIVE_NAME" -C "$DIST_DIR" .
    
    # Clean up temporary binary
    rm -f "$BINARY_NAME"
    
    echo -e "${GREEN}  ✓ Package created for ${ARCH_NAME}${NC}"
done

echo ""
echo -e "${GREEN}✓ Build complete!${NC}"
echo ""
echo "Distribution packages:"
for ARCH in "${ARCHITECTURES[@]}"; do
    if [ -f "thinline-radio-linux-${ARCH}-v${VERSION}.tar.gz" ]; then
        echo "  ✓ thinline-radio-linux-${ARCH}-v${VERSION}.tar.gz"
    fi
done
echo ""
echo "Distribution directories:"
for ARCH in "${ARCHITECTURES[@]}"; do
    if [ -d "dist-linux-${ARCH}" ]; then
        echo "  ✓ dist-linux-${ARCH}/"
    fi
done
echo ""
echo "To deploy:"
echo "  1. Extract the appropriate archive on your Linux server"
echo "  2. Follow the instructions in the distribution directory README.md"
echo ""

