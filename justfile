# Metron development tasks. Run `just` to list them.

# Metron ships CGO-free so the binary has no runtime dependencies.
export CGO_ENABLED := "0"

platforms := "linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64"

# List the available recipes.
default:
    @just --list

# Format check, vet and tests. Run this before considering work done.
check: lint test

# Build bin/metron.
build:
    go build -trimpath -o bin/metron ./cmd/metron

# Run the tests.
test:
    go test ./...

# Run the tests under the race detector, which needs cgo.
race:
    CGO_ENABLED=1 go test -race ./...

# Fail on unformatted files, then vet.
lint:
    #!/usr/bin/env bash
    set -euo pipefail
    unformatted=$(gofmt -l .)
    if [ -n "$unformatted" ]; then
      echo "gofmt needed:"
      echo "$unformatted"
      exit 1
    fi
    go vet ./...

# Run every fuzz target briefly. For a longer session: just fuzz 5m
fuzz fuzztime="10s":
    #!/usr/bin/env bash
    set -euo pipefail
    for pkg in $(go list ./...); do
      for target in $(go test -list '^Fuzz' "$pkg" | grep '^Fuzz' || true); do
        echo "fuzz $pkg $target"
        go test -run '^$' -fuzz "^${target}\$" -fuzztime {{fuzztime}} "$pkg"
      done
    done

# Cross-compile CGO-free binaries into dist/.
cross:
    #!/usr/bin/env bash
    set -euo pipefail
    for p in {{platforms}}; do
      os=${p%/*}
      arch=${p#*/}
      ext=""
      if [ "$os" = windows ]; then ext=.exe; fi
      echo "build $os/$arch"
      GOOS=$os GOARCH=$arch go build -trimpath -o "dist/metron-$os-$arch$ext" ./cmd/metron
    done

# Remove build output.
clean:
    rm -rf bin dist
