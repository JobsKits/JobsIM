#!/usr/bin/env python3
"""Create an explicitly owned, fresh formal-export realm; never migrate a prior realm."""

import argparse
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import secrets
import shutil
import signal
import socket
import ssl
import subprocess
import time
import urllib.parse
import urllib.request
import uuid


REPOSITORY = Path(__file__).resolve().parents[4]
MINIMUM_FREE = 4 * 1024 ** 3


def private_json(path, value):
    path.parent.mkdir(parents=True, exist_ok=True, mode=0o700)
    with path.open("w", encoding="utf-8") as stream:
        os.chmod(path, 0o600)
        json.dump(value, stream, ensure_ascii=False, indent=2)
        stream.write("\n")


def free_space(path):
    if shutil.disk_usage(path).free < MINIMUM_FREE:
        raise RuntimeError("4GiB disk guard rejected this step")


def clean_environment():
    return {key: value for key, value in os.environ.items()
            if not key.startswith("PG") and key not in {
                "DATABASE_URL", "JOURNAL_DATABASE_URL", "JOBS_IM_DATABASE_URL",
                "JOBS_IM_JOURNAL_DATABASE_URL", "JOBS_IM_NATS_URL", "JOBS_IM_NATS_TOKEN_FILE",
                "JOBS_IM_PUBLIC_URL", "JOBS_IM_WEB_DIR"}}


def command(arguments, directory, log, environment=None, payload=None):
    free_space(directory)
    with log.open("wb") as output:
        os.chmod(log, 0o600)
        completed = subprocess.run([str(value) for value in arguments], cwd=directory,
                                   env=environment or clean_environment(), input=payload,
                                   stdout=output, stderr=subprocess.STDOUT, timeout=180)
    if completed.returncode:
        raise RuntimeError("Owned step failed; private log: " + str(log))


def sql(environment, journal, query, administrator=False):
    raw = environment["journalAdminURL" if journal and administrator else
                      "mainAdminURL" if administrator else
                      "journalURL" if journal else "businessURL"]
    parsed = urllib.parse.urlparse(raw)
    expected = environment["journalPort" if journal else "mainPort"]
    if parsed.hostname != "127.0.0.1" or parsed.port != expected:
        raise RuntimeError("Owned SQL routing guard rejected this connection")
    wanted_user = environment[("journal" if journal else "main") + ("AdminUsername" if administrator else "Username")]
    if parsed.username != wanted_user or parsed.password:
        raise RuntimeError("Owned SQL principal guard rejected this connection")
    if not parsed.path[1:].startswith(environment["databasePrefix"]) and parsed.path != "/postgres":
        raise RuntimeError("Owned SQL database prefix guard rejected this connection")
    child = clean_environment()
    child.update({"PGHOST": parsed.hostname, "PGPORT": str(parsed.port),
                  "PGUSER": urllib.parse.unquote(parsed.username), "PGDATABASE": parsed.path[1:],
                  "PGPASSFILE": environment["pgPassFile"], "PGCONNECTTIMEOUT": "5",
                  "PGAPPNAME": "jobsim-formal-owned-fixture", "PGOPTIONS": "-c statement_timeout=15000"})
    response = subprocess.run([str(Path(environment["pgBin"]) / "psql"), "-X", "-At", "-v", "ON_ERROR_STOP=1"],
                              input=query, env=child, text=True, stdout=subprocess.PIPE,
                              stderr=subprocess.PIPE, timeout=20)
    if response.returncode:
        private_json(Path(environment["runDir"]) / "Run/last-sql-failure.json",
                     {"stderr": response.stderr, "at": datetime.now(timezone.utc).isoformat()})
        raise RuntimeError("Owned SQL failed; diagnostics remain private")
    return response.stdout.strip()


def start_process(arguments, directory, log, environment=None):
    free_space(directory)
    with log.open("ab") as output:
        os.chmod(log, 0o600)
        process = subprocess.Popen([str(value) for value in arguments], cwd=directory,
                                   env=environment or clean_environment(), stdin=subprocess.DEVNULL,
                                   stdout=output, stderr=subprocess.STDOUT, start_new_session=True)
    return process.pid


def build(environment, gocache, report):
    run = Path(environment["runDir"])
    child = clean_environment()
    child.update({"GOMAXPROCS": "2", "GOTOOLCHAIN": "local", "GOPROXY": "off",
                  "GOSUMDB": "off", "CGO_ENABLED": "0", "GOCACHE": str(gocache)})
    source = REPOSITORY / "代码/后端/Go"
    inputs = {str(path.relative_to(REPOSITORY)): hashlib.sha256(path.read_bytes()).hexdigest()
              for path in sorted(source.rglob("*.go")) + [source / "go.mod", source / "go.sum"]}
    for name, target in [("jobsimd", "./cmd/jobsimd"), ("jobsimops", "./cmd/jobsimops")]:
        command([environment["go"], "build", "-p", "1", "-trimpath", "-o", run / "Bin" / name, target],
                source, run / "Run" / ("build-" + name + ".log"), child)
    if any(hashlib.sha256((REPOSITORY / name).read_bytes()).hexdigest() != digest for name, digest in inputs.items()):
        raise RuntimeError("Production source changed during owned build")
    environment.update({"binary": str(run / "Bin/jobsimd"), "opsBinary": str(run / "Bin/jobsimops"),
                        "binarySHA256": hashlib.sha256((run / "Bin/jobsimd").read_bytes()).hexdigest(),
                        "opsBinarySHA256": hashlib.sha256((run / "Bin/jobsimops").read_bytes()).hexdigest()})
    private_json(report / "生产构建.json", {"source": inputs, "binarySHA256": environment["binarySHA256"],
                                          "opsBinarySHA256": environment["opsBinarySHA256"],
                                          "apiRequests": 0, "sourceUnchanged": True})
    for name in ["jobsimd", "jobsimops"]:
        shutil.copyfile(run / "Run" / ("build-" + name + ".log"), report / ("构建-" + name + ".log"))


def restart_owned(seed, gocache, report, label, remove_arguments=()):
    environment = json.loads(Path(seed).read_text())
    if not environment.get("freshFormalExportRealm") or environment["serverURL"] != "https://127.0.0.1:58443":
        raise RuntimeError("Only the explicitly owned formal realm can restart")
    run = Path(environment["runDir"])
    if not run.name == "Safety" or not run.parent.name.startswith("formal-export-"):
        raise RuntimeError("Owned formal namespace guard")
    actual = subprocess.run(["/bin/ps", "-p", str(environment["serverPID"]), "-o", "command="],
                            capture_output=True, text=True, timeout=5, check=True).stdout.strip()
    if actual != " ".join(environment["serverCommand"]):
        raise RuntimeError("Exact owned application PID/argv mismatch; nothing stopped")
    if hashlib.sha256(Path(environment["binary"]).read_bytes()).hexdigest() != environment["binarySHA256"]:
        raise RuntimeError("Exact owned binary identity mismatch; nothing stopped")
    report.mkdir(parents=True, exist_ok=True)
    private_json(run / ("environment-before-" + label + ".json"), environment)
    for name in ["jobsimd", "jobsimops"]:
        shutil.copy2(run / "Bin" / name, run / "Bin" / (name + "-before-" + label))
    os.kill(environment["serverPID"], signal.SIGTERM)
    for _ in range(100):
        state = subprocess.run(["/bin/ps", "-p", str(environment["serverPID"]), "-o", "stat="],
                               capture_output=True, text=True, timeout=5).stdout.strip()
        if not state or state.startswith("Z"):
            break
        time.sleep(0.05)
    else:
        raise RuntimeError("Owned application did not stop; no force signal sent")
    build(environment, gocache, report)
    environment["serverCommand"] = [argument for argument in environment["serverCommand"] if argument not in remove_arguments]
    child = clean_environment()
    child.update({"JOBS_IM_DATABASE_URL": environment["businessURL"], "JOBS_IM_JOURNAL_DATABASE_URL": environment["journalURL"],
                  "PGPASSFILE": environment["pgPassFile"], "JOBS_IM_NATS_URL": environment["natsURL"],
                  "JOBS_IM_NATS_TOKEN_FILE": environment["natsTokenFile"], "JOBS_IM_PUBLIC_URL": environment["serverURL"], "GOMAXPROCS": "2"})
    environment["serverPID"] = start_process(environment["serverCommand"], run, run / "Run" / ("server-" + label + ".log"), child)
    private_json(Path(seed), environment)
    for _ in range(60):
        try:
            with urllib.request.urlopen(environment["serverURL"] + "/api/bootstrap", context=ssl.create_default_context(cafile=environment["caPath"]), timeout=2) as response:
                bootstrap = json.load(response)
            break
        except OSError:
            time.sleep(0.2)
    else:
        raise RuntimeError("Owned candidate did not become ready")
    if bootstrap["deploymentId"] != environment["deploymentId"] or bootstrap["restoreEpoch"] != environment["restoreEpoch"]:
        raise RuntimeError("Owned restart changed deployment identity or epoch")
    private_json(report / "受控重启.json", {"status": "ready", "bootstrap": bootstrap, "binarySHA256": environment["binarySHA256"], "PGAndNATSUnchanged": True})
    return environment


def create(arguments):
    os.umask(0o077)
    tools = json.loads(arguments.tool_seed.read_text())
    token = uuid.uuid4().hex
    run = Path.home() / "Library/Application Support/JobsIM/Integration" / ("formal-export-" + token) / "Safety"
    run.mkdir(parents=True, mode=0o700)
    free_space(run)
    report = arguments.report.resolve()
    report.mkdir(parents=True, exist_ok=True)
    for leaf in ["Secrets", "Bin", "Run", "TLS", "State", "NATS", "Postgres/Main", "Postgres/Journal", "Sockets/Main", "Sockets/Journal"]:
        (run / leaf).mkdir(parents=True, exist_ok=True, mode=0o700)
    ports = [58443, 59222, 63381, 63382] + list(range(59101, 59110))
    for port in ports:
        with socket.socket() as probe:
            probe.bind(("127.0.0.1", port))
    environment = {
        "freshFormalExportRealm": True, "status": "preparing", "runDir": str(run),
        "databasePrefix": "jobsim_formal_" + token[:12], "serverURL": "https://127.0.0.1:58443",
        "mainPort": 63381, "journalPort": 63382, "internalBasePort": 59101,
        "pgBin": tools["pgBin"], "natsBinary": tools["natsBinary"], "go": str(arguments.go),
        "stateDir": str(run / "State"), "pgMainData": str(run / "Postgres/Main"),
        "pgJournalData": str(run / "Postgres/Journal"), "pgPassFile": str(run / "Secrets/postgres.pgpass"),
        "caPath": str(run / "TLS/ca.pem"), "tlsCert": str(run / "TLS/server.pem"),
        "tlsKey": str(run / "TLS/server.key"), "natsURL": "nats://127.0.0.1:59222",
        "natsTokenFile": str(run / "Secrets/nats-token"), "natsConfig": str(run / "NATS/nats.conf"),
        "backupDir": str(Path.home() / "Documents/JobsIMBackups" / ("formal-export-" + token)),
        "reportDir": str(report), "maxConnectionsTheoretical": {"main": 80, "journal": 16},
    }
    seed = run / "environment.json"
    private_json(seed, environment)
    identities = []
    pgpass = []
    for journal in [False, True]:
        label = "Journal" if journal else "Main"
        port = environment["journalPort" if journal else "mainPort"]
        administrator = "formal_admin_" + ("j_" if journal else "m_") + token[:12]
        user = "formal_app_" + ("j_" if journal else "m_") + token[:12]
        database = environment["databasePrefix"] + ("_journal" if journal else "_business")
        admin_password, app_password = secrets.token_hex(32), secrets.token_hex(32)
        password_file = run / "Secrets" / (label.lower() + "-init-password")
        password_file.write_text(admin_password + "\n")
        os.chmod(password_file, 0o600)
        pgpass.extend([f"127.0.0.1:{port}:*:{administrator}:{admin_password}", f"127.0.0.1:{port}:*:{user}:{app_password}"])
        Path(environment["pgPassFile"]).write_text("\n".join(pgpass) + "\n")
        os.chmod(environment["pgPassFile"], 0o600)
        data = Path(environment["pgJournalData" if journal else "pgMainData"])
        command([Path(environment["pgBin"]) / "initdb", "-D", data, "-U", administrator,
                 "--pwfile", password_file, "--auth=scram-sha-256", "--encoding=UTF8", "--locale=C"],
                run, run / "Run" / ("initdb-" + label + ".log"))
        settings = ["listen_addresses='127.0.0.1'", f"port={port}", "unix_socket_directories=''",
                    "shared_buffers='16MB'", "max_connections=" + ("16" if journal else "80"),
                    "fsync=on", "full_page_writes=on", "synchronous_commit=on", "max_wal_size='128MB'",
                    "min_wal_size='32MB'", "log_statement='none'", "log_min_error_statement='panic'"]
        with (data / "postgresql.conf").open("a") as stream:
            stream.write("\n" + "\n".join(settings) + "\n")
        prefix = "journal" if journal else "main"
        environment[prefix + "AdminURL"] = f"postgresql://{administrator}@127.0.0.1:{port}/postgres?sslmode=disable"
        environment[prefix + "AdminUsername"] = administrator
        environment["journalURL" if journal else "businessURL"] = f"postgresql://{user}@127.0.0.1:{port}/{database}?sslmode=disable"
        environment["journalUsername" if journal else "mainUsername"] = user
        environment["journalDatabase" if journal else "businessDatabase"] = database
        private_json(seed, environment)
        command([Path(environment["pgBin"]) / "pg_ctl", "-D", data, "-l", run / "Run" / ("postgres-" + label + ".log"), "-w", "start"],
                run, run / "Run" / ("start-" + label + ".log"))
        environment["journalPGPID" if journal else "mainPGPID"] = int((data / "postmaster.pid").read_text().splitlines()[0])
        sql(environment, journal, f"CREATE ROLE {user} LOGIN PASSWORD '{app_password}'; CREATE DATABASE {database} OWNER {user};", True)
        identifier = sql(environment, journal, "SELECT system_identifier::text FROM pg_control_system();", True)
        identities.append({"port": port, "systemIdentifier": identifier, "dataDir": str(data)})
        private_json(seed, environment)
    if identities[0]["systemIdentifier"] == identities[1]["systemIdentifier"]:
        raise RuntimeError("Independent physical journal identity check failed")
    environment["pgIdentities"] = identities
    openssl = Path("/opt/homebrew/opt/openssl@3/bin/openssl")
    tls = run / "TLS"
    command([openssl, "genpkey", "-algorithm", "EC", "-pkeyopt", "ec_paramgen_curve:P-256", "-out", tls / "ca.key"], run, run / "Run/ca-key.log")
    command([openssl, "req", "-new", "-x509", "-key", tls / "ca.key", "-sha256", "-days", "30", "-subj", "/CN=JobsIM Formal Export Private CA", "-addext", "basicConstraints=critical,CA:TRUE", "-addext", "keyUsage=critical,keyCertSign,cRLSign", "-addext", "subjectKeyIdentifier=hash", "-out", tls / "ca.pem"], run, run / "Run/ca-cert.log")
    command([openssl, "genpkey", "-algorithm", "EC", "-pkeyopt", "ec_paramgen_curve:P-256", "-out", tls / "server.key"], run, run / "Run/server-key.log")
    command([openssl, "req", "-new", "-key", tls / "server.key", "-subj", "/CN=localhost", "-out", tls / "server.csr"], run, run / "Run/server-csr.log")
    (tls / "server.ext").write_text("subjectAltName=DNS:localhost,IP:127.0.0.1\nbasicConstraints=critical,CA:FALSE\nkeyUsage=critical,digitalSignature\nextendedKeyUsage=serverAuth\nsubjectKeyIdentifier=hash\nauthorityKeyIdentifier=keyid,issuer\n")
    command([openssl, "x509", "-req", "-in", tls / "server.csr", "-CA", tls / "ca.pem", "-CAkey", tls / "ca.key", "-CAcreateserial", "-days", "14", "-sha256", "-extfile", tls / "server.ext", "-out", tls / "server.pem"], run, run / "Run/server-cert.log")
    nats_token = secrets.token_hex(32)
    Path(environment["natsTokenFile"]).write_text(nats_token + "\n")
    nats_config = {"listen": "127.0.0.1:59222", "server_name": "jobsim-formal-" + token[:12],
                   "authorization": {"token": nats_token}, "jetstream": {"store_dir": str(run / "NATS/Store"), "max_memory_store": 16 << 20, "max_file_store": 256 << 20}}
    private_json(Path(environment["natsConfig"]), nats_config)
    build(environment, arguments.gocache, report)
    environment["natsPID"] = start_process([environment["natsBinary"], "-c", environment["natsConfig"]], run, run / "Run/nats.log")
    child = clean_environment()
    child.update({"JOBS_IM_DATABASE_URL": environment["businessURL"], "JOBS_IM_JOURNAL_DATABASE_URL": environment["journalURL"],
                  "PGPASSFILE": environment["pgPassFile"], "JOBS_IM_NATS_URL": environment["natsURL"],
                  "JOBS_IM_NATS_TOKEN_FILE": environment["natsTokenFile"], "JOBS_IM_PUBLIC_URL": environment["serverURL"],
                  "GOMAXPROCS": "2"})
    server = [environment["binary"], "--service=all", "--addr", "127.0.0.1:58443", "--internal-base-port", "59101",
              "--state-dir", environment["stateDir"], "--tls-cert", environment["tlsCert"], "--tls-key", environment["tlsKey"],
              "--web-dir", str(REPOSITORY / "代码/前端/Web/Dist")]
    environment["serverCommand"] = server
    environment["serverPID"] = start_process(server, run, run / "Run/server.log", child)
    private_json(seed, environment)
    context = ssl.create_default_context(cafile=environment["caPath"])
    bootstrap = None
    for attempt in range(60):
        try:
            with urllib.request.urlopen(environment["serverURL"] + "/api/bootstrap", context=context, timeout=2) as response:
                bootstrap = json.load(response)
            break
        except (OSError, ValueError):
            time.sleep(0.5)
    if not bootstrap:
        raise RuntimeError("Candidate not ready; original startup log retained private")
    environment.update({"status": "ready", "deploymentId": bootstrap["deploymentId"], "restoreEpoch": bootstrap["restoreEpoch"]})
    private_json(seed, environment)
    private_json(report / "启动结果.json", {"status": "ready", "serverURL": environment["serverURL"], "deploymentId": environment["deploymentId"],
                                           "restoreEpoch": environment["restoreEpoch"], "binarySHA256": environment["binarySHA256"],
                                           "physicalPGIdentities": identities, "maxConnectionsTheoretical": environment["maxConnectionsTheoretical"],
                                           "existingRealmsTouched": False, "bootstrap": bootstrap})
    print(json.dumps({"environmentFile": str(seed), "serverURL": environment["serverURL"], "status": "ready"}, ensure_ascii=False))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--create-owned-realm", action="store_true", required=True)
    parser.add_argument("--tool-seed", type=Path, required=True)
    parser.add_argument("--go", type=Path, required=True)
    parser.add_argument("--gocache", type=Path, required=True)
    parser.add_argument("--report", type=Path, required=True)
    arguments = parser.parse_args()
    create(arguments)


if __name__ == "__main__":
    main()
