# Release Process

This document describes how to create a new release of etcd-secret-reader.

## Prerequisites

- Write access to the repository
- Git configured with your GitHub credentials

## Release Steps

### 1. Update Version Information

Ensure all documentation references the correct version number.

### 2. Create and Push a Version Tag

The release process is triggered by pushing a version tag to GitHub. Tags should follow semantic versioning (e.g., `v1.0.0`, `v1.2.3`).

```bash
# Make sure you're on the main/master branch and up to date
git checkout main
git pull origin main

# Create a new version tag
VERSION=v1.0.0  # Change this to your desired version
git tag -a $VERSION -m "Release $VERSION"

# Push the tag to GitHub
git push origin $VERSION
```

### 3. GitHub Actions Workflow

Once the tag is pushed, the GitHub Actions workflow (`.github/workflows/release.yml`) will automatically:

1. Build binaries for multiple platforms:
   - Linux (amd64, arm64)
   - macOS/Darwin (amd64, arm64)
   - Windows (amd64)

2. Create archives:
   - `.tar.gz` for Linux and macOS
   - `.zip` for Windows

3. Generate SHA256 checksums for all archives

4. Create a GitHub Release with:
   - Automatically generated release notes
   - All binary archives attached
   - Checksum files attached

### 4. Monitor the Release

1. Go to the [Actions tab](../../actions) in your GitHub repository
2. Find the "Release Binaries" workflow run
3. Monitor the progress and ensure all jobs complete successfully

### 5. Edit Release Notes (Optional)

After the automated release is created:

1. Go to the [Releases page](../../releases)
2. Find your new release
3. Click "Edit release"
4. Add any additional notes, breaking changes, or upgrade instructions
5. Update the automatically generated changelog if needed

## Testing a Release Locally

Before creating an official release, you can test the build process locally:

```bash
# Build for all platforms
make build-all

# Or build for a specific platform
GOOS=linux GOARCH=amd64 go build -o etcd-secret-reader-linux-amd64 ./cmd/etcd-secret-reader
```

## Manual Trigger

The release workflow can also be triggered manually:

1. Go to the [Actions tab](../../actions)
2. Select "Release Binaries" workflow
3. Click "Run workflow"
4. Select the branch/tag and run

## Version Numbering

We follow [Semantic Versioning](https://semver.org/):

- **MAJOR** version (v**X**.0.0): Incompatible API changes
- **MINOR** version (v1.**X**.0): New functionality (backwards compatible)
- **PATCH** version (v1.0.**X**): Bug fixes (backwards compatible)

Examples:
- `v1.0.0`: Initial release
- `v1.0.1`: Bug fix
- `v1.1.0`: New feature
- `v2.0.0`: Breaking change

## Troubleshooting

### Build Fails

If the GitHub Actions build fails:
1. Check the Actions logs for errors
2. Test the build locally: `go build ./cmd/etcd-secret-reader`
3. Fix any issues and create a new tag

### Wrong Version in Binary

The version is injected at build time. Verify it with:
```bash
./etcd-secret-reader --version
```

If it shows the wrong version, check that the tag was created correctly.

## Rollback

If you need to delete a release:

```bash
# Delete the tag locally
git tag -d v1.0.0

# Delete the tag from GitHub
git push origin :refs/tags/v1.0.0
```

Then manually delete the GitHub Release from the [Releases page](../../releases).
