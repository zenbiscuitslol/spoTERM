#!/bin/sh
# Builds spoTERM into ~/.local/bin so you can just type `spoTERM`.
set -e
cd "$(dirname "$0")"
mkdir -p ~/.local/bin
go build -ldflags "-X main.srcDir=$PWD" -o ~/.local/bin/spoTERM .
echo "Installed ~/.local/bin/spoTERM (make sure ~/.local/bin is on your PATH)"
