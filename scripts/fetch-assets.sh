#!/usr/bin/env bash
# mediabunny is renamed .js: some servers send .mjs as application/octet-stream, refused as a module.
# Altcha "external" build: no bundled inline styles or blob workers, required by the CSP.
set -euo pipefail

FFMPEG_PACKAGE_VERSION="0.12.15"
FFMPEG_PACKAGE_SHA256="c8a23365fb39b46d3d1d9baa2e74b522d00ce5d57e8b20471ad2665eaad38e3e"
FFMPEG_CORE_VERSION="0.12.10"
FFMPEG_CORE_SHA256="d00089ce82e1bdf637ddbe42e0c3d41a1ba8cf4c9e825e7fa4d0bb970e844bd4"
MEDIABUNNY_VERSION="1.65.2"
MEDIABUNNY_SHA256="93bceb27fd807e06e8b6a0e9e02350bf69e1d70da12d685dd0b133b54c3b1ff6"
ALTCHA_VERSION="3.5.0"
ALTCHA_SHA256="55c563b7441c94b12ef2344636e06c84d66afd455db809735c13eab8253fbb38"
NPM_REGISTRY="https://registry.npmjs.org"

cd "$(dirname "$0")/.."
expectedVersions="{\"ffmpegWasm\":\"${FFMPEG_PACKAGE_VERSION}\",\"ffmpegCore\":\"${FFMPEG_CORE_VERSION}\",\"mediabunny\":\"${MEDIABUNNY_VERSION}\"}"

isInstalled() {
    [ -f web/static/vendor/ffmpeg/versions.json ] && [ "$(cat web/static/vendor/ffmpeg/versions.json)" = "${expectedVersions}" ] \
        && [ -f web/static/vendor/ffmpeg/ffmpeg-core.wasm ] && [ -f web/static/vendor/ffmpeg/mediabunny.min.js ] \
        && [ -f web/static/vendor/altcha/version.txt ] && [ "$(cat web/static/vendor/altcha/version.txt)" = "${ALTCHA_VERSION}" ]
}

if isInstalled; then
    echo "assets already installed (ffmpeg ${FFMPEG_PACKAGE_VERSION} / core ${FFMPEG_CORE_VERSION} / mediabunny ${MEDIABUNNY_VERSION} / altcha ${ALTCHA_VERSION})"
    exit 0
fi

workDirectory="web/static/.fetch-work"
rm -rf -- web/static/.fetch-work
mkdir -p web/static/.fetch-work
trap 'rm -rf -- web/static/.fetch-work' EXIT

fetch_package() {
    local registryPath="$1" archiveName="$2" expectedSha256="$3" extractionDirectory="$4"
    curl --fail --silent --show-error --location --output "${workDirectory}/${archiveName}" "${NPM_REGISTRY}/${registryPath}/-/${archiveName}"
    echo "${expectedSha256}  ${workDirectory}/${archiveName}" | sha256sum --check --status \
        || { echo "SHA-256 mismatch for ${archiveName}" >&2; exit 1; }
    mkdir -p "${workDirectory}/${extractionDirectory}"
    tar xzf "${workDirectory}/${archiveName}" -C "${workDirectory}/${extractionDirectory}"
}

fetch_package "@ffmpeg/ffmpeg" "ffmpeg-${FFMPEG_PACKAGE_VERSION}.tgz" "${FFMPEG_PACKAGE_SHA256}" ffmpeg
fetch_package "@ffmpeg/core" "core-${FFMPEG_CORE_VERSION}.tgz" "${FFMPEG_CORE_SHA256}" core
fetch_package "mediabunny" "mediabunny-${MEDIABUNNY_VERSION}.tgz" "${MEDIABUNNY_SHA256}" mediabunny
fetch_package "altcha" "altcha-${ALTCHA_VERSION}.tgz" "${ALTCHA_SHA256}" altcha

rm -rf -- web/static/vendor
mkdir -p web/static/vendor/ffmpeg web/static/vendor/altcha
cp "${workDirectory}"/ffmpeg/package/dist/esm/*.js web/static/vendor/ffmpeg/
cp "${workDirectory}/core/package/dist/esm/ffmpeg-core.js" "${workDirectory}/core/package/dist/esm/ffmpeg-core.wasm" web/static/vendor/ffmpeg/
cp "${workDirectory}/mediabunny/package/dist/bundles/mediabunny.min.mjs" web/static/vendor/ffmpeg/mediabunny.min.js
echo "${expectedVersions}" > web/static/vendor/ffmpeg/versions.json
cp "${workDirectory}/altcha/package/dist/external/altcha.min.js" "${workDirectory}/altcha/package/dist/external/altcha.css" web/static/vendor/altcha/
cp "${workDirectory}/altcha/package/dist/workers/sha.js" web/static/vendor/altcha/sha.js
echo "${ALTCHA_VERSION}" > web/static/vendor/altcha/version.txt
echo "assets installed in web/static/vendor"
