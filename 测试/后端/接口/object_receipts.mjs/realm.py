#!/usr/bin/env python3
"""Create a distinct receipt realm using new databases on the owned test clusters."""

import argparse
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import secrets
import signal
import shutil
import socket
import ssl
import time
import urllib.parse
import urllib.request
import uuid


ROOT = Path(__file__).resolve().parents[4]
SPEC = importlib.util.spec_from_file_location("jobsim_owned_realm", Path(__file__).resolve().parents[1] / "personal_exports.py/realm.py")
OWNED = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(OWNED)


def save(path, value):
    temporary = path.with_suffix(path.suffix + ".writing")
    OWNED.private_json(temporary, value)
    with temporary.open("rb") as stream:
        os.fsync(stream.fileno())
    temporary.replace(path)
    directory = os.open(path.parent, os.O_RDONLY)
    try:
        os.fsync(directory)
    finally:
        os.close(directory)


def stop_owned(seed):
    environment = json.loads(seed.read_text())
    run = Path(environment["runDir"])
    base = Path.home() / "Library/Application Support/JobsIM/Integration"
    if not environment.get("freshObjectReceiptRealm") or environment.get("serverURL") != "https://127.0.0.1:56443" or run.name != "Safety" or not run.parent.name.startswith("object-receipt-") or run.resolve().parent.parent != base.resolve() or seed.resolve() != run.resolve() / "environment.json":
        raise RuntimeError("Owned realm identity guard rejected stop; nothing was signalled")
    if hashlib.sha256(Path(environment["binary"]).read_bytes()).hexdigest() != environment["binarySHA256"]:
        raise RuntimeError("Frozen binary changed; nothing was signalled")
    expected = [("serverPID", environment["serverCommand"]), ("natsPID", [environment["natsBinary"], "-c", environment["natsConfig"]])]
    live = []
    for key, command in expected:
        pid = environment.get(key)
        if not isinstance(pid, int) or pid <= 1:
            continue
        query = subprocess.run(["/bin/ps", "-p", str(pid), "-o", "command="], capture_output=True, text=True, timeout=5)
        if query.returncode != 0:
            continue
        if query.stdout.strip() != " ".join(map(str, command)):
            raise RuntimeError("Exact owned PID/argv mismatch; nothing was signalled")
        live.append((key, pid))
    for key, pid in live:
        os.kill(pid, signal.SIGTERM)
        for attempt in range(80):
            query = subprocess.run(["/bin/ps", "-p", str(pid), "-o", "stat="], capture_output=True, text=True, timeout=5)
            if not query.stdout.strip() or query.stdout.strip().startswith("Z"):
                break
            time.sleep(0.1)
        else:
            raise RuntimeError("Owned process did not exit; no force signal or database deletion was used")
        environment[key + "Stopped"] = True
        save(seed, environment)
    environment["status"] = "stopped"
    save(seed, environment)
    print(json.dumps({"status": "stopped", "databasesPreserved": True, "CAAndKeysPreserved": True, "sharedPostgreSQLStopped": False}))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--cluster-seed", type=Path)
    parser.add_argument("--binary", type=Path)
    parser.add_argument("--binary-sha256")
    parser.add_argument("--profile-file", type=Path)
    mode = parser.add_mutually_exclusive_group(required=True)
    mode.add_argument("--create-owned-realm", action="store_true")
    mode.add_argument("--stop-owned-realm", type=Path)
    args = parser.parse_args()
    if args.stop_owned_realm:
        stop_owned(args.stop_owned_realm)
        return
    if any(value is None for value in [args.cluster_seed, args.binary, args.binary_sha256, args.profile_file]):
        parser.error("Create requires --cluster-seed, --binary, --binary-sha256 and --profile-file")
    os.umask(0o077)
    tools = json.loads(args.cluster_seed.read_text())
    if not tools.get("cloudDraftRealm") or tools.get("serverURL") != "https://127.0.0.1:54443":
        raise RuntimeError("An explicitly owned cluster seed is required; no service was touched")
    if hashlib.sha256(args.binary.read_bytes()).hexdigest() != args.binary_sha256:
        raise RuntimeError("Frozen candidate digest mismatch")
    profiles = json.loads(args.profile_file.read_text())
    if not profiles.get("profiles") or any(p.get("storageContract") != "native_sqlite_full_file_fsync_v1" for p in profiles["profiles"]):
        raise RuntimeError("Explicit combined file and SQLite verification approval required")
    token = uuid.uuid4().hex
    run = Path.home() / "Library/Application Support/JobsIM/Integration" / ("object-receipt-" + token) / "Safety"
    run.mkdir(parents=True, mode=0o700)
    OWNED.free_space(run)
    for port in [56443, 57222, *range(57101, 57110)]:
        with socket.socket() as probe:
            probe.bind(("127.0.0.1", port))
    for name in ["Secrets", "Run", "TLS", "State", "NATS"]:
        (run / name).mkdir(mode=0o700)
    prefix = "jobsim_object_" + token[:16]
    environment = {"freshObjectReceiptRealm": True, "status": "preparing", "runDir": str(run),
                   "stateDir": str(run / "State"), "databasePrefix": prefix,
                   "mainPort": 62381, "journalPort": 62382, "serverURL": "https://127.0.0.1:56443",
                   "internalBasePort": 57101, "pgBin": tools["pgBin"], "natsBinary": tools["natsBinary"],
                   "pgPassFile": str(run / "Secrets/pgpass"), "binary": str(args.binary.resolve()),
                   "binarySHA256": args.binary_sha256, "caPath": str(run / "TLS/ca.pem"),
                   "tlsCert": str(run / "TLS/server.pem"), "tlsKey": str(run / "TLS/server.key"),
                   "natsURL": "nats://127.0.0.1:57222", "natsTokenFile": str(run / "Secrets/nats-token"),
                   "natsConfig": str(run / "NATS/nats.conf"), "receiptPolicy": str(run / "Secrets/receipt-policy.json")}
    shutil.copyfile(tools["pgPassFile"], environment["pgPassFile"])
    os.chmod(environment["pgPassFile"], 0o600)
    save(Path(environment["receiptPolicy"]), profiles)
    seed = run / "environment.json"
    save(seed, environment)
    identities = []
    for journal in [False, True]:
        label = "journal" if journal else "main"
        existing = urllib.parse.urlsplit(tools["journalURL" if journal else "businessURL"])
        admin = urllib.parse.urlsplit(tools[label + "AdminURL"])
        expected = 62382 if journal else 62381
        if any(url.hostname != "127.0.0.1" or url.port != expected or url.password or not url.username for url in [existing, admin]):
            raise RuntimeError("Owned cluster routing or credential boundary mismatch")
        database = prefix + ("_journal" if journal else "_business")
        environment[label + "Username"] = existing.username
        environment[label + "AdminUsername"] = admin.username
        environment[label + "AdminURL"] = urllib.parse.urlunsplit(admin._replace(path="/postgres"))
        environment["journalURL" if journal else "businessURL"] = urllib.parse.urlunsplit(existing._replace(path="/" + database))
        environment["journalDatabase" if journal else "businessDatabase"] = database
        save(seed, environment)
        if not all(c.isalnum() or c == "_" for c in existing.username):
            raise RuntimeError("Noncanonical owned database principal")
        OWNED.sql(environment, journal, f'CREATE DATABASE "{database}" OWNER "{existing.username}";', True)
        identity = OWNED.sql(environment, journal, "SELECT system_identifier::text FROM pg_control_system();", True)
        identities.append({"port": expected, "systemIdentifier": identity})
    if identities[0]["systemIdentifier"] == identities[1]["systemIdentifier"]:
        raise RuntimeError("Independent physical journal is required")
    environment["pgIdentities"] = identities
    openssl = Path("/opt/homebrew/opt/openssl@3/bin/openssl")
    tls = run / "TLS"
    commands = [
        ["genpkey", "-algorithm", "EC", "-pkeyopt", "ec_paramgen_curve:P-256", "-out", tls / "ca.key"],
        ["req", "-new", "-x509", "-key", tls / "ca.key", "-sha256", "-days", "30", "-subj", "/CN=JobsIM Object Receipt Private CA", "-addext", "basicConstraints=critical,CA:TRUE", "-addext", "keyUsage=critical,keyCertSign,cRLSign", "-addext", "subjectKeyIdentifier=hash", "-out", tls / "ca.pem"],
        ["genpkey", "-algorithm", "EC", "-pkeyopt", "ec_paramgen_curve:P-256", "-out", tls / "server.key"],
        ["req", "-new", "-key", tls / "server.key", "-subj", "/CN=localhost", "-out", tls / "server.csr"],
        ["x509", "-req", "-in", tls / "server.csr", "-CA", tls / "ca.pem", "-CAkey", tls / "ca.key", "-CAcreateserial", "-days", "14", "-sha256", "-extfile", tls / "server.ext", "-out", tls / "server.pem"],
    ]
    (tls / "server.ext").write_text("subjectAltName=DNS:localhost,IP:127.0.0.1\nbasicConstraints=critical,CA:FALSE\nkeyUsage=critical,digitalSignature\nextendedKeyUsage=serverAuth\nsubjectKeyIdentifier=hash\nauthorityKeyIdentifier=keyid,issuer\n")
    for index, command in enumerate(commands):
        OWNED.command([openssl, *command], run, run / "Run" / f"tls-{index}.log")
    nats_token = secrets.token_hex(32)
    Path(environment["natsTokenFile"]).write_text(nats_token + "\n")
    save(Path(environment["natsConfig"]), {"listen": "127.0.0.1:57222", "server_name": "jobsim-object-" + token[:12], "authorization": {"token": nats_token}, "jetstream": {"store_dir": str(run / "NATS/Store"), "max_memory_store": 16 << 20, "max_file_store": 256 << 20}})
    environment["natsPID"] = OWNED.start_process([environment["natsBinary"], "-c", environment["natsConfig"]], run, run / "Run/nats.log")
    # Role-specific inherited URLs must not redirect one owner into an older
    # business database while the new realm's common URL appears correct.
    child = {key: value for key, value in OWNED.clean_environment().items()
             if not (key.startswith("JOBS_IM_") and key.endswith("_DATABASE_URL"))}
    child.update({"JOBS_IM_DATABASE_URL": environment["businessURL"], "JOBS_IM_JOURNAL_DATABASE_URL": environment["journalURL"], "PGPASSFILE": environment["pgPassFile"], "JOBS_IM_NATS_URL": environment["natsURL"], "JOBS_IM_NATS_TOKEN_FILE": environment["natsTokenFile"], "JOBS_IM_PUBLIC_URL": environment["serverURL"], "GOMAXPROCS": "2"})
    server = [environment["binary"], "--service=all", "--addr", "127.0.0.1:56443", "--internal-base-port", "57101", "--state-dir", environment["stateDir"], "--tls-cert", environment["tlsCert"], "--tls-key", environment["tlsKey"], "--receipt-policy-file", environment["receiptPolicy"]]
    environment["serverCommand"] = server
    environment["serverPID"] = OWNED.start_process(server, run, run / "Run/server.log", child)
    save(seed, environment)
    for attempt in range(60):
        try:
            with urllib.request.urlopen(environment["serverURL"] + "/api/bootstrap", context=ssl.create_default_context(cafile=environment["caPath"]), timeout=2) as response:
                bootstrap = json.load(response)
            break
        except (OSError, ValueError):
            time.sleep(0.2)
    else:
        raise RuntimeError("Owned candidate failed readiness; preserve private startup logs and original new databases")
    environment.update({"status": "ready", "deploymentId": bootstrap["deploymentId"], "restoreEpoch": bootstrap["restoreEpoch"]})
    save(seed, environment)
    report = ROOT / "项目相关报告/测试/后端/20261116-完整对象收讫/独立HTTPS域"
    report.mkdir(parents=True, exist_ok=True)
    save(report / "启动结果.json", {"serverURL": environment["serverURL"], "deploymentId": environment["deploymentId"], "restoreEpoch": environment["restoreEpoch"], "binarySHA256": args.binary_sha256, "physicalClusters": identities, "bootstrap": bootstrap, "existingBusinessDatabasesTouched": False, "oldServicesRestarted": False, "serverProcess": "all: 9 owners and gateway / 10 listeners"})
    print(json.dumps({"environmentFile": str(seed), "status": "ready"}, ensure_ascii=False))


if __name__ == "__main__":
    main()
