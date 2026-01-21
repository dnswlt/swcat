#!/bin/bash
set -e

# 1. Check git state
if ! git diff-index --quiet HEAD --; then
    echo "Error: Git repository is dirty. Please commit or stash changes."
    exit 1
fi

# Verify we are at a tag
if ! VERSION=$(git describe --tags --exact-match HEAD 2>/dev/null); then
    echo "Error: Current commit is not exactly at a tag."
    exit 1
fi

echo "Building Windows release for version: $VERSION"

RELEASE_DIR="swcat-${VERSION}"
ZIP_FILE="swcat-${VERSION}-windows-amd64.zip"

# Clean up previous artifacts
rm -rf "$RELEASE_DIR" "$ZIP_FILE"

# 2. Prepare directory structure
mkdir -p "$RELEASE_DIR"

# 3. Build Web Assets
echo "Building web assets..."
make build-web

# 4. Build Windows binary
echo "Building swcat.exe..."
make build-windows
mv swcat.exe "${RELEASE_DIR}/"

# 5. Copy examples
echo "Copying examples..."
# Copy recursively
cp -r examples "${RELEASE_DIR}/"

# 6. Zip it
echo "Creating archive $ZIP_FILE..."
# Check if zip is available
if ! command -v zip &> /dev/null; then
    echo "Error: 'zip' command not found."
    exit 1
fi

zip -r -q "$ZIP_FILE" "$RELEASE_DIR" swcat-launcher.exe

# Cleanup
rm -rf "$RELEASE_DIR" swcat-launcher.exe

echo "Success! Created $ZIP_FILE"
