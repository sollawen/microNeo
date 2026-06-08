#!/bin/sh

set -e

VERSION="$1"
if [ -z "$VERSION" ]; then
	VERSION="$(go run tools/build-version.go)"
fi

mkdir -p binaries
mkdir -p microneo-$VERSION

cp LICENSE microneo-$VERSION
cp README.md microneo-$VERSION
cp LICENSE-THIRD-PARTY microneo-$VERSION
cp assets/packaging/micro.1 microneo-$VERSION
cp assets/packaging/micro.desktop microneo-$VERSION
cp assets/micro-logo-mark.svg microneo-$VERSION/microneo.svg

create_artefact_generic()
{
	mv micro microneo-$VERSION/
	tar -czf microneo-$VERSION-$1.tar.gz microneo-$VERSION
	sha256sum microneo-$VERSION-$1.tar.gz > microneo-$VERSION-$1.tar.gz.sha
	mv microneo-$VERSION-$1.* binaries
	rm microneo-$VERSION/micro
}

create_artefact_windows()
{
	mv micro.exe microneo-$VERSION/
	zip -r -q -T microneo-$VERSION-$1.zip microneo-$VERSION
	sha256sum microneo-$VERSION-$1.zip > microneo-$VERSION-$1.zip.sha
	mv microneo-$VERSION-$1.* binaries
	rm microneo-$VERSION/micro.exe
}

detect_os() {
	case "$(uname -s)" in
		Linux) echo "linux" ;;
		Darwin) echo "darwin" ;;
		MINGW*|MSYS*|CYGWIN*) echo "windows" ;;
		*) echo "unknown" ;;
	esac
}

HOST_OS=$(detect_os)
echo "Detected host OS: $HOST_OS"

# Linux builds
if [ "$HOST_OS" = "linux" ]; then
	# Linux x64
	echo "Linux x64"
	GOOS=linux GOARCH=amd64 make build
	if ./tools/package-deb.sh $VERSION; then
		sha256sum microneo-$VERSION-amd64.deb > microneo-$VERSION-amd64.deb.sha
		mv microneo-$VERSION-amd64.* binaries
	fi
	create_artefact_generic "linux64"

	# Linux ARM64
	echo "Linux ARM64"
	GOOS=linux GOARCH=arm64 make build
	create_artefact_generic "linux-arm64"

	# Windows x64
	echo "Windows x64"
	GOOS=windows GOARCH=amd64 make build
	create_artefact_windows "win64"

	# Windows ARM64
	echo "Windows ARM64"
	GOOS=windows GOARCH=arm64 make build
	create_artefact_windows "win-arm64"
fi

# macOS builds
if [ "$HOST_OS" = "darwin" ]; then
	# macOS Intel
	echo "macOS Intel"
	GOOS=darwin GOARCH=amd64 make build
	create_artefact_generic "osx"

	# macOS ARM64
	echo "macOS ARM64"
	GOOS=darwin GOARCH=arm64 make build
	create_artefact_generic "macos-arm64"

	# Linux x64
	echo "Linux x64"
	GOOS=linux GOARCH=amd64 make build
	if ./tools/package-deb.sh $VERSION; then
		sha256sum microneo-$VERSION-amd64.deb > microneo-$VERSION-amd64.deb.sha
		mv microneo-$VERSION-amd64.* binaries
	fi
	create_artefact_generic "linux64"

	# Linux ARM64
	echo "Linux ARM64"
	GOOS=linux GOARCH=arm64 make build
	create_artefact_generic "linux-arm64"

	# Windows x64
	echo "Windows x64"
	GOOS=windows GOARCH=amd64 make build
	create_artefact_windows "win64"

	# Windows ARM64
	echo "Windows ARM64"
	GOOS=windows GOARCH=arm64 make build
	create_artefact_windows "win-arm64"
fi

# Windows builds
if [ "$HOST_OS" = "windows" ]; then
	# Windows x64
	echo "Windows x64"
	GOOS=windows GOARCH=amd64 make build
	create_artefact_windows "win64"

	# Windows ARM64
	echo "Windows ARM64"
	GOOS=windows GOARCH=arm64 make build
	create_artefact_windows "win-arm64"
fi

rm -rf microneo-$VERSION