#!/usr/bin/env bash
# Build, checksum, publish and verify a docker-scanner release.
#
#   scripts/release.sh build   v0.3.9   clean build of every binary, plus dist/checksums.txt
#   scripts/release.sh publish v0.3.9   gh release create with those files, then verify
#   scripts/release.sh verify  v0.3.9   check the published assets against dist/checksums.txt
#
# build refuses a dirty tree and an existing tag, deletes every old binary
# first, and stops on the first failure, so checksums.txt can only describe
# binaries built from the committed source in this run.
set -euo pipefail

TARGETS=(linux/amd64 linux/arm64 darwin/arm64 darwin/amd64 windows/amd64)

die() { echo "release: $*" >&2; exit 1; }

sha256() {
  if command -v sha256sum >/dev/null; then sha256sum "$@"; else shasum -a 256 "$@"; fi
}

binary_name() {
  local os=${1%/*} arch=${1#*/}
  local name="docker-scanner-$os-$arch"
  [[ $os == windows ]] && name+=".exe"
  echo "$name"
}

[[ $# -eq 2 ]] || die "usage: $0 build|publish|verify vX.Y.Z"
cmd=$1 version=$2
[[ $version =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]] || die "version must look like v1.2.3, got $version"
cd "$(git rev-parse --show-toplevel)"

build() {
  git diff --quiet && git diff --cached --quiet || die "the working tree has uncommitted changes; commit them first"
  git fetch --quiet --tags
  ! git rev-parse -q --verify "refs/tags/$version" >/dev/null || die "tag $version already exists"

  go vet ./...
  go test ./...

  rm -f dist/docker-scanner-* dist/checksums.txt
  mkdir -p dist
  local target
  for target in "${TARGETS[@]}"; do
    echo "building $target"
    CGO_ENABLED=0 GOOS=${target%/*} GOARCH=${target#*/} \
      go build -trimpath -ldflags="-X=main.version=$version" -o "dist/$(binary_name "$target")" .
  done

  (cd dist && sha256 docker-scanner-* > checksums.txt && sha256 -c checksums.txt)
  [[ $(wc -l < dist/checksums.txt) -eq ${#TARGETS[@]} ]] || die "checksums.txt doesn't list ${#TARGETS[@]} binaries"

  local host
  host="dist/$(binary_name "$(go env GOOS)/$(go env GOARCH)")"
  if [[ -x $host ]]; then
    [[ $("$host" -version) == *"$version"* ]] || die "$host -version doesn't report $version"
  fi

  echo
  echo "Built $version from $(git rev-parse --short HEAD). Next:"
  echo "  git add dist/checksums.txt && git commit -m 'Update dist/checksums.txt to $version' && git push"
  echo "  $0 publish $version"
}

verify() {
  [[ -f dist/checksums.txt ]] || die "no dist/checksums.txt; run build first"
  local published
  published=$(gh release view "$version" --json assets --jq '.assets[] | "\(.digest | sub("^sha256:"; ""))  \(.name)"')
  local failed=0 line
  while read -r line; do
    grep -qxF "$line" <<<"$published" || { echo "MISMATCH: dist/checksums.txt has '$line', the release doesn't"; failed=1; }
  done < dist/checksums.txt
  local own
  own="$(sha256 dist/checksums.txt | cut -d' ' -f1)  checksums.txt"
  grep -qxF "$own" <<<"$published" || { echo "MISMATCH: the published checksums.txt isn't dist/checksums.txt"; failed=1; }
  [[ $failed -eq 0 ]] || die "$version's published assets don't match dist/checksums.txt"
  echo "verified: every $version asset matches dist/checksums.txt"
}

publish() {
  [[ -f dist/checksums.txt ]] || die "no dist/checksums.txt; run build first"
  (cd dist && sha256 -c checksums.txt) || die "dist/ no longer matches checksums.txt; rebuild"
  git diff --quiet HEAD -- dist/checksums.txt || die "commit dist/checksums.txt before publishing"
  [[ -z $(git log --oneline '@{upstream}..' 2>/dev/null) ]] || die "push your commits before publishing"
  gh release create "$version" --target "$(git rev-parse HEAD)" --title "$version" --generate-notes \
    dist/docker-scanner-* dist/checksums.txt
  verify
}

case $cmd in
  build) build ;;
  publish) publish ;;
  verify) verify ;;
  *) die "unknown command $cmd; use build, publish or verify" ;;
esac
