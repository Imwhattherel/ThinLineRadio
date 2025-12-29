# ThinLine Radio - Setup and Administration Guide

This guide covers how to run ThinLine Radio and configure essential features.

## Table of Contents

1. [Running the Package](#running-the-package)
2. [Database Configuration](#database-configuration)
3. [Configuration File Options](#configuration-file-options)
4. [Command-Line Tools](#command-line-tools)
5. [Transcription Services Setup](#transcription-services-setup)
6. [Tone Detection](#tone-detection)
7. [Keyword Alerts](#keyword-alerts)
8. [System Administration](#system-administration)
9. [Advanced Configuration](#advanced-configuration)
10. [Troubleshooting](#troubleshooting)

---

## Running the Package

### Prerequisites

Before running ThinLine Radio, ensure you have:

- **PostgreSQL 12+** installed and running
- **FFmpeg** installed (required for audio processing)
- Sufficient disk space for audio files
- Network access for API calls (if using transcription services)

### Quick Start

1. **Extract the distribution package** to your desired location

2. **Set up PostgreSQL:**
   
   You have two options for PostgreSQL setup:
   
   **Option A: Use Interactive Setup Wizard (Recommended)**
   - Simply run the server without a config file:
     ```bash
     # Linux/macOS
     ./thinline-radio
     
     # Windows
     thinline-radio.exe
     ```
   - The interactive wizard will guide you through:
     - PostgreSQL installation detection
     - Database and user creation (local installation)
     - Remote PostgreSQL server configuration (if PostgreSQL is on another machine)
     - Configuration file generation
   - Follow the on-screen prompts to complete setup
   
   **Option B: Manual Configuration**
   - Install PostgreSQL (see [Database Configuration](#database-configuration) below)
   - Create database and user manually
   - Create and edit configuration file:
     ```bash
     # Linux/macOS
     cp thinline-radio.ini.template thinline-radio.ini
     nano thinline-radio.ini
     
     # Windows
     copy thinline-radio.ini.template thinline-radio.ini
     notepad thinline-radio.ini
     ```

3. **Run the server:**
   ```bash
   # Linux/macOS
   ./thinline-radio -config thinline-radio.ini
   
   # Windows
   thinline-radio.exe -config thinline-radio.ini
   ```

4. **Access the admin dashboard:**
   - Navigate to `http://localhost:3000/admin`
   - Default password: `admin`
   - **Important**: Change the default password immediately after first login

For platform-specific deployment instructions (system services, installation paths, etc.), see:
- [Linux Deployment](platforms/linux.md)
- [Windows Deployment](platforms/windows.md)
- [macOS Deployment](platforms/macos.md)
- [FreeBSD Deployment](platforms/freebsd.md)

---

## Database Configuration

ThinLine Radio includes an **Interactive Setup Wizard** that can automatically configure PostgreSQL for you. The wizard supports:
- **Local PostgreSQL**: Automatically creates database and user if PostgreSQL is installed locally
- **Remote PostgreSQL**: Connects to an existing PostgreSQL server on another machine
- **Guided Installation**: Provides platform-specific installation instructions if PostgreSQL is not detected

**To use the Interactive Setup Wizard**, simply run ThinLine Radio without a configuration file and follow the on-screen prompts.

If you prefer manual configuration, follow the instructions below.

### PostgreSQL Setup

#### Linux/macOS Installation

**Ubuntu/Debian:**
```bash
sudo apt update
sudo apt install postgresql postgresql-contrib
```

**Fedora/CentOS/RHEL:**
```bash
sudo dnf install postgresql postgresql-server
sudo postgresql-setup --initdb
sudo systemctl enable postgresql
sudo systemctl start postgresql
```

**macOS (Homebrew):**
```bash
brew install postgresql@16
brew services start postgresql@16
```

#### Windows Installation

1. **Download PostgreSQL:**
   - Visit: https://www.postgresql.org/download/windows/
   - Download the PostgreSQL installer
   - Run the installer and follow the setup wizard

2. **During installation:**
   - Choose installation directory (default: `C:\Program Files\PostgreSQL\<version>`)
   - Set a password for the `postgres` superuser account
   - Choose port (default: 5432)
   - Select locale (default: English, United States)

3. **Verify installation:**
   - Open Command Prompt as Administrator
   - Navigate to PostgreSQL bin directory:
     ```cmd
     cd "C:\Program Files\PostgreSQL\<version>\bin"
     ```
   - Test connection:
     ```cmd
     psql -U postgres
     ```
   - Enter the password you set during installation

#### Create Database and User

**Linux/macOS:**
```bash
sudo -u postgres psql
```

**Windows (Command Prompt):**
```cmd
cd "C:\Program Files\PostgreSQL\<version>\bin"
psql -U postgres
```

In the PostgreSQL prompt (all platforms):
```sql
CREATE DATABASE thinline_radio;
CREATE USER thinline_user WITH PASSWORD 'your_secure_password';
GRANT ALL PRIVILEGES ON DATABASE thinline_radio TO thinline_user;
\q
```

### Configuration File

Update `thinline-radio.ini` with your database settings:

```ini
db_type = postgresql
db_host = localhost
db_port = 5432
db_name = thinline_radio
db_user = thinline_user
db_pass = your_secure_password
```

**Note:** SQLite is no longer supported in v7. PostgreSQL is recommended.

---

## Configuration File Options

The `thinline-radio.ini` configuration file supports the following options:

### Database Settings

```ini
# Database type (postgresql only)
db_type = postgresql

# Database host (IP address or hostname)
db_host = localhost

# Database port (default: 5432 for PostgreSQL)
db_port = 5432

# Database name
db_name = thinline_radio

# Database username
db_user = thinline_user

# Database password
db_pass = your_secure_password
```

### Server Settings

```ini
# HTTP listening address (default: :3000)
# Use 0.0.0.0:3000 to listen on all interfaces
listen = 0.0.0.0:3000

# HTTPS listening address (optional)
# Uncomment to enable HTTPS on a different port
# ssl_listen = 0.0.0.0:3443
```

### SSL/TLS Configuration

**Option 1: Manual Certificate Files**
```ini
# Path to SSL certificate file (PEM format)
ssl_cert_file = /path/to/cert.pem

# Path to SSL private key file (PEM format)
ssl_key_file = /path/to/key.pem

# HTTPS listening address
ssl_listen = 0.0.0.0:3443
```

**Option 2: Let's Encrypt Automatic Certificate**
```ini
# Domain name for Let's Encrypt automatic certificate
# Requires ports 80 and 443 to be open to the world
ssl_auto_cert = yourdomain.com

# HTTPS listening address
ssl_listen = 0.0.0.0:3443
```

**Note:** For secure remote access, consider using [Cloudflare Tunnels](platforms/linux.md#cloudflare-tunnels) instead of traditional SSL certificates.

### Data Storage

```ini
# Base directory where all data will be written
# Default: same directory as executable (or ~/ThinLine Radio if not writable)
# Linux example: /var/lib/thinline-radio
# Windows example: C:\ProgramData\thinline-radio
base_dir = /var/lib/thinline-radio
```

### Debug Logging

```ini
# Enable debug logging for tone and keyword detection
# Creates tone-keyword-debug.log in the base directory
enable_debug_log = true
```

**When to use debug logging:**
- Troubleshooting tone detection issues
- Debugging keyword matching problems
- Verifying tone frequencies and tolerances

**Note:** Debug logging can generate large log files. Disable when not needed.

---

## Command-Line Tools

ThinLine Radio includes a command-line tool for advanced administrative tasks.

### Basic Usage

```bash
# Linux/macOS
./thinline-radio -cmd <command> [options]

# Windows
thinline-radio.exe -cmd <command> [options]
```

### Available Commands

#### Change Admin Password

Change the administrator password from the command line:

```bash
# Linux/macOS
./thinline-radio -cmd admin-password +password <new_password>

# Windows
thinline-radio.exe -cmd admin-password +password <new_password>
```

**Alternative method (Linux/macOS only):**
```bash
RDIO_ADMIN_PASSWORD=<new_password> ./thinline-radio -cmd admin-password
```

#### Export Configuration

Export the server's configuration to a JSON file:

```bash
./thinline-radio -cmd config-get +out config-backup.json
```

This exports all system configuration including:
- Systems and talkgroups
- Options and settings
- Radio Reference settings
- Transcription configuration
- Email and Stripe settings

#### Import Configuration

Import a configuration file to restore settings:

```bash
./thinline-radio -cmd config-set +in config-backup.json
```

**Warning:** This will overwrite your current configuration. Use with caution.

#### Login/Logout

Manage authentication sessions:

```bash
# Login (creates a session token)
./thinline-radio -cmd login +password <admin_password>

# Logout (removes session token)
./thinline-radio -cmd logout
```

**Session tokens:**
- Stored in `.<executable-name>.token` in the current directory
- Used for authenticated command-line operations
- Automatically used for subsequent commands after login

### Global Options

```bash
# Specify server URL (default: http://localhost:3000/)
./thinline-radio -cmd <command> +url http://your-server.com:3000

# Specify token file location (default: .<executable-name>.token)
./thinline-radio -cmd <command> +token /path/to/token.file
```

### Command-Line Arguments

ThinLine Radio supports the following command-line arguments:

```bash
# Configuration
-config <file>              # Configuration file (default: thinline-radio.ini)
-config_save                # Save current configuration to thinline-radio.ini

# Database
-db_type <type>             # Database type (postgresql only)
-db_host <host>             # Database host (default: localhost)
-db_port <port>             # Database port (default: 5432 for PostgreSQL)
-db_name <name>             # Database name
-db_user <user>             # Database username
-db_pass <password>         # Database password

# Server
-listen <address>           # HTTP listening address (default: :3000)
-base_dir <path>            # Base directory for data storage

# SSL/TLS
-ssl_listen <address>       # HTTPS listening address
-ssl_cert_file <path>       # SSL certificate file (PEM format)
-ssl_key_file <path>        # SSL private key file (PEM format)
-ssl_auto_cert <domain>     # Domain name for Let's Encrypt automatic certificate

# Service Management
-service <action>           # Service command: start, stop, restart, install, uninstall

# Administrative
-admin_password <password>  # Change admin password
-cmd <command>              # Advanced administrative tasks (see above)

# Information
-version                    # Show application version
-h                          # Show help message
```

**Examples:**

```bash
# Run with custom config file
./thinline-radio -config /etc/thinline-radio/config.ini

# Run on different port
./thinline-radio -listen 0.0.0.0:8080

# Save current command-line settings to config file
./thinline-radio -db_host localhost -db_name mydb -config_save

# Install as system service
sudo ./thinline-radio -service install

# Show version
./thinline-radio -version
```

For platform-specific service installation, see the [Platform-Specific Guides](platforms/).

---

## Transcription Services Setup

ThinLine Radio supports multiple transcription providers. Choose the one that best fits your needs.

### Important: Transcription Requirements

**Transcription only occurs for channels where users have alerts enabled and keyword detection configured.**

- Transcription is performed when:
  1. A user has **alerts enabled** for a specific talkgroup/channel
  2. The user has **keyword alerts enabled** for that talkgroup
  3. The user has **keywords configured** (either individual keywords or keyword lists)

- If no users have alerts and keywords configured for a talkgroup, transcription will not be performed for that talkgroup, saving processing resources and API costs.

- To enable transcription for a talkgroup:
  1. Users must configure their alert preferences for that talkgroup
  2. Users must enable "Keyword Alerts" in their preferences
  3. Users must add keywords they want to monitor

This ensures transcription only happens when it's needed for alerting purposes.

### Google Cloud Speech-to-Text

#### Prerequisites
- A Google account (Gmail account works)
- Access to Google Cloud Console (free to access)

#### Setup Steps

1. **Access Google Cloud Console**
   - Navigate to: https://console.cloud.google.com
   - Sign in with your Google account

2. **Create or Select a Project**
   - Click "Select a project" dropdown → "NEW PROJECT"
   - Enter project name: `ThinLine Radio Transcription`
   - Click "CREATE"

3. **Enable Speech-to-Text API**
   - Navigate to: `APIs & Services` → `Library`
   - Search for: `Cloud Speech-to-Text API`
   - Click "ENABLE"

4. **Create an API Key**
   - Navigate to: `APIs & Services` → `Credentials`
   - Click "+ CREATE CREDENTIALS" → `API key`
   - Copy your API key immediately (you won't see it again)

5. **Restrict Your API Key (Recommended)**
   - Click on your API key
   - Under "API restrictions": Select `Restrict key` → Check `Cloud Speech-to-Text API`
   - Click "SAVE"

6. **Configure in ThinLine Radio Admin**
   - Navigate to: `http://your-server:3000/admin` → `Config` → `Transcription Settings`
   - Toggle "Transcription Enabled" to **ON**
   - **Transcription Provider**: Select `Google Cloud Speech-to-Text`
   - **Google Cloud API Key**: Paste your API key
   - **Language**: Enter language code (e.g., `en-US`, `en-GB`, `auto`)
   - **Worker Pool Size**: 3-5 workers (adjust based on server capacity)
   - **Min Call Duration**: 0 (or set minimum seconds to skip short calls)
   - Click "Save"

#### Supported Languages

- **English**: `en-US`, `en-GB`, `en-AU`
- **Spanish**: `es-ES`, `es-US`, `es-MX`
- **French**: `fr-FR`, `fr-CA`
- **German**: `de-DE`
- **Auto-detect**: `auto`

For full list: https://cloud.google.com/speech-to-text/docs/languages

#### Pricing

- **Free Tier**: 60 minutes per month free
- **Paid Tier**: $0.006 per 15 seconds (~$0.024/minute) for Standard models

#### Troubleshooting

**"transcription provider 'Google Cloud Speech-to-Text' not available"**
- Verify API key is entered correctly (no leading/trailing spaces)
- Ensure API is enabled in Google Cloud Console

**"Google API request failed with status 403"**
- Verify API key is correct
- Check that Speech-to-Text API is enabled
- Verify billing account is linked (required even for free tier)

**"Google API request failed with status 400"**
- Check audio format (supports: WAV, MP3, M4A, OGG, WebM)
- Verify sample rate (default 16000 Hz)

---

### Azure Speech Services

#### Prerequisites
- A Microsoft account (Outlook, Hotmail, or any Microsoft account)
- Access to Azure Portal (free tier available)
- ffmpeg installed on your server (required for audio conversion)

#### Setup Steps

1. **Access Azure Portal**
   - Navigate to: https://portal.azure.com
   - Sign in with your Microsoft account

2. **Create a Speech Services Resource**
   - Click "+ Create a resource"
   - Search for "Speech Services"
   - Click "Create"
   - Fill in:
     - **Subscription**: Select your subscription
     - **Resource Group**: Create new or use existing
     - **Region**: Select closest region (note the code, e.g., `eastus`, `westus2`)
     - **Name**: Enter a name (e.g., `thinline-radio-speech`)
     - **Pricing tier**: Free (F0) or Standard (S0)
   - Click "Review + create" → "Create"

3. **Get Your Subscription Key and Region**
   - Go to your Speech resource → "Keys and Endpoint"
   - Copy **Key 1** (or Key 2)
   - Note your **Region Code** (e.g., `eastus`, not "East US")

4. **Configure in ThinLine Radio Admin**
   - Navigate to: `http://your-server:3000/admin` → `Config` → `Transcription Settings`
   - Toggle "Transcription Enabled" to **ON**
   - **Transcription Provider**: Select `Azure Speech Services`
   - **Azure Subscription Key**: Paste your key
   - **Azure Region**: Enter region code exactly (lowercase, no spaces, e.g., `eastus`)
   - **Language**: Enter language code (e.g., `en-US`, `auto`)
   - **Worker Pool Size**: 3-5 workers (2-5 for Free tier, 5-10 for Standard)
   - **Min Call Duration**: 0 (or set minimum seconds)
   - Click "Save"

#### Audio Format Conversion

ThinLine Radio automatically converts audio to WAV format before sending to Azure:
- **Input formats**: M4A, MP3, WAV, OGG, WebM (handled automatically)
- **Output format**: WAV 16kHz mono (optimized for Azure)
- **Conversion**: Uses ffmpeg (must be installed)

#### Supported Languages

- **English**: `en-US`, `en-GB`, `en-AU`, `en-CA`
- **Spanish**: `es-ES`, `es-US`, `es-MX`
- **French**: `fr-FR`, `fr-CA`
- **German**: `de-DE`
- **Auto-detect**: `auto`

For full list: https://docs.microsoft.com/azure/cognitive-services/speech-service/language-support

#### Pricing

- **Free Tier (F0)**: 5 hours per month free, then $1.00/hour
- **Standard Tier (S0)**: $1.00 per hour (pay-as-you-go)

#### Rate Limiting

**Free Tier (F0):**
- Up to 20 concurrent requests
- 5 hours per month quota

**Standard Tier (S0):**
- Up to 100+ concurrent requests
- No monthly quota

**Best Practices:**
- Free tier: Use 2-5 workers maximum
- Standard tier: Use 5-10 workers
- Monitor for HTTP 429 errors (rate limit exceeded)

#### Troubleshooting

**"transcription provider 'Azure Speech Services' not available"**
- Verify both subscription key and region are filled
- Region must be lowercase, no spaces (e.g., `eastus` not `East US`)

**"Azure API request failed with status 401"**
- Verify subscription key is correct
- Check for leading/trailing spaces

**"Azure API request failed with status 404"**
- Verify region code is correct (lowercase, no spaces)
- Common codes: `eastus`, `westus2`, `westeurope`, `southeastasia`

**"Azure API request failed with status 429"**
- Reduce worker pool size
- Free tier: Use 2-5 workers maximum
- Wait for rate limit window to reset

**"ffmpeg conversion failed"**
- Install ffmpeg:
  - macOS: `brew install ffmpeg`
  - Linux: `sudo apt install ffmpeg` or `sudo dnf install ffmpeg`
  - Windows: Download from https://ffmpeg.org/download.html

---

### AssemblyAI

#### Prerequisites
- An AssemblyAI account
- Access to AssemblyAI dashboard

#### Setup Steps

1. **Create AssemblyAI Account**
   - Navigate to: https://www.assemblyai.com/
   - Sign up for a free account
   - Free tier includes 5 hours of transcription per month

2. **Get Your API Key**
   - Log in to AssemblyAI dashboard: https://www.assemblyai.com/app
   - Navigate to your account settings
   - Copy your **API Key** (starts with your account identifier)

3. **Configure in ThinLine Radio Admin**
   - Navigate to: `http://your-server:3000/admin` → `Config` → `Transcription Settings`
   - Toggle "Transcription Enabled" to **ON**
   - **Transcription Provider**: Select `AssemblyAI`
   - **AssemblyAI API Key**: Paste your API key
   - **Language**: Enter language code (e.g., `en`, `en-US`, `auto`)
   - **Worker Pool Size**: 3-5 workers (adjust based on server capacity)
   - **Min Call Duration**: 0 (or set minimum seconds to skip short calls)
   - Click "Save"

#### Audio Format Conversion

AssemblyAI automatically receives audio converted to WAV format:
- **Input formats**: M4A, MP3, WAV, OGG, WebM (handled automatically)
- **Output format**: WAV 16kHz mono (optimized for AssemblyAI)
- **Conversion**: Uses ffmpeg (must be installed)

#### Supported Languages

AssemblyAI supports many languages. Common options:
- **English**: `en`, `en-US`, `en-GB`
- **Spanish**: `es`, `es-ES`, `es-US`
- **French**: `fr`, `fr-FR`
- **German**: `de`, `de-DE`
- **Auto-detect**: `auto`

For full list: https://www.assemblyai.com/docs/guides/supported-languages

#### Pricing

- **Free Tier**: 5 hours per month free
- **Pay-as-you-go**: $0.00025 per second (~$0.90/hour) after free tier

#### Troubleshooting

**"transcription provider 'AssemblyAI' not available"**
- Verify API key is entered correctly (no leading/trailing spaces)
- Ensure API key is valid and active

**"AssemblyAI API request failed"**
- Verify API key is correct
- Check your AssemblyAI account quota/usage
- Ensure ffmpeg is installed (required for audio conversion)

---

### Whisper (Local)

Whisper can be run locally using an OpenAI-compatible Whisper API server. This provides privacy and no per-transcription costs, but requires local compute resources.

#### Prerequisites
- Python 3.8+ installed
- Sufficient CPU/GPU resources for local transcription
- ffmpeg installed (required for audio processing)

#### Setup Steps

1. **Install a Whisper API Server**

   **Option A: Using whisper-api (Recommended)**
   ```bash
   # Install whisper-api
   pip install git+https://github.com/openai/whisper.git
   pip install fastapi uvicorn
   
   # Or use a pre-built solution like:
   # https://github.com/ahmetoner/whisper-asr-webservice
   ```

   **Option B: Using a Docker container**
   ```bash
   docker run -d -p 8000:8000 onerahmet/openai-whisper-asr-webservice:latest-gpu
   # Or for CPU-only:
   docker run -d -p 8000:8000 onerahmet/openai-whisper-asr-webservice:latest-cpu
   ```

2. **Start the Whisper API Server**

   If running manually:
   ```bash
   # Example using a simple FastAPI server
   uvicorn whisper_server:app --host 0.0.0.0 --port 8000
   ```

   The server should be accessible at `http://localhost:8000` (or your server's IP if running remotely)

3. **Verify the Server is Running**

   Test the endpoint:
   ```bash
   curl http://localhost:8000/health
   # Or
   curl http://localhost:8000/v1/models
   ```

4. **Configure in ThinLine Radio Admin**
   - Navigate to: `http://your-server:3000/admin` → `Config` → `Transcription Settings`
   - Toggle "Transcription Enabled" to **ON**
   - **Transcription Provider**: Select `Whisper API`
   - **Whisper API URL**: Enter your Whisper server URL (e.g., `http://localhost:8000`)
   - **Whisper API Key**: Leave empty unless your server requires authentication
   - **Language**: Enter language code (e.g., `en`, `auto`)
   - **Worker Pool Size**: 2-3 workers (local processing is CPU/GPU intensive)
   - **Min Call Duration**: 0 (or set minimum seconds to skip short calls)
   - Click "Save"

#### Supported Languages

Whisper supports 99+ languages. Common options:
- **English**: `en`
- **Spanish**: `es`
- **French**: `fr`
- **German**: `de`
- **Auto-detect**: `auto` (recommended - Whisper detects language automatically)

For full list: https://github.com/openai/whisper#available-models-and-languages

#### Performance Considerations

**CPU vs GPU:**
- **CPU**: Slower but works on any system (expect 1-5x real-time processing)
- **GPU**: Much faster (expect 10-50x real-time processing) but requires CUDA-compatible GPU

**Model Selection:**
- **tiny**: Fastest, least accurate (~39M parameters)
- **base**: Good balance (~74M parameters)
- **small**: Better accuracy (~244M parameters)
- **medium**: High accuracy (~769M parameters)
- **large**: Best accuracy (~1550M parameters)

**Recommendations:**
- Start with `base` or `small` model for good balance
- Use `tiny` only for testing or very low-resource systems
- Use `large` only if you have powerful GPU and need best accuracy

#### Troubleshooting

**"transcription provider 'Whisper API' not available"**
- Verify Whisper API URL is correct and accessible
- Check that the Whisper server is running
- Test the URL in a browser: `http://localhost:8000/health`

**"Connection refused" or "Connection timeout"**
- Ensure Whisper API server is running
- Check firewall settings if running on a different machine
- Verify the port (default: 8000) is correct

**"Transcription is very slow"**
- Reduce worker pool size (1-2 workers for CPU)
- Consider using a smaller Whisper model
- If using CPU, expect slower processing times
- Consider using GPU-accelerated Whisper server

**"Out of memory" errors**
- Reduce worker pool size
- Use a smaller Whisper model
- Ensure sufficient RAM/VRAM available

#### Running Whisper on a Separate Server

If running Whisper on a different machine:

1. **Configure Whisper server to accept remote connections:**
   ```bash
   # Example: Run on 0.0.0.0 to accept connections from any IP
   uvicorn whisper_server:app --host 0.0.0.0 --port 8000
   ```

2. **Update ThinLine Radio configuration:**
   - **Whisper API URL**: Use the remote server's IP or hostname (e.g., `http://192.168.1.100:8000`)

3. **Security considerations:**
   - Use a firewall to restrict access
   - Consider using an API key if your Whisper server supports it
   - Use HTTPS if accessing over the internet

---

## Tone Detection

ThinLine Radio supports tone detection for alerting. You can configure tone sets manually or import them from CSV files or TwoToneDetect configuration.

### Tone Detection Overview

Tone detection can identify:
- **Two-Tone Sequences**: A tone followed by a B tone (e.g., 853.0 Hz + 960.0 Hz)
- **Long Tones**: Single continuous tone (e.g., 1500.0 Hz for 5+ seconds)

### CSV Import Format

You can import tone sets from a CSV file. The CSV format is flexible and supports various header names.

#### Required Fields

- **Description/Label/Name** (required): The name/description of the tone set
  - Accepts: `description`, `label`, `name` (case-insensitive)

#### Tone Frequency Fields (at least one required)

- **A Tone Frequency**: First tone in a two-tone sequence
  - Accepts: `atone`, `a`, `afreq`, `a_frequency` (case-insensitive)
  - Value: Frequency in Hz (e.g., `853.0`)

- **B Tone Frequency**: Second tone in a two-tone sequence
  - Accepts: `btone`, `b`, `bfreq`, `b_frequency` (case-insensitive)
  - Value: Frequency in Hz (e.g., `960.0`)

- **Long Tone Frequency**: Single continuous tone
  - Accepts: `longtone`, `long`, `longfreq`, `long_frequency` (case-insensitive)
  - Value: Frequency in Hz (e.g., `1500.0`)

#### Optional Fields

- **A Tone Duration**: Minimum duration for A tone in seconds
  - Accepts: `atonelength`, `a_length`, `a_duration` (case-insensitive)
  - Default: `0.6` seconds

- **B Tone Duration**: Minimum duration for B tone in seconds
  - Accepts: `btonelength`, `b_length`, `b_duration` (case-insensitive)
  - Default: `0.6` seconds

- **Long Tone Duration**: Minimum duration for long tone in seconds
  - Accepts: `longtonelength`, `long_length`, `long_duration` (case-insensitive)
  - Default: `5.0` seconds

- **Tolerance**: Frequency tolerance as a percentage (1% = 5 Hz)
  - Accepts: `tone_tolerance`, `tolerance` (case-insensitive)
  - Default: `10` (which equals 10% = 50 Hz)
  - **Note**: Tolerance is specified as a percentage value. For example:
    - `1` = 1% = 5 Hz tolerance
    - `10` = 10% = 50 Hz tolerance
    - `20` = 20% = 100 Hz tolerance

#### Sample CSV File

Example CSV format:

```csv
Description,ATone,BTone,AToneLength,BToneLength,LongTone,LongToneLength,Tolerance
Fire Department Dispatch,853.0,960.0,0.6,0.6,,,10
Police Dispatch,1000.0,1200.0,0.6,0.6,,,10
EMS Dispatch,1100.0,1300.0,0.6,0.6,,,10
Long Tone Alert,,,5.0,,1500.0,5.0,10
Two Tone Pager,800.0,900.0,0.6,0.6,,,15
Single A Tone Only,500.0,,0.6,,,,10
Custom Duration Tones,700.0,800.0,1.0,1.0,,,20
```

**Note:** Empty cells are allowed and will use default values. You only need to include columns you're using.

A complete sample CSV file is available at: `examples/tone-detection-sample.csv`

#### CSV Import Instructions

1. **Prepare your CSV file** using the format above
2. **Navigate to Talkgroup Configuration**:
   - Go to `http://your-server:3000/admin` → `Config` → `Systems` → [Select System] → [Select Talkgroup]
3. **Import Tone Sets**:
   - Click the "Import Tone Sets" button
   - Select "CSV" format
   - Choose your CSV file
   - Click "Import"
4. **Verify Import**:
   - Check that tone sets appear in the tone detection list
   - Review any warnings shown during import

#### CSV Format Notes

- Headers are case-insensitive (e.g., `ATone`, `atone`, `A_Tone` all work)
- Spaces, dashes, and underscores in headers are ignored
- Empty cells are allowed (will use defaults)
- At least one tone frequency (A, B, or Long) must be present in each row
- Each row must have a description/label/name

#### TwoToneDetect Import

ThinLine Radio also supports importing from TwoToneDetect configuration format. Use the "TwoToneDetect" option when importing tone sets.

### FDMA Tones (Digital)

**FDMA (Frequency Division Multiple Access)** systems use digital tones that are different from standard analog two-tone paging systems.

#### Important: FDMA Frequency Range Requirements

**FDMA tones are digital tones that must fall within specific frequency ranges and cannot overlap other ranges, or they will cause false alerts.**

- **Frequency Range Restrictions**: Each FDMA channel group has a defined frequency range
- **No Overlap**: Tones must fall within their assigned range and cannot overlap with other FDMA channel ranges
- **Standard Frequencies**: Each FDMA channel has a standard frequency within its range
- **False Alert Prevention**: Using frequencies outside the assigned range or overlapping ranges will cause false alerts

#### FDMA Tone Frequency Table

FDMA defines 72 channel groups, each with a standard frequency and frequency range. A complete FDMA tone frequency table is available in the distribution package:

- **CSV Format**: `examples/fdma-tone-frequency-table.csv`
- **Reference Image**: `examples/Tones for FDMA.PNG`

The table includes:
- **Index**: Channel group number (1-72)
- **Freq Range (Hz)**: The allowed frequency range for that channel
- **Standard Freq (Hz)**: The standard frequency within that range

#### Configuring FDMA Tones

When configuring FDMA tones, you must:

1. **Use the correct frequency range**: Ensure your tone frequency falls within the assigned FDMA channel range
2. **Avoid overlap**: Do not use frequencies that overlap with other FDMA channel ranges
3. **Use standard frequencies when possible**: The standard frequency for each channel is recommended

**Example FDMA Tone Configuration:**
```csv
Description,ATone,BTone,AToneLength,BToneLength,LongTone,LongToneLength,Tolerance
FDMA Channel 1,283.0,,0.5,,,,10
FDMA Channel 19,849.0,,0.5,,,,10
FDMA Channel 37,1406.5,,0.5,,,,10
FDMA Channel 55,2110.0,,0.5,,,,10
FDMA Channel 72,3082.25,,0.5,,,,10
```

**Note:** Always verify your FDMA tone frequencies against the frequency table to ensure they fall within the correct range and don't overlap with other channels.

### Standard Analog Tones

Standard analog two-tone paging systems do **not** have the frequency range restrictions of FDMA tones:

- **No Range Restrictions**: Analog tones can use any frequency within the detector's range (0-5000 Hz)
- **Flexible Configuration**: You can configure any frequency combination without worrying about overlapping ranges
- **Traditional Two-Tone**: Standard A+B tone sequences work as expected
- **Long Tones**: Single continuous tones can be configured at any frequency

Standard analog tone detection is the default mode and works for traditional paging systems, two-tone sequences, and long tone alerts.

---

## Keyword Alerts

Keyword alerts allow users to receive notifications when specific words or phrases are detected in transcribed audio.

### How Keyword Alerts Work

1. **Transcription Required**: Keywords are matched against transcribed audio text
2. **User-Specific**: Each user configures their own keywords for each talkgroup
3. **Case-Insensitive**: Keyword matching is case-insensitive (e.g., "FIRE" matches "fire", "Fire", "FIRE")
4. **Alert Triggering**: When a keyword is detected in a transcript, an alert is created and notifications are sent

### Configuring Keywords

Users can configure keywords in two ways:

#### Individual Keywords

Users can add individual keywords to monitor:
- Navigate to Alert Preferences for a specific talkgroup
- Enable "Keyword Alerts"
- Add keywords one at a time (e.g., "FIRE", "ACCIDENT", "EMERGENCY")

#### Keyword Lists

Administrators can create keyword lists that users can subscribe to:
- Navigate to Admin → Config → Keyword Lists
- Create a new keyword list (e.g., "Emergency Keywords", "Fire Department Keywords")
- Add multiple keywords to the list


### Keyword Matching

- **Exact Match**: Keywords must appear exactly as configured (case-insensitive)
- **Word Boundaries**: Keywords are matched as whole words within the transcript
- **Multiple Matches**: If multiple keywords match in a single call, all matched keywords are included in the alert

### Best Practices

1. **Use ALL CAPS**: Since transcripts are typically in ALL CAPS, configure keywords in uppercase for clarity
2. **Be Specific**: Use specific keywords to reduce false positives (e.g., "STRUCTURE FIRE" instead of just "FIRE")
3. **Common Keywords**: Consider creating keyword lists for common alert types that multiple users need
4. **Test Keywords**: Test your keywords with sample transcripts to ensure they match as expected

### Example Keywords

A comprehensive keyword list is available in the distribution package at `examples/keywords.json`. This file contains pre-configured keyword lists for:

- **General Keywords**: Emergency situations, mutual aid, weather events, power outages
- **Fire Keywords**: Structure fires, vehicle fires, hazmat, alarms, rescue operations
- **Medical Keywords**: Cardiac events, trauma, medical emergencies, transport
- **Law Enforcement Keywords**: Pursuits, domestic incidents, weapons, arrests, traffic stops

Common keyword examples:
- **Emergency**: "EMERGENCY", "URGENT", "IMMEDIATE"
- **Fire**: "FIRE", "STRUCTURE FIRE", "GRASS FIRE", "SMOKE"
- **Medical**: "AMBULANCE", "MEDICAL", "INJURY", "HEART ATTACK"
- **Traffic**: "ACCIDENT", "MVA", "TRAFFIC ACCIDENT", "ROLLOVER"
- **Weather**: "TORNADO", "SEVERE WEATHER", "FLOOD", "STORM"

You can use the `keywords.json` file as a reference when creating keyword lists in the admin interface.

---

## System Administration

### System Admin Role

System admins have elevated permissions to:
- View all system alerts
- Create manual system alerts
- Receive push notifications for system health issues

#### Making a User a System Admin

```sql
UPDATE "users" SET "systemAdmin" = true WHERE "email" = 'admin@example.com';
```

### System Alerts

System alerts provide monitoring and alerting for system health issues.

#### Alert Types

- `transcription_failure` - Transcription service issues
- `tone_detection_issue` - Tone detection problems
- `service_health` - General service health issues
- `manual` - Manually created by system admins

#### Severity Levels

- `info` ℹ️ - Informational messages
- `warning` ⚠️ - Warnings that need attention
- `error` ❌ - Errors that affect functionality
- `critical` 🚨 - Critical issues requiring immediate attention

#### Automated Health Monitoring

Runs every hour:
1. **Transcription Failure Monitoring**
   - Checks for failed transcriptions in last 24 hours
   - Alerts if ≥10 failures detected

2. **Tone Detection Monitoring**
   - Checks talkgroups with tone detection enabled
   - Alerts if ≥5 calls received but no tones detected in 24 hours

#### API Endpoints

**GET /api/system-alerts**
- Get system alerts (system admins only)
- Query parameters:
  - `limit` (default: 50, max: 500)
  - `includeDismissed` (true/false)

**POST /api/system-alerts**
- Create a manual system alert (system admins only)
- Body: `{ "title": "...", "message": "...", "severity": "info|warning|error|critical" }`

**PUT /api/system-alerts/:id/dismiss**
- Dismiss a system alert (system admins only)

#### Push Notifications

System admins automatically receive push notifications for all system alerts:
- Notifications sent to all registered devices
- Icon varies by severity
- Format includes alert type, severity, and message

---

## Advanced Configuration

ThinLine Radio includes many advanced features that can be configured through the web admin interface. These settings are managed via the Admin → Config menu.

### Radio Reference Integration

Enable automatic talkgroup and system import from Radio Reference:

- **Enable Radio Reference**: Toggle to enable/disable integration
- **Username**: Your Radio Reference username
- **Password**: Your Radio Reference password
- **API Key**: Your Radio Reference API key (if available)

**Note:** Radio Reference integration requires a valid Radio Reference account.

### User Registration

Configure user registration and access control:

- **User Registration Enabled**: Allow users to create accounts
- **Public Registration Enabled**: Allow public (unauthenticated) registration
- **Public Registration Mode**: 
  - `codes` - Require registration codes
  - `email` - Require email verification
  - `both` - Require both codes and email verification

### Email Services

Configure email delivery for user verification, password resets, and notifications.

#### Email Providers

ThinLine Radio supports three email providers:

##### SendGrid

A cloud-based email delivery service with high deliverability.

**Configuration:**
- **API Key**: Get from [SendGrid Dashboard](https://app.sendgrid.com/settings/api_keys)
- **From Email**: Sender email address (e.g., `noreply@yourdomain.com`)
- **From Name**: Display name for sent emails (e.g., `ThinLine Radio`)

##### Mailgun

A transactional email service with flexible routing.

**Configuration:**
- **API Key**: Get from [Mailgun Dashboard](https://app.mailgun.com/app/account/security/api_keys)
- **Domain**: Your verified sending domain in Mailgun (e.g., `mg.yourdomain.com`)
- **API Base URL**: 
  - US Region: `https://api.mailgun.net`
  - EU Region: `https://api.eu.mailgun.net`
- **From Email**: Sender email address
- **From Name**: Display name for sent emails

##### SMTP (Direct Mail Server)

Connect directly to any SMTP server (Gmail, Outlook, Office 365, or custom mail servers).

**Configuration:**
- **SMTP Host**: Server hostname or IP address (e.g., `smtp.gmail.com`, `smtp.office365.com`)
- **SMTP Port**: Common ports:
  - `25` - Unencrypted (not recommended)
  - `465` - Implicit SSL/TLS (recommended for secure connections)
  - `587` - STARTTLS (recommended for most servers)
- **SMTP Username**: Authentication username (often your email address)
- **SMTP Password**: Authentication password or app-specific password
- **Use TLS/SSL**: Enable for encrypted connections (recommended)
- **Skip Certificate Verification**: Enable only for self-signed certificates (not recommended for production)
- **From Email**: Sender email address
- **From Name**: Display name for sent emails

**Popular SMTP Server Examples:**

*Gmail:*
- Host: `smtp.gmail.com`
- Port: `587` (STARTTLS) or `465` (SSL/TLS)
- Username: Your full Gmail address
- Password: App-specific password (enable 2FA and generate at [myaccount.google.com/apppasswords](https://myaccount.google.com/apppasswords))
- Use TLS: Enabled

*Office 365:*
- Host: `smtp.office365.com`
- Port: `587` (STARTTLS)
- Username: Your full email address
- Password: Your account password
- Use TLS: Enabled

*Outlook.com:*
- Host: `smtp-mail.outlook.com`
- Port: `587` (STARTTLS)
- Username: Your full Outlook.com address
- Password: Your account password
- Use TLS: Enabled

#### Email Logo (Optional)

Upload a custom logo to include in verification emails:
- Supported formats: PNG, JPG, SVG
- Recommended size: 200-400px wide
- **Border Radius**: CSS value for logo rounding (e.g., `0px` for square, `8px` for rounded, `50%` for circle)

#### Testing Email Configuration

Use the "Send Test Email" button in the admin panel to verify your email settings are working correctly before users attempt registration.

**Common Email Settings:**
- **From Email**: Sender email address
- **From Name**: Sender display name
- **Logo Filename**: Custom logo for email templates
- **Logo Border Radius**: CSS border radius for email logo

### Stripe Paywall

Enable subscription-based access control:

- **Stripe Paywall Enabled**: Enable/disable subscription requirements
- **Publishable Key**: Stripe publishable API key
- **Secret Key**: Stripe secret API key
- **Webhook Secret**: Stripe webhook signing secret
- **Price ID**: Stripe price ID for subscriptions
- **Grace Period Days**: Days before subscription expiration to restrict access

**Note:** Requires a Stripe account and proper webhook configuration.

### Cloudflare Turnstile

Enable Cloudflare Turnstile for bot protection on registration and login:

- **Turnstile Enabled**: Enable/disable Turnstile verification
- **Site Key**: Cloudflare Turnstile site key
- **Secret Key**: Cloudflare Turnstile secret key

### Config Sync

Enable configuration synchronization across multiple instances:

- **Config Sync Enabled**: Enable/disable config sync
- **Config Sync Path**: File system path for shared configuration

### Relay Server

Configure relay server for multi-instance deployments:

- **Relay Server URL**: URL of the relay server
- **Relay Server API Key**: API key for relay server authentication

### Other Advanced Options

Additional configuration options available in Admin → Config:

- **Branding**: Custom branding text
- **Max Clients**: Maximum concurrent client connections
- **Prune Days**: Days to retain audio files before deletion
- **Default System Delay**: Default delay for new systems
- **Audio Conversion**: Audio format conversion settings
- **Duplicate Detection**: Enable/disable duplicate call detection
- **Playback Goes Live**: Auto-switch to live feed during playback
- **Show Listeners Count**: Display active listener count
- **Time Format**: 12-hour or 24-hour time format
- **Alert Retention Days**: Days to retain keyword alerts

For detailed information on these options, see the Admin → Config interface in the web dashboard.

---

## Troubleshooting

### Service Won't Start

**Check the logs:**
```bash
# Linux/macOS
sudo journalctl -u thinline-radio -n 50

# Windows
# Check Event Viewer or service logs
```

**Common issues:**
- Database connection errors: Verify database credentials in `thinline-radio.ini`
- Port already in use: Change the port in `thinline-radio.ini` or stop the conflicting service
- Permission errors: Ensure the service user has read/write access to installation directory

### Database Connection Issues

**PostgreSQL:**
```bash
# Linux/macOS
sudo -u postgres psql -h localhost -U thinline_user -d thinline_radio

# Windows
cd "C:\Program Files\PostgreSQL\<version>\bin"
psql -U thinline_user -d thinline_radio
```

### Transcription Not Working

1. **Verify transcription is enabled** in admin → Config → Transcription Settings
2. **Check provider configuration** (API keys, region codes, etc.)
3. **Check server logs** for transcription errors
4. **Verify talkgroup has transcription enabled** in Config → Systems
5. **Check worker pool size** - may need adjustment based on traffic

### FFmpeg Issues

**Verify installation:**
```bash
ffmpeg -version
```

**Install if missing:**
- macOS: `brew install ffmpeg`
- Linux: `sudo apt install ffmpeg` or `sudo dnf install ffmpeg`
- Windows: Download from https://ffmpeg.org/download.html

### Rate Limiting (Transcription)

If you see HTTP 429 errors:
1. **Reduce worker pool size** immediately
2. **Check provider metrics** (Google Cloud Console or Azure Portal)
3. **Wait for rate limit window to reset** (1-2 minutes)
4. **Consider upgrading tier** if consistently hitting limits

---

## Additional Resources

- [Main README](../README.md)
- [API Documentation](api.md)
- [Web App Guide](webapp.md)
- [FAQ](faq.md)
- [Platform-Specific Guides](platforms/)
- [Update from v6 Guide](update-from-v6.md)

---

## Support

For issues or questions:
- Check the main README.md
- Review server logs
- Check the project documentation in the `docs/` directory
- Visit the project repository: https://github.com/Thinline-Dynamic-Solutions/ThinLineRadio
