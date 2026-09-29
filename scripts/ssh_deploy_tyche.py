import argparse
import os
import posixpath
import shlex
import socket
import sys
import tarfile
import tempfile
from datetime import datetime

import paramiko


EXCLUDED_DIRS = {
    ".git",
    ".idea",
    ".vscode",
    "node_modules",
    "coverage",
}

EXCLUDED_SUFFIXES = {
    ".test",
    ".tmp",
    ".log",
}


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


def should_include(path, source_root):
    rel = os.path.relpath(path, source_root)
    if rel == ".":
        return True
    parts = set(rel.split(os.sep))
    if parts.intersection(EXCLUDED_DIRS):
        return False
    name = os.path.basename(path)
    return not any(name.endswith(suffix) for suffix in EXCLUDED_SUFFIXES)


def make_archive(source_root):
    fd, archive_path = tempfile.mkstemp(prefix="tyche-upload-", suffix=".tar.gz")
    os.close(fd)
    file_count = 0
    with tarfile.open(archive_path, "w:gz") as archive:
        for current_root, dirs, files in os.walk(source_root):
            dirs[:] = [name for name in dirs if should_include(os.path.join(current_root, name), source_root)]
            for name in files:
                full_path = os.path.join(current_root, name)
                if not should_include(full_path, source_root):
                    continue
                rel = os.path.relpath(full_path, source_root).replace(os.sep, "/")
                archive.add(full_path, arcname=rel, recursive=False)
                file_count += 1
    return archive_path, file_count


def run(client, command, timeout=120):
    _, stdout, stderr = client.exec_command(command, timeout=timeout)
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


def shell_join(command):
    return " ".join(shlex.quote(part) for part in command)


def bash(command):
    return shell_join(["bash", "-lc", command])


def deploy(client, local_archive, remote_base, release_name):
    remote_base = remote_base.rstrip("/")
    releases_dir = posixpath.join(remote_base, "releases")
    release_dir = posixpath.join(releases_dir, release_name)
    current_link = posixpath.join(remote_base, "tyche")
    next_link = posixpath.join(remote_base, ".tyche-next")
    remote_archive = posixpath.join("/tmp", release_name + ".tar.gz")

    sftp = client.open_sftp()
    try:
        print(f"uploading={remote_archive}")
        sftp.put(local_archive, remote_archive)
    finally:
        sftp.close()

    setup = (
        "set -euo pipefail\n"
        f"mkdir -p {shlex.quote(releases_dir)} {shlex.quote(release_dir)}\n"
        f"tar -xzf {shlex.quote(remote_archive)} -C {shlex.quote(release_dir)}\n"
        f"if [ -e {shlex.quote(current_link)} ] && [ ! -L {shlex.quote(current_link)} ]; then "
        f"echo 'refusing to replace non-symlink {current_link}' >&2; exit 1; fi\n"
        f"ln -sfn {shlex.quote(release_dir)} {shlex.quote(next_link)}\n"
        f"mv -Tf {shlex.quote(next_link)} {shlex.quote(current_link)}\n"
        f"rm -f {shlex.quote(remote_archive)}\n"
        f"cd {shlex.quote(current_link)}\n"
        "pwd\n"
        "ls -la | sed -n '1,20p'\n"
    )
    code, out, err = run(client, bash(setup), timeout=180)
    print_result("deploy tyche", code, out, err)
    if code != 0:
        raise SystemExit("failed to deploy tyche")
    return current_link


def run_tests(client, remote_tyche):
    env_check = "go version && go env GOOS GOARCH CGO_ENABLED CC CXX GOPATH GOMODCACHE"
    code, out, err = run(client, bash(env_check), timeout=60)
    print_result("remote go env", code, out, err)
    if code != 0:
        raise SystemExit("remote Go environment is not usable")

    command = f"cd {shlex.quote(remote_tyche)} && go test ./..."
    code, out, err = run(client, bash(command), timeout=900)
    print_result("tyche go test", code, out, err)
    if code != 0:
        raise SystemExit("tyche tests failed")


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--test", action="store_true", help="run go test ./... after deploy")
    args = parser.parse_args()

    root = os.path.abspath(os.path.join(os.path.dirname(__file__), ".."))
    source_root = os.path.join(root, "tyche")
    cfg = read_env(os.path.join(root, "server-connection.local.env"))
    remote_base = cfg.get("REMOTE_DIR") or "/data/dydx"
    release_name = "tyche-" + datetime.utcnow().strftime("%Y%m%d-%H%M%S")

    archive_path, file_count = make_archive(source_root)
    try:
        print(f"source={source_root}")
        print(f"archive_files={file_count}")
        client, credential_label = connect(cfg)
        try:
            print(f"connected_with={credential_label}")
            remote_tyche = deploy(client, archive_path, remote_base, release_name)
            if args.test:
                run_tests(client, remote_tyche)
            print("tyche_deployed=true")
        finally:
            client.close()
    finally:
        try:
            os.remove(archive_path)
        except OSError:
            pass


if __name__ == "__main__":
    main()
