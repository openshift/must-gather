#!/usr/bin/env bash

set -eu -o pipefail

# constants
PROJECT_ROOT="$(git rev-parse --show-toplevel)"
GOOS="$(go env GOOS)"
GOARCH="$(go env GOARCH)"

declare -r PROJECT_ROOT GOOS GOARCH

declare -r LOCAL_BIN="$PROJECT_ROOT/tmp/bin"

# versions
declare -r SHFMT_VERSION=${SHFMT_VERSION:-v3.12.0}
declare -r SHELLCHECK_VERSION=${SHELLCHECK_VERSION:-v0.11.0}

# install
declare -r SHFMT_INSTALL_URL="https://github.com/mvdan/sh/releases/download/v3.12.0/shfmt_${SHFMT_VERSION}_${GOOS}_${GOARCH}"
declare -r SHELLCHECK_INSTALL_URL="https://github.com/koalaman/shellcheck/releases/download/${SHELLCHECK_VERSION}"

source "$PROJECT_ROOT/hack/utils.bash"

validate_version() {
	local cmd="$1"
	local version_arg="$2"
	local version_regex="$3"
	shift 3

	command -v "$cmd" >/dev/null 2>&1 || return 1

	[[ "$(eval "$cmd $version_arg" | grep -o "$version_regex")" =~ $version_regex ]] || {
		return 1
	}

	ok "$cmd matching $version_regex already installed"
}

install_shfmt() {

	local version_regex="${SHFMT_VERSION}"

	validate_version shfmt --version "$version_regex" && return 0

	local install_url="$SHFMT_INSTALL_URL"
	curl -sSL "$install_url" -o "$LOCAL_BIN/shfmt" || {
		fail "failed to install shfmt"
		return 1
	}
	chmod +x "$LOCAL_BIN/shfmt"
	ok "shfmt was installed successfully"
}

install_shellcheck() {
	local version_regex="version: ${SHELLCHECK_VERSION}"
	validate_version shellcheck --version "$version_regex" && return 0

	info "installing shellcheck version: $SHELLCHECK_VERSION"

	local arch=""
	case "$GOARCH" in
	amd64) arch="x86_64" ;;
	arm64) arch="aarch64" ;;
	*) fail "unsupported arch: $GOARCH" ;;
	esac

	local shellcheck_tar="shellcheck-${SHELLCHECK_VERSION}.${GOOS}.${arch}.tar.gz"
	local install_url="$SHELLCHECK_INSTALL_URL/$shellcheck_tar"

	local shellcheck_tmp="$LOCAL_BIN/tmp-shellcheck"
	mkdir -p "$shellcheck_tmp"

	curl -sSL "$install_url" | tar -xzf - -C "$shellcheck_tmp" || {
		fail "failed to install shellcheck"
		return 1
	}

	mv "$shellcheck_tmp/shellcheck-${SHELLCHECK_VERSION}/shellcheck" "$LOCAL_BIN/"
	rm -rf "$shellcheck_tmp"

	ok "shellcheck was installed successfully"
}

version_shfmt() {
	shfmt --version
}

version_shellcheck() {
	shellcheck --version
}

install_all() {
	info "installing all tools ..."
	local ret=0
	for tool in $(declare -F | cut -f3 -d ' ' | grep install_ | grep -v 'install_all'); do
		"$tool" || ret=1
	done
	return $ret
}

version_all() {

	header "Versions"
	for version_tool in $(declare -F | cut -f3 -d ' ' | grep version_ | grep -v 'version_all'); do
		local tool="${version_tool#version_}"
		local location=""
		location="$(command -v "$tool")"
		info "$tool -> $location"
		"$version_tool"
		echo
	done
	line "50"
}

main() {
	local op="${1:-all}"
	shift || true

	mkdir -p "$LOCAL_BIN"
	export PATH="$LOCAL_BIN:$PATH"

	# NOTE: skip installation if invocation is tools.sh version
	if [[ "$op" == "version" ]]; then
		version_all
		return $?
	fi

	install_"$op"
	version_"$op"
}

main "$@"
