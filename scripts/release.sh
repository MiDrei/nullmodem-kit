#!/usr/bin/env bash
# Tags a new NullModem Kit version and moves the sibling checkouts of
# NullModem BBS and NullModem Reader (../bbs, ../reader) onto it.
#
#   scripts/release.sh v0.2.0
#
# The dependents' go.mod/go.sum are updated, built and tested, but not
# committed -- review, commit and push them yourself. Until they are,
# BBS's Docker build and Reader's releases keep using the old kit: both
# build with GOWORK=off against the version go.mod pins.
set -euo pipefail

MODULE=github.com/midrei/nullmodem-kit
DEPENDENTS=(bbs reader)
# Fetch the new tag straight from GitHub; the public proxy may not have
# seen it yet.
export GOPRIVATE=$MODULE

die() { echo "release: $*" >&2; exit 1; }

version=${1:-}
[[ $version =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.]+)?$ ]] || die "usage: $0 vX.Y.Z[-suffix]"

cd "$(dirname "$0")/.."
kit=$(pwd)

[[ -z $(git status --porcelain) ]] || die "working tree is not clean"
[[ $(git rev-parse --abbrev-ref HEAD) == main ]] || die "not on main"
git fetch -q origin
[[ $(git rev-parse HEAD) == $(git rev-parse origin/main) ]] || die "main is not in sync with origin/main"
! git rev-parse -q --verify "refs/tags/$version" >/dev/null || die "tag $version already exists locally"
[[ -z $(git ls-remote --tags origin "refs/tags/$version") ]] || die "tag $version already exists on origin"

GOWORK=off go vet ./...
GOWORK=off go test ./...

git tag -a "$version" -m "NullModem Kit $version"
git push -q origin "$version"
echo "release: tagged and pushed $MODULE $version"

for dep in "${DEPENDENTS[@]}"; do
	dir="$kit/../$dep"
	if [[ ! -f $dir/go.mod ]]; then
		echo "release: ../$dep not checked out -- update it later with: go get $MODULE@$version && go mod tidy"
		continue
	fi
	echo "release: moving ../$dep onto $version"
	(
		cd "$dir"
		GOWORK=off go get "$MODULE@$version"
		GOWORK=off go mod tidy
		GOWORK=off go build ./...
		GOWORK=off go test ./...
	) || die "../$dep does not build or test cleanly against $version -- its go.mod/go.sum are left modified"
done

echo
echo "release: done. Next, in each dependent:"
echo "  git diff go.mod go.sum && git commit -am \"Kit $version\" && git push"
