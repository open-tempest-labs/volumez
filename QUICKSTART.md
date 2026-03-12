# Volumez Quick Start Guide

Get started with Volumez in 5 minutes!

## What You'll Build

Mount an S3 bucket as a local filesystem and perform file operations using standard tools.

## Prerequisites

- Go 1.21+
- AWS account with S3 access
- AWS credentials configured (`aws configure`)
- macFUSE installed (macOS) or libfuse-dev (Linux)

## Step 1: Build Volumez

```bash
cd volumez
go build -o volumez ./cmd/volumez
```

**Note**: On macOS, you may see build warnings about FUSE - these are expected. Ensure macFUSE is installed first.

## Step 2: Generate Configuration

```bash
./volumez -gen-config
```

This creates `volumez.example.json`.

## Step 3: Create Your Configuration

Create `volumez.json`:

```json
{
  "mounts": [
    {
      "path": "/mybucket",
      "backend": "s3",
      "config": {
        "bucket": "your-bucket-name",
        "region": "us-east-1",
        "prefix": ""
      }
    }
  ],
  "cache": {
    "enabled": false,
    "max_size": 1073741824,
    "ttl": 300,
    "metadata_ttl": 60
  },
  "debug": false
}
```

Replace `your-bucket-name` with your actual S3 bucket.

## Step 4: Create Mount Point

```bash
mkdir -p /tmp/volumez-mount
```

## Step 5: Mount the Filesystem

```bash
./volumez -config volumez.json -mount /tmp/volumez-mount
```

You should see:
```
2024/01/01 12:00:00 Initializing backend s3 for path /mybucket
2024/01/01 12:00:00 Mounting filesystem at /tmp/volumez-mount
2024/01/01 12:00:00 Filesystem mounted successfully. Press Ctrl+C to unmount.
```

## Step 6: Use Your Filesystem!

Open a new terminal and try these commands:

### List files
```bash
ls -la /tmp/volumez-mount/mybucket/
```

### Read a file
```bash
cat /tmp/volumez-mount/mybucket/myfile.txt
```

### Write a file
```bash
echo "Hello from Volumez!" > /tmp/volumez-mount/mybucket/test.txt
```

### Copy files to S3
```bash
cp ~/Documents/report.pdf /tmp/volumez-mount/mybucket/
```

### Create directories
```bash
mkdir /tmp/volumez-mount/mybucket/new-folder
```

### Remove files
```bash
rm /tmp/volumez-mount/mybucket/old-file.txt
```

### Use with standard tools
```bash
# grep through S3 files
grep "error" /tmp/volumez-mount/mybucket/logs/*.log

# tar directly to S3
tar czf /tmp/volumez-mount/mybucket/backup.tar.gz ~/important-files/

# rsync to S3
rsync -av ~/photos/ /tmp/volumez-mount/mybucket/photos/
```

## Step 7: Unmount

Press `Ctrl+C` in the terminal running volumez, or:

```bash
umount /tmp/volumez-mount
```

## Common Issues

### "command not found: volumez"

Make sure you built the binary:
```bash
go build -o volumez ./cmd/volumez
./volumez -version
```

### "Failed to mount: operation not permitted"

On Linux, ensure FUSE is installed:
```bash
sudo apt-get install fuse libfuse-dev  # Debian/Ubuntu
sudo yum install fuse fuse-devel       # RHEL/CentOS
```

On macOS, install macFUSE:
```bash
brew install macfuse
```

### "NoCredentialProviders: no valid providers in chain"

Configure AWS credentials:
```bash
aws configure
```

Or set environment variables:
```bash
export AWS_ACCESS_KEY_ID=your_access_key
export AWS_SECRET_ACCESS_KEY=your_secret_key
export AWS_REGION=us-east-1
```

### "Failed to create backend: bucket is required"

Check your `volumez.json` has the correct bucket name in the config section.

## Next Steps

### Try Multiple Backends

Edit `volumez.json` to add multiple mount points:

```json
{
  "mounts": [
    {
      "path": "/s3-private",
      "backend": "s3",
      "config": {
        "bucket": "my-private-bucket",
        "region": "us-east-1"
      }
    },
    {
      "path": "/s3-public",
      "backend": "s3",
      "config": {
        "bucket": "my-public-bucket",
        "region": "us-west-2"
      }
    }
  ],
  "cache": {
    "enabled": false
  }
}
```

Now you can access both buckets:
```bash
ls /tmp/volumez-mount/s3-private/
ls /tmp/volumez-mount/s3-public/
```

### Enable Caching for Better Performance

```json
{
  "cache": {
    "enabled": true,
    "max_size": 2147483648,
    "ttl": 600,
    "metadata_ttl": 120
  }
}
```

### Use with HTTP REST API

```json
{
  "mounts": [
    {
      "path": "/api",
      "backend": "http",
      "config": {
        "base_url": "https://api.example.com/files",
        "headers": {
          "Authorization": "Bearer YOUR_API_TOKEN"
        },
        "timeout": 30
      }
    }
  ]
}
```

### Mount Read-Only

For safety in production:
```bash
./volumez -config volumez.json -mount /tmp/volumez-mount -read-only
```

### Enable Debug Mode

To troubleshoot issues:
```bash
./volumez -config volumez.json -mount /tmp/volumez-mount -debug
```

## Advanced Usage

### Background Mounting

Use a process manager like systemd (Linux) or launchd (macOS):

**systemd service** (`/etc/systemd/system/volumez.service`):
```ini
[Unit]
Description=Volumez FUSE Filesystem
After=network.target

[Service]
Type=simple
User=youruser
ExecStart=/usr/local/bin/volumez -config /etc/volumez/volumez.json -mount /mnt/volumez
Restart=on-failure

[Install]
WantedBy=multi-user.target
```

Enable and start:
```bash
sudo systemctl enable volumez
sudo systemctl start volumez
```

### Allow Other Users

```bash
./volumez -config volumez.json -mount /tmp/volumez-mount -allow-other
```

**Note**: Requires `user_allow_other` in `/etc/fuse.conf`

## Learn More

- Full documentation: [README.md](README.md)
- Architecture details: [ARCHITECTURE.md](ARCHITECTURE.md)
- Create custom backends: See `pkg/backend/` examples

## Getting Help

- Check the logs when running with `-debug`
- Review error messages carefully
- Ensure AWS credentials are valid
- Verify S3 bucket permissions
- Try with a simple test first

Happy mounting! 🚀
