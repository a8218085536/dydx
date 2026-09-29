import os
import socket
import sys

import paramiko


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


def run(client, command):
    _, stdout, stderr = client.exec_command(command, timeout=30)
    out = stdout.read().decode("utf-8", errors="replace").strip()
    err = stderr.read().decode("utf-8", errors="replace").strip()
    code = stdout.channel.recv_exit_status()
    print(f"$ {command}")
    print(f"exit={code}")
    if out:
        print(out)
    if err:
        print(err, file=sys.stderr)
    print()
    return code


def main():
    root = os.path.abspath(os.path.join(os.path.dirname(__file__), ".."))
    cfg = read_env(os.path.join(root, "server-connection.local.env"))
    remote_dir = cfg.get("REMOTE_DIR") or "/data/dydx"
    client, credential_label = connect(cfg)
    try:
        print(f"connected_with={credential_label}")
        commands = [
            "whoami",
            "uname -a",
            "cat /etc/os-release | sed -n '1,8p'",
            "pwd",
            "command -v go && go version || true",
            "command -v gcc && gcc --version | head -n 1 || true",
            "command -v g++ && g++ --version | head -n 1 || true",
            "command -v make && make --version | head -n 1 || true",
            "command -v git && git --version || true",
            "command -v protoc && protoc --version || true",
            "command -v docker && docker --version || true",
            f"test -d {shell_quote(remote_dir)} && echo remote_dir_exists || echo remote_dir_missing",
            f"test -w {shell_quote(remote_dir)} && echo remote_dir_writable || echo remote_dir_not_writable",
            "df -h . /data 2>/dev/null || df -h .",
            "free -h",
        ]
        failures = 0
        for command in commands:
            code = run(client, command)
            if code != 0:
                failures += 1
        if failures:
            raise SystemExit(1)
    finally:
        client.close()


def shell_quote(value):
    return "'" + value.replace("'", "'\"'\"'") + "'"


if __name__ == "__main__":
    main()
