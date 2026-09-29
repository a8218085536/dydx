import os
import posixpath
import shlex
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
    kwargs = {
        "hostname": cfg.get("SSH_HOST"),
        "port": int(cfg.get("SSH_PORT") or "22"),
        "username": cfg.get("SSH_USER"),
        "timeout": 10,
        "banner_timeout": 10,
        "auth_timeout": 10,
        "look_for_keys": False,
        "allow_agent": False,
    }
    key_path = cfg.get("SSH_KEY_PATH") or None
    if key_path:
        client = paramiko.SSHClient()
        client.set_missing_host_key_policy(paramiko.AutoAddPolicy())
        client.connect(**{**kwargs, "key_filename": key_path})
        return client
    for label in ("SSH_PASSWORD", "SUDO_PASSWORD"):
        password = cfg.get(label) or None
        if not password:
            continue
        client = paramiko.SSHClient()
        client.set_missing_host_key_policy(paramiko.AutoAddPolicy())
        try:
            client.connect(**{**kwargs, "password": password})
            return client
        except paramiko.AuthenticationException:
            client.close()
            print(f"{label}=authentication_failed")
        except (paramiko.SSHException, socket.error):
            client.close()
            raise
    raise SystemExit("ssh auth failed")


def run(client, command):
    _, stdout, stderr = client.exec_command(command, timeout=60)
    out = stdout.read().decode("utf-8", errors="replace").strip()
    err = stderr.read().decode("utf-8", errors="replace").strip()
    code = stdout.channel.recv_exit_status()
    if out:
        print(out)
    if err:
        print(err, file=sys.stderr)
    if code != 0:
        raise SystemExit(code)


def mkdir_p_sftp(sftp, path):
    parts = []
    current = path
    while current not in ("", "/"):
        parts.append(current)
        current = posixpath.dirname(current)
    for item in reversed(parts):
        try:
            sftp.stat(item)
        except IOError:
            sftp.mkdir(item)


def upload_file(sftp, local_path, remote_path):
    mkdir_p_sftp(sftp, posixpath.dirname(remote_path))
    sftp.put(local_path, remote_path)
    print(f"uploaded={remote_path}")


def main():
    root = os.path.abspath(os.path.join(os.path.dirname(__file__), ".."))
    cfg = read_env(os.path.join(root, "server-connection.local.env"))
    remote_base = (cfg.get("REMOTE_DIR") or "/data/dydx").rstrip("/")
    local_deploy = os.path.join(root, "tyche", "deploy")
    remote_services = posixpath.join(remote_base, "services")

    client = connect(cfg)
    try:
        run(client, f"mkdir -p {shlex.quote(remote_services)} {shlex.quote(posixpath.join(remote_base, 'logs'))}")
        sftp = client.open_sftp()
        try:
            upload_file(sftp, os.path.join(local_deploy, "tyche.mock.toml"), posixpath.join(remote_services, "tyche.mock.toml"))
            upload_file(sftp, os.path.join(local_deploy, "tyche.dydx.single-node.toml"), posixpath.join(remote_services, "tyche.dydx.single-node.toml"))
            upload_file(sftp, os.path.join(local_deploy, "render-tyche-dydx-config.sh"), posixpath.join(remote_services, "render-tyche-dydx-config.sh"))
            upload_file(sftp, os.path.join(local_deploy, "start-tyche-mock.sh"), posixpath.join(remote_services, "start-tyche-mock.sh"))
            upload_file(sftp, os.path.join(local_deploy, "start-tyche-dydx.sh"), posixpath.join(remote_services, "start-tyche-dydx.sh"))
            upload_file(sftp, os.path.join(local_deploy, "systemd", "tyche-mock.service"), posixpath.join(remote_services, "tyche-mock.service"))
            upload_file(sftp, os.path.join(local_deploy, "systemd", "tyche-dydx.service"), posixpath.join(remote_services, "tyche-dydx.service"))
            upload_file(sftp, os.path.join(local_deploy, "systemd", "mock-dydx-sidecar.service"), posixpath.join(remote_services, "mock-dydx-sidecar.service"))
        finally:
            sftp.close()
        run(client, f"chmod +x {shlex.quote(posixpath.join(remote_services, 'start-tyche-mock.sh'))} {shlex.quote(posixpath.join(remote_services, 'start-tyche-dydx.sh'))} {shlex.quote(posixpath.join(remote_services, 'render-tyche-dydx-config.sh'))}")
        print("tyche_service_templates_installed=true")
    finally:
        client.close()


if __name__ == "__main__":
    main()
