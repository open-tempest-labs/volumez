# Release Process

This document outlines the process for creating a new release of volumez.

## Prerequisites

- [GoReleaser](https://goreleaser.com/) installed (`brew install goreleaser`)
- [GitHub CLI](https://cli.github.com/) installed (`brew install gh`)
- Write access to the `open-tempest-labs/volumez` repository
- Write access to the `open-tempest-labs/homebrew-volumez` tap repository

## Release Steps

### 1. Update Version Information

If you have version-specific code or documentation that needs updating, do so now. The version is automatically injected by GoReleaser during build.

### 2. Create and Push a Git Tag

```bash
# Ensure you're on main and up to date
git checkout main
git pull

# Create a new tag (use semantic versioning: vMAJOR.MINOR.PATCH)
git tag -a v0.2.0 -m "Release v0.2.0"

# Push the tag to GitHub
git push origin v0.2.0
```

### 3. Run GoReleaser

GoReleaser will:
- Build binaries for all platforms (darwin/linux, amd64/arm64)
- Create archives with documentation files
- Generate checksums
- Create a GitHub release with release notes
- Automatically update the Homebrew tap with the new formula

```bash
# For a real release
goreleaser release --clean

# For a test run (doesn't publish anything)
goreleaser release --snapshot --clean
```

### 4. Verify the Release

1. **Check GitHub Release**: Visit https://github.com/open-tempest-labs/volumez/releases
   - Verify all binaries are attached
   - Review the generated release notes
   - Edit release notes if needed

2. **Check Homebrew Tap**: Visit https://github.com/open-tempest-labs/homebrew-volumez
   - Verify the formula was automatically updated with:
     - New version number
     - Updated SHA256 checksum
   - A pull request or commit should have been created automatically

3. **Test Homebrew Installation**:
   ```bash
   # Update your local tap
   brew update

   # Upgrade to the new version
   brew upgrade volumez

   # Verify the version
   volumez -version
   ```

### 5. Announce the Release (Optional)

- Update any project documentation that references specific versions
- Announce on relevant channels (Twitter, blog, etc.)

## What GoReleaser Does Automatically

Based on `.goreleaser.yml`, the release process automatically:

1. **Builds** cross-platform binaries:
   - `volumez_<version>_darwin_amd64.tar.gz`
   - `volumez_<version>_darwin_arm64.tar.gz`
   - `volumez_<version>_linux_amd64.tar.gz`
   - `volumez_<version>_linux_arm64.tar.gz`

2. **Includes** documentation in each archive:
   - LICENSE
   - README.md
   - QUICKSTART.md
   - INSTALL.md
   - volumez.example.json

3. **Creates** a GitHub release with:
   - Automatically generated changelog
   - All binary archives attached
   - Checksums file (checksums.txt)
   - Installation instructions in the release footer

4. **Updates** the Homebrew tap:
   - Creates a new formula version in `open-tempest-labs/homebrew-volumez`
   - Updates the version number and SHA256 checksum
   - Commits and pushes the update automatically

## Release Checklist

Before creating a release, ensure:

- [ ] All tests pass (`go test ./...`)
- [ ] Code is committed and pushed to main
- [ ] CHANGELOG or release notes are prepared (if maintaining separately)
- [ ] Version number follows semantic versioning
- [ ] You have required permissions for both repositories

After creating a release:

- [ ] GitHub release created successfully
- [ ] All binary assets are attached
- [ ] Homebrew formula updated in tap repository
- [ ] Homebrew installation tested and working
- [ ] Release notes reviewed and edited if needed

## Troubleshooting

### GoReleaser fails with authentication error

Ensure you're authenticated with GitHub:
```bash
gh auth login
```

### Homebrew formula not updated

Check the `brews` section in `.goreleaser.yml`. Ensure:
- Repository owner and name are correct
- You have write access to the tap repository
- GitHub token has appropriate permissions

Manually update if needed:
```bash
cd ~/Projects/homebrew-volumez

# Update the formula with new version and checksum
# Get the SHA256 from the GitHub release
curl -sL https://github.com/open-tempest-labs/volumez/archive/refs/tags/v0.2.0.tar.gz | shasum -a 256

# Edit Formula/volumez.rb with new version and sha256
# Then commit and push
git add Formula/volumez.rb
git commit -m "Update volumez to v0.2.0"
git push
```

### Binary won't run on macOS (Apple Silicon)

Ensure the `goos` and `goarch` in `.goreleaser.yml` include:
```yaml
goos:
  - darwin
goarch:
  - arm64
  - amd64
```

## Version Numbering Guidelines

Follow [semantic versioning](https://semver.org/):

- **MAJOR** (v1.0.0 → v2.0.0): Breaking changes, incompatible API changes
- **MINOR** (v1.0.0 → v1.1.0): New features, backwards compatible
- **PATCH** (v1.0.0 → v1.0.1): Bug fixes, backwards compatible

Pre-release versions can use suffixes:
- `v1.0.0-alpha.1` - Alpha release
- `v1.0.0-beta.1` - Beta release
- `v1.0.0-rc.1` - Release candidate

## Rollback Procedure

If a release has critical issues:

1. **Delete the GitHub release** (if not yet widely distributed):
   ```bash
   gh release delete v0.2.0
   git tag -d v0.2.0
   git push --delete origin v0.2.0
   ```

2. **Revert Homebrew formula** in the tap repository:
   ```bash
   cd ~/Projects/homebrew-volumez
   git revert HEAD
   git push
   ```

3. **Create a new patch release** with the fix:
   - Fix the issue
   - Create a new tag (e.g., v0.2.1)
   - Run GoReleaser again

## References

- [GoReleaser Documentation](https://goreleaser.com/intro/)
- [Homebrew Formula Cookbook](https://docs.brew.sh/Formula-Cookbook)
- [Semantic Versioning](https://semver.org/)
- [GitHub Releases](https://docs.github.com/en/repositories/releasing-projects-on-github)
