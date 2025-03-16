#!/bin/bash
set -e

VERSION=$(git describe --tags --always --dirty)

go build -ldflags="-X 'main.Version=${VERSION}'" -o ./bin/solarplant ./main.go