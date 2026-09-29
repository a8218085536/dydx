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


def main():
    root = os.path.abspath(os.path.join(os.path.dirname(__file__), ".."))
    cfg = read_env(os.path.join(root, "server-connection.local.env"))
    host = cfg.get("SSH_HOST")
    port = int(cfg.get("SSH_PORT") or "22")
    user = cfg.get("SSH_USER")
    password = cfg.get("SSH_PASSWORD") or None
    sudo_password = cfg.get("SUDO_PASSWORD") or None
    key_path = cfg.get("SSH_KEY_PATH") or None
    if not host or not user:
        raise SystemExit("missing SSH_HOST or SSH_USER")

    client = paramiko.SSHClient()
    client.set_missing_host_key_policy(paramiko.AutoAddPolicy())
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
    credential_label = "SSH_KEY_PATH"
    if key_path:
        kwargs["key_filename"] = key_path
        client.connect(**kwargs)
    else:
        client, credential_label = connect_with_passwords(kwargs, [
            ("SSH_PASSWORD", password),
            ("SUDO_PASSWORD", sudo_password),
        ])
    try:
        print(f"connected_with={credential_label}")
        for command in ("whoami", "uname -a", "pwd"):
            _, stdout, stderr = client.exec_command(command, timeout=15)
            out = stdout.read().decode("utf-8", errors="replace").strip()
            err = stderr.read().decode("utf-8", errors="replace").strip()
            code = stdout.channel.recv_exit_status()
            print(f"$ {command}")
            if out:
                print(out)
            if err:
                print(err, file=sys.stderr)
            if code != 0:
                raise SystemExit(code)
    finally:
        client.close()


def connect_with_passwords(kwargs, candidates):
    last_error = None
    for label, password in candidates:
        if not password:
            continue
        client = paramiko.SSHClient()
        client.set_missing_host_key_policy(paramiko.AutoAddPolicy())
        try:
            client.connect(**{**kwargs, "password": password})
            return client, label
        except paramiko.AuthenticationException as exc:
            last_error = exc
            client.close()
            print(f"{label}=authentication_failed")
        except (paramiko.SSHException, socket.error) as exc:
            client.close()
            raise exc
    if last_error is not None:
        raise SystemExit("all configured passwords failed authentication")
    raise SystemExit("no SSH password or SSH key configured")


if __name__ == "__main__":
    main()
