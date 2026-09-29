#!/usr/bin/env bash
set -u
set -o pipefail

FAILS=0
WARNINGS=0

if [ -t 1 ] && [ -z "${NO_COLOR:-}" ]; then
	RED="$(printf '\033[31m')"
	GREEN="$(printf '\033[32m')"
	YELLOW="$(printf '\033[33m')"
	BLUE="$(printf '\033[34m')"
	RESET="$(printf '\033[0m')"
else
	RED=""
	GREEN=""
	YELLOW=""
	BLUE=""
	RESET=""
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROTOCOL_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

pass() {
	printf "%s[PASS]%s %s\n" "$GREEN" "$RESET" "$1"
}

warn() {
	WARNINGS=$((WARNINGS + 1))
	printf "%s[WARN]%s %s\n" "$YELLOW" "$RESET" "$1"
}

fail() {
	FAILS=$((FAILS + 1))
	printf "%s[FAIL]%s %s\n" "$RED" "$RESET" "$1"
}

info() {
	printf "%s[INFO]%s %s\n" "$BLUE" "$RESET" "$1"
}

have_cmd() {
	command -v "$1" >/dev/null 2>&1
}

check_required_cmd() {
	if have_cmd "$1"; then
		pass "$1 found: $(command -v "$1")"
	else
		fail "$1 is required but was not found"
	fi
}

check_recommended_cmd() {
	if have_cmd "$1"; then
		pass "$1 found: $(command -v "$1")"
	else
		warn "$1 was not found; this is recommended for the normal Makefile flow"
	fi
}

check_file() {
	if [ -f "$PROTOCOL_ROOT/$1" ]; then
		pass "Found $1"
	else
		fail "Missing $1"
	fi
}

check_port() {
	port="$1"
	if have_cmd ss; then
		if ss -ltn 2>/dev/null | awk '{print $4}' | grep -Eq ":${port}$"; then
			warn "Port $port is already listening; this is fine only if the chain stack is already running"
		else
			pass "Port $port is free"
		fi
	elif have_cmd lsof; then
		if lsof -iTCP:"$port" -sTCP:LISTEN >/dev/null 2>&1; then
			warn "Port $port is already listening; this is fine only if the chain stack is already running"
		else
			pass "Port $port is free"
		fi
	else
		warn "Cannot check port $port because neither ss nor lsof is installed"
	fi
}

check_disk() {
	available_kb="$(df -Pk "$PROTOCOL_ROOT" 2>/dev/null | awk 'NR == 2 {print $4}')"
	if [ -z "$available_kb" ]; then
		warn "Could not check free disk space"
		return
	fi

	required_kb=$((20 * 1024 * 1024))
	if [ "$available_kb" -lt "$required_kb" ]; then
		warn "Free disk space is below 20 GB; Docker builds and chain data may fail"
	else
		pass "Free disk space is at least 20 GB"
	fi
}

check_memory() {
	if [ ! -r /proc/meminfo ]; then
		warn "Could not check memory on this system"
		return
	fi

	mem_kb="$(awk '/MemTotal/ {print $2}' /proc/meminfo)"
	recommended_kb=$((8 * 1024 * 1024))
	if [ "$mem_kb" -lt "$recommended_kb" ]; then
		warn "Memory is below 8 GB; the chain may run, but Docker builds can be slow or fail"
	else
		pass "Memory is at least 8 GB"
	fi
}

check_docker() {
	if ! have_cmd docker; then
		fail "docker is required but was not found"
		return
	fi

	pass "$(docker --version 2>/dev/null)"

	if docker info >/dev/null 2>&1; then
		pass "Docker daemon is reachable"
	else
		fail "Docker CLI exists, but the Docker daemon is not reachable"
		return
	fi

	if docker compose version >/dev/null 2>&1; then
		pass "$(docker compose version 2>/dev/null)"
	else
		fail "Docker Compose v2 plugin is required; expected: docker compose"
	fi

	if docker network inspect flydex_backend >/dev/null 2>&1; then
		pass "Docker network flydex_backend exists"
	else
		warn "Docker network flydex_backend does not exist; create it with: docker network create flydex_backend"
	fi

	if [ -f "$PROTOCOL_ROOT/docker-compose.mainnet-single.yml" ]; then
		if docker compose -f "$PROTOCOL_ROOT/docker-compose.mainnet-single.yml" config >/dev/null 2>&1; then
			pass "docker-compose.mainnet-single.yml parses successfully"
		else
			fail "docker-compose.mainnet-single.yml did not parse successfully"
		fi
	fi
}

check_chain_state() {
	state_dir="$PROTOCOL_ROOT/localnet-mainnet-single/dydxprotocol0"
	keys_dir="$PROTOCOL_ROOT/localnet-mainnet-single/keys"
	marker="$state_dir/.mainnet-single-initialized"

	if [ -f "$marker" ]; then
		warn "mainnet-single state already exists; restart with make mainnet-single-continue, not first-start"
		return
	fi

	if [ -d "$state_dir/config" ] || [ -d "$keys_dir/keyring-test" ]; then
		fail "Partial mainnet-single state exists without init marker; entrypoint will refuse to initialize"
	else
		pass "No partial mainnet-single state found"
	fi
}

check_git_context() {
	if have_cmd git; then
		pass "git found: $(command -v git)"
		if [ -d "$PROTOCOL_ROOT/.git" ]; then
			pass "protocol appears to include git metadata"
		else
			warn "protocol/.git is missing; Makefile may print git metadata warnings, but Docker build can still work"
		fi
	else
		warn "git was not found; Makefile version metadata may be empty"
	fi
}

print_next_steps() {
	printf "\nSuggested next commands:\n"
	if have_cmd docker && ! docker network inspect flydex_backend >/dev/null 2>&1; then
		printf "  docker network create flydex_backend\n"
	fi

	if have_cmd make; then
		printf "  cd %s\n" "$PROTOCOL_ROOT"
		printf "  make mainnet-single-start\n"
	else
		printf "  cd %s\n" "$PROTOCOL_ROOT"
		printf "  mkdir -p localnet-mainnet-single/dydxprotocol0\n"
		printf "  mkdir -p localnet-mainnet-single/keys\n"
		printf "  docker build . -t dydxprotocol-base --no-cache\n"
		printf "  docker build . -t local:dydxprotocol-mainnet-single -f testing/mainnet-single/Dockerfile --no-cache\n"
		printf "  docker compose -f docker-compose.mainnet-single.yml up\n"
	fi
}

printf "FlyDEX dYdX mainnet-single environment check\n"
printf "Protocol root: %s\n\n" "$PROTOCOL_ROOT"

info "Checking required project files"
check_file "Dockerfile"
check_file "Makefile"
check_file "go.mod"
check_file "docker-compose.mainnet-single.yml"
check_file "testing/genesis.sh"
check_file "testing/mainnet-single/Dockerfile"
check_file "testing/mainnet-single/entrypoint.sh"

printf "\n"
info "Checking system commands"
check_required_cmd bash
check_docker
check_recommended_cmd make
check_recommended_cmd gcc
check_recommended_cmd go
check_git_context

printf "\n"
info "Checking server capacity"
check_disk
check_memory

printf "\n"
info "Checking mainnet-single local state"
check_chain_state

printf "\n"
info "Checking ports used by the chain stack"
for port in 26656 26657 9090 9092 1317 8080 8002; do
	check_port "$port"
done

printf "\n"
info "Deployment notes"
warn "docker-compose.mainnet-single.yml publishes RPC/REST/gRPC/Slinky ports to the host; keep them private with firewall rules"
warn "DYDX_INDEXER_KAFKA_CONN_STR defaults to kafka:9092; make sure Kafka exists, or decide how to disable indexer publishing before long runs"

printf "\nSummary: %s failure(s), %s warning(s)\n" "$FAILS" "$WARNINGS"
if [ "$FAILS" -eq 0 ]; then
	pass "Environment looks ready enough to try mainnet-single startup"
	print_next_steps
	exit 0
fi

fail "Fix the failures above before starting the chain"
print_next_steps
exit 1
