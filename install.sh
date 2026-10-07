#!/bin/sh
# Builds spotiCLI into ~/.local/bin so you can just type `spotiCLI`.
set -e
cd "$(dirname "$0")"
mkdir -p ~/.local/bin
go build -ldflags "-X main.srcDir=$PWD" -o ~/.local/bin/spotiCLI .
echo "Installed ~/.local/bin/spotiCLI (make sure ~/.local/bin is on your PATH)"
