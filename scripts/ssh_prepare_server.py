import os
import re
import socket
import sys

import paramiko


MIN_GO_VERSION = (1, 23, 3)


def read_env(path):
    values = {}
    with open(path, "r", encoding="utf-8") as handle:
        for raw in handle:
            line = raw.strip()
            if not line or line.startswith("#") or "=" not in line:
                continue
            key, value = line.split("=", 1)
            values[key.strip()] = value.strip()
    return values


def connect(cfg):
    host = cfg.get("SSH_HOST")
    port = int(cfg.get("SSH_PORT") or "22")
    user = cfg.get("SSH_USER")
    password = cfg.get("SSH_PASSWORD") or None
    sudo_password = cfg.get("SUDO_PASSWORD") or None
    key_path = cfg.get("SSH_KEY_PATH") or None
    if not host or not user:
        raise SystemExit("missing SSH_HOST or SSH_USER")

    kwargs = {
        "hostname": host,
        "port": port,
        "username": user,
        "timeout": 10,
        "banner_timeout": 10,
        "auth_timeout": 10,
        "look_for_keys": False,
        "allow_agent": False,
    }
    if key_path:
        client = paramiko.SSHClient()
        client.set_missing_host_key_policy(paramiko.AutoAddPolicy())
        client.connect(**{**kwargs, "key_filename": key_path})
        return client, "SSH_KEY_PATH"

    last_error = None
    for label, candidate in (("SSH_PASSWORD", password), ("SUDO_PASSWORD", sudo_password)):
        if not candidate:
            continue
        client = paramiko.SSHClient()
        client.set_missing_host_key_policy(paramiko.AutoAddPolicy())
        try:
            client.connect(**{**kwargs, "password": candidate})
            return client, label
        except paramiko.AuthenticationException as exc:
            last_error = exc
            client.close()
            print(f"{label}=authentication_failed")
        except (paramiko.SSHException, socket.error):
            client.close()
            raise

    if last_error is not None:
        raise SystemExit("all configured passwords failed authentication")
    raise SystemExit("no SSH password or SSH key configured")


def run(client, command, timeout=60, sudo_password=None):
    stdin, stdout, stderr = client.exec_command(command, get_pty=bool(sudo_password), timeout=timeout)
    if sudo_password:
        stdin.write(sudo_password + "\n")
        stdin.flush()
    out = stdout.read().decode("utf-8", errors="replace").strip()
    err = stderr.read().decode("utf-8", errors="replace").strip()
    code = stdout.channel.recv_exit_status()
    return code, out, err


def print_result(label, code, out, err):
    print(f"[{label}] exit={code}")
    if out:
        print(out)
    if err:
        print(err, file=sys.stderr)
    print()


def require_command(client, label, command):
    code, out, err = run(client, f"bash -lc {quote(command)}")
    print_result(label, code, out, err)
    return code == 0 and bool(out.strip())


def go_version_ok(output):
    match = re.search(r"go version go(\d+)\.(\d+)\.(\d+)", output)
    if not match:
        return False
    got = tuple(int(part) for part in match.groups())
    return got >= MIN_GO_VERSION


def quote(value):
    return "'" + value.replace("'", "'\"'\"'") + "'"


def create_remote_dir(client, remote_dir, user, sudo_password):
    quoted_dir = quote(remote_dir)
    code, out, err = run(client, f"bash -lc {quote(f'mkdir -p {quoted_dir} && test -w {quoted_dir} && echo created_as_user')}")
    print_result("create remote dir as user", code, out, err)
    if code == 0:
        return True

    if not sudo_password:
        print("[create remote dir with sudo] skipped: no SUDO_PASSWORD configured")
        return False

    command = (
        "sudo -S -p '' bash -lc "
        + quote(
            "mkdir -p "
            + quoted_dir
            + " && chown -R "
            + quote(user)
            + ":"
            + quote(user)
            + " "
            + quoted_dir
            + " && chmod 775 "
            + quoted_dir
            + " && echo created_with_sudo"
        )
    )
    code, out, err = run(client, command, sudo_password=sudo_password)
    print_result("create remote dir with sudo", code, out, err)
    return code == 0


def run_cgo_smoke(client):
    command = r"""
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
cat > "$tmp/main.go" <<'EOF'
package main

/*
#include <stdlib.h>
*/
import "C"

func main() {
	p := C.malloc(1)
	C.free(p)
}
EOF
CGO_ENABLED=1 go run "$tmp/main.go"
"""
    code, out, err = run(client, f"bash -lc {quote(command)}", timeout=120)
    print_result("cgo smoke", code, out, err)
    return code == 0


def main():
    root = os.path.abspath(os.path.join(os.path.dirname(__file__), ".."))
    cfg = read_env(os.path.join(root, "server-connection.local.env"))
    remote_dir = cfg.get("REMOTE_DIR") or "/data/dydx"
    sudo_password = cfg.get("SUDO_PASSWORD") or None
    user = cfg.get("SSH_USER") or ""

    client, credential_label = connect(cfg)
    try:
        print(f"connected_with={credential_label}")

        critical_ok = True
        for label, command in (
            ("whoami", "whoami"),
            ("os", "cat /etc/os-release | sed -n '1,5p'"),
            ("go path", "command -v go"),
            ("gcc path", "command -v gcc"),
            ("g++ path", "command -v g++"),
            ("make path", "command -v make"),
            ("git path", "command -v git"),
        ):
            critical_ok = require_command(client, label, command) and critical_ok

        code, out, err = run(client, "bash -lc 'go version'")
        print_result("go version", code, out, err)
        critical_ok = critical_ok and code == 0 and go_version_ok(out)

        code, out, err = run(client, "bash -lc 'go env GOOS GOARCH CGO_ENABLED CC CXX GOPATH GOMODCACHE'")
        print_result("go env", code, out, err)
        critical_ok = critical_ok and code == 0

        critical_ok = run_cgo_smoke(client) and critical_ok

        for label, command in (
            ("protoc optional", "command -v protoc && protoc --version"),
            ("docker optional", "command -v docker && docker --version"),
        ):
            code, out, err = run(client, f"bash -lc {quote(command + ' || true')}")
            print_result(label, code, out, err)

        if not critical_ok:
            raise SystemExit("critical environment checks failed; remote directory was not created")

        if not create_remote_dir(client, remote_dir, user, sudo_password):
            raise SystemExit("failed to create remote directory")

        code, out, err = run(client, f"bash -lc {quote('ls -ld ' + quote(remote_dir) + ' && test -w ' + quote(remote_dir) + ' && echo remote_dir_writable')}")
        print_result("remote dir final", code, out, err)
        if code != 0:
            raise SystemExit("remote directory is not writable")

        print("server_ready=true")
    finally:
        client.close()


if __name__ == "__main__":
    main()
