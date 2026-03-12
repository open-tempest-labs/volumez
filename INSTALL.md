# Installation Guide

## Prerequisites

volumez requires macFUSE (on macOS) or FUSE (on Linux) to be installed.

### macOS

Install macFUSE:

```bash
brew install --cask macfuse
```

**Important**: After installing macFUSE, you must:
1. Go to System Settings > Privacy & Security
2. Allow the macFUSE kernel extension
3. Restart your computer

### Linux

Install FUSE:

```bash
# Ubuntu/Debian
sudo apt-get install fuse3 libfuse3-dev

# Fedora/RHEL
sudo dnf install fuse3 fuse3-devel

# Arch Linux
sudo pacman -S fuse3
```

## Installation Methods

### Option 1: Homebrew (macOS)

```bash
brew tap open-tempest-labs/tap
brew install volumez
```

This will automatically install all dependencies including macFUSE.

### Option 2: Pre-built Binaries

Download the latest release from the [releases page](https://github.com/open-tempest-labs/volumez/releases):

```bash
# macOS (ARM64)
curl -L -o volumez https://github.com/open-tempest-labs/volumez/releases/download/v0.1.0/volumez-darwin-arm64
chmod +x volumez
sudo mv volumez /usr/local/bin/

# macOS (AMD64)
curl -L -o volumez https://github.com/open-tempest-labs/volumez/releases/download/v0.1.0/volumez-darwin-amd64
chmod +x volumez
sudo mv volumez /usr/local/bin/

# Linux (AMD64)
curl -L -o volumez https://github.com/open-tempest-labs/volumez/releases/download/v0.1.0/volumez-linux-amd64
chmod +x volumez
sudo mv volumez /usr/local/bin/
```

### Option 3: Build from Source

#### Requirements
- Go 1.21 or later
- macFUSE (macOS) or FUSE (Linux)

#### Build Steps

```bash
# Clone the repository
git clone https://github.com/open-tempest-labs/volumez.git
cd volumez

# Build
go build -o volumez ./cmd/volumez

# Install (optional)
sudo mv volumez /usr/local/bin/
```

## Configuration

1. Copy the example configuration:
```bash
cp volumez.example.json volumez.json
```

2. Edit `volumez.json` with your backend configuration:
```json
{
  "debug": false,
  "mounts": [
    {
      "path": "/s3-data",
      "backend": "s3",
      "config": {
        "bucket": "your-bucket-name",
        "region": "us-east-1",
        "prefix": "data/"
      }
    }
  ]
}
```

## Usage

1. Create a mount point:
```bash
mkdir /tmp/mymount
```

2. Mount the filesystem:
```bash
volumez -config volumez.json -mount /tmp/mymount
```

3. Access your backends:
```bash
ls /tmp/mymount/s3-data/
```

4. Unmount (Ctrl+C in the terminal running volumez, or):
```bash
umount /tmp/mymount
```

## Troubleshooting

### macOS: "Operation not permitted" error

Make sure you've:
1. Installed macFUSE
2. Allowed the kernel extension in System Settings
3. Restarted your computer

### Linux: "fusermount: command not found"

Install the FUSE utilities:
```bash
sudo apt-get install fuse3  # Ubuntu/Debian
sudo dnf install fuse3      # Fedora/RHEL
```

### AWS Credentials

volumez uses the AWS SDK, which looks for credentials in:
1. Environment variables (`AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`)
2. AWS credentials file (`~/.aws/credentials`)
3. IAM role (if running on EC2)

See the [AWS SDK documentation](https://docs.aws.amazon.com/sdk-for-go/api/aws/session/) for more details.

## Next Steps

- See [QUICKSTART.md](QUICKSTART.md) for a quick start guide
- See [README.md](README.md) for full documentation
- See [ARCHITECTURE.md](ARCHITECTURE.md) for technical details
