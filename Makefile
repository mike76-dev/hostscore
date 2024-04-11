# These variables get inserted into build/commit.go
GIT_REVISION=$(shell git rev-parse --short HEAD)
GIT_DIRTY=$(shell git diff-index --quiet HEAD -- || echo "✗-")
ifeq ("$(GIT_DIRTY)", "✗-")
	BUILD_TIME=$(shell date)
else
	BUILD_TIME=$(shell git show -s --format=%ci HEAD)
endif

ldflags= \
-X "github.com/mike76-dev/hostscore/internal/build.NodeBinaryName=hsd" \
-X "github.com/mike76-dev/hostscore/internal/build.NodeVersion=1.0.3" \
-X "github.com/mike76-dev/hostscore/internal/build.ClientBinaryName=hsc" \
-X "github.com/mike76-dev/hostscore/internal/build.ClientVersion=1.2.0" \
-X "github.com/mike76-dev/hostscore/internal/build.GitRevision=${GIT_DIRTY}${GIT_REVISION}" \
-X "github.com/mike76-dev/hostscore/internal/build.BuildTime=${BUILD_TIME}"

# all will build and install release binaries
all: release

# pkgs changes which packages the makefile calls operate on.
pkgs = \
	./api \
	./cmd/hsc \
	./cmd/hsd \
	./external \
	./hostdb \
	./persist \
	./rhp \
	./wallet

# release-pkgs determine which packages are built for release and distribution
# when running a 'make release' command.
release-pkgs = ./cmd/hsc ./cmd/hsd

# lockcheckpkgs are the packages that are checked for locking violations.
lockcheckpkgs = \
	./cmd/hsc \
	./cmd/hsd

# dependencies list all packages needed to run make commands used to build
# and lint hsc/hsd locally and in CI systems.
dependencies:
	go get -d ./...

# fmt calls go fmt on all packages.
fmt:
	gofmt -s -l -w $(pkgs)

# vet calls go vet on all packages.
# NOTE: go vet requires packages to be built in order to obtain type info.
vet:
	go vet $(pkgs)

static:
	go build -trimpath -o release/ -tags='netgo' -ldflags='-s -w $(ldflags)' $(release-pkgs)

# release builds and installs release binaries.
release:
	go install -tags='netgo' -ldflags='-s -w $(ldflags)' $(release-pkgs)

# clean removes all directories that get automatically created during
# development.
clean:
ifneq ("$(OS)","Windows_NT")
# Linux
	rm -rf release
else
# Windows
	- DEL /F /Q release
endif

.PHONY: all fmt install release clean

