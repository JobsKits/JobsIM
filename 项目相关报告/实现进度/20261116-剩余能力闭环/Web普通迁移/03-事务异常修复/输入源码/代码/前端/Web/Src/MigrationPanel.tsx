// Created by Jobs on 2026年11月17日，星期二.
import { useEffect, useRef, useState, type FormEvent } from "react";
import { ArrowDownToLine, ChevronRight, Copy, RefreshCw } from "lucide-react";
import { MigrationActions, type MigrationContext, type MigrationPairing } from "./MigrationActions";
import { migrationSyncSource } from "./SyncStore";
import { migrationLabel } from "./MigrationLabels";
import { migrationLimit, migrationSelect } from "./MigrationProtocol";
import type { MigrationIntent } from "./MigrationStore";
import type { MigrationView } from "./MigrationArchive";
import type { MigrationHistoryEntry } from "./MigrationHistoryStore";
import type { PasswordChangeIdentity } from "./PasswordChangeProtocol";
import type { Message } from "./types";
import { subscribeGlobalMessages } from "./MessageFacts";

type Context = { identity: PasswordChangeIdentity; isCurrent: () => boolean; request?: MigrationContext["request"]; binary?: MigrationContext["binary"] };
type Props = { context?: Context; enabled: boolean; administrator: boolean; locale: string; t: (key: string) => string; onLogin: () => void; onReload: () => void; observations?: string };
type OriginalResult = MigrationPairing | MigrationView | undefined;
export function MigrationPanel({ context, enabled, administrator, locale, t, onLogin, onReload, observations }: Props) {
  const m = (key: string) => migrationLabel(locale, key);
  const [actions, setActions] = useState<MigrationActions>();
  const [available, setAvailable] = useState(false), [busy, setBusy] = useState(false), [notice, setNotice] = useState("");
  const [rows, setRows] = useState<MigrationIntent[]>([]), [history, setHistory] = useState<MigrationHistoryEntry[]>([]), [candidates, setCandidates] = useState<Message[]>([]);
  const [selected, setSelected] = useState<string[]>([]), [full, setFull] = useState(false), [shown, setShown] = useState(50), [originalsShown, setOriginalsShown] = useState(20), [historyShown, setHistoryShown] = useState(20);
  const [pairing, setPairing] = useState<MigrationPairing>(), [target, setTarget] = useState<MigrationView>(), [targetId, setTargetId] = useState("");
  const [retryRow, setRetryRow] = useState<MigrationIntent>(), [file, setFile] = useState<File>(), [reload, setReload] = useState(0);
  const generation = useRef(0), urls = useRef(new Set<string>()), panel = useRef<HTMLElement>(null), fileInput = useRef<HTMLInputElement>(null);
  const denied = useRef(new Set<string>()), historyRevision = useRef(0);
  const contextKey = context ? [context.identity.namespace, context.identity.familyId].join("|") : "";
  useEffect(() => {
    const currentGeneration = ++generation.current, abort = new AbortController(); let live = true;
    const current = () => live && generation.current === currentGeneration && !!context?.isCurrent();
    denied.current = new Set(); historyRevision.current++;
    panel.current?.querySelectorAll("form").forEach(form => form.reset());
    setActions(undefined); setAvailable(false); setRows([]); setHistory([]); setCandidates([]); setSelected([]); setPairing(undefined); setTarget(undefined); setTargetId(""); setRetryRow(undefined); setFile(undefined); setNotice(""); setBusy(true);
    for (const url of urls.current) URL.revokeObjectURL(url); urls.current.clear();
    if (!context || !enabled) { setBusy(false); return () => { live = false; abort.abort(); }; }
    const identity = context.identity;
    void (async () => {
      const physical = await migrationSyncSource(identity.namespace, { scopeId: identity.scopeId, principalId: identity.userId, sourceContextId: "personal", deviceId: identity.deviceId, restoreEpoch: identity.epoch }, current);
      if (!current()) return;
      const value = new MigrationActions({ ...context, identity: { ...identity, storeGeneration: physical.storeGeneration }, guardGeneration: physical.guardGeneration, isCurrent: current, signal: abort.signal });
      const originals = await value.rows(); if (!current()) return;
      setActions(value); setCandidates(physical.messages); setRows(originals);
      const imported = await value.history(); if (!current()) return; setHistory(imported.filter(entry => !denied.current.has(entry.message.id)));
      try { await value.capability(); if (current()) setAvailable(true); } catch { if (current()) setNotice("unavailable"); }
    })().catch(() => { if (current()) setNotice("unavailable"); }).finally(() => { if (current()) setBusy(false); });
    return () => { live = false; abort.abort(); panel.current?.querySelectorAll("form").forEach(form => form.reset()); for (const url of urls.current) URL.revokeObjectURL(url); urls.current.clear(); };
  }, [contextKey, enabled, reload]);
  useEffect(() => {
    if (!actions) return;
    const scope = actions.context.identity.namespace.split("|").slice(0, 4).join("|");
    return subscribeGlobalMessages(notice => {
      if (!actions.context.isCurrent() || notice.scope !== scope) return;
      notice.ids.forEach(id => denied.current.add(id)); historyRevision.current++;
      setHistory(rows => notice.storageFailed ? [] : rows.filter(entry => !denied.current.has(entry.message.id)));
    });
  }, [actions]);
  useEffect(() => {
    if (!actions) return;
    const refresh = () => {
      const revision = ++historyRevision.current;
      setHistory(rows => rows.filter(entry => !denied.current.has(entry.message.id)).map(entry => entry.message.forwarded ? { ...entry, message: { ...entry.message, text: "" }, unavailable: true } : entry));
      void actions.history().then(rows => { if (actions.context.isCurrent() && historyRevision.current === revision) setHistory(rows.filter(entry => !denied.current.has(entry.message.id))); }).catch(() => {});
    };
    refresh(); const timer = window.setInterval(refresh, 5000); return () => { window.clearInterval(timer); historyRevision.current++; };
  }, [actions, observations]);
  const fresh = (value: MigrationActions) => value === actions && value.context.isCurrent();
  const refreshRows = async (value: MigrationActions) => {
    const revision = historyRevision.current, [originals, imported] = await Promise.all([value.rows(), value.history()]); if (!fresh(value)) return; setRows(originals); if (historyRevision.current === revision) setHistory(imported.filter(entry => !denied.current.has(entry.message.id)));
  };
  const run = async (operation: (value: MigrationActions) => Promise<void>) => {
    const value = actions; if (!value || busy || !fresh(value)) return;
    setBusy(true); setNotice("");
    try { await operation(value); }
    catch (error) {
      if (fresh(value)) {
        const code = error && typeof error === "object" && "code" in error ? String(error.code) : "";
        setNotice(code === "MIGRATION_ALREADY_REPORTED" ? "installed_reported" : /EXPIRED/.test(code) ? "expired" : /CHANGED|ORIGINAL|FLOOR|CONTEXT|CLOSED/.test(code) ? "sourceChanged" : "unknown");
      }
    } finally { if (fresh(value)) { try { await refreshRows(value); } catch { if (fresh(value)) setNotice("unknown"); } if (fresh(value)) setBusy(false); } }
  };
  const accept = (result: OriginalResult) => {
    if (!result) { setNotice("unknown"); return; }
    if ("pairingProof" in result) { setPairing(result); setNotice(Date.parse(result.expiresAt) <= Date.now() ? "expired" : "ready"); }
    else { setNotice(result.status); if (actions && result.manifest.targetDeviceId === actions.context.identity.deviceId && result.manifest.targetStoreGeneration === actions.context.identity.storeGeneration) { setTarget(result); setTargetId(result.id); } }
  };
  const proofFields = () => <>
    <label>{t("password")}<input name="password" type="password" required autoComplete="off" maxLength={512} /></label>
    {administrator && <label>{t("otp")}<input name="otp" type="password" required autoComplete="off" pattern="[0-9]{6}" inputMode="numeric" maxLength={6} /></label>}
  </>;
  const submit = (event: FormEvent<HTMLFormElement>, mode: "pair" | "source" | "install") => {
    event.preventDefault(); const form = event.currentTarget, data = new FormData(form);
    const password = String(data.get("password") ?? ""), otp = String(data.get("otp") ?? ""), code = String(data.get("pairingProof") ?? "").trim(), consent = data.get("consent") === "on";
    form.reset();
    if (!consent || !password || administrator && !/^[0-9]{6}$/.test(otp)) { setNotice("invalid"); return; }
    const original = retryRow;
    void run(async value => {
      if (mode === "pair") accept(await value.pair(password, otp, original?.kind === "pairing" ? original.requestId : undefined));
      if (mode === "source") {
        if (!code) { setNotice("invalid"); return; }
        const row = original?.kind === "create" ? original : await value.select(candidates.filter(item => selected.includes(item.id)).map(item => migrationSelect(item, full)));
        accept(await value.create(row.requestId, code, password, otp));
      }
      if (mode === "install") {
        if (!target || target.id !== targetId) { setNotice("invalid"); return; }
        const selectedFile = file; setFile(undefined);
        const result = await value.install(target.id, password, otp, selectedFile); setNotice(result.reported ? "installed" : "notReported");
      }
      if (fresh(value)) setRetryRow(undefined);
    });
  };
  const copy = (text: string) => void run(async value => { if (!fresh(value)) return; await navigator.clipboard.writeText(text); if (fresh(value)) setNotice("copied"); });
  const save = (blob: Blob, name: string) => {
    const url = URL.createObjectURL(blob); urls.current.add(url); const link = document.createElement("a"); link.href = url; link.download = name; link.rel = "noopener"; document.body.appendChild(link); link.click(); link.remove();
    window.setTimeout(() => { URL.revokeObjectURL(url); urls.current.delete(url); }, 60000);
  };
  const reloadPanel = () => { setReload(value => value + 1); onReload(); };
  const empty = (label = m("empty")) => <div className="empty"><div className="empty-icon"><ArrowDownToLine /></div><h3>{label}</h3><button className="secondary" disabled={busy} onClick={reloadPanel}><RefreshCw size={16} />{t("reload")}</button></div>;
  const canRetry = (row: MigrationIntent) => !!actions && row.identity.namespace === actions.context.identity.namespace && row.identity.familyId === actions.context.identity.familyId && row.identity.storeGeneration === actions.context.identity.storeGeneration;
  if (!context || !enabled) return <section className="migration-panel" aria-label={m("title")}><h2>{m("title")}</h2><p>{m("transport")}</p>{empty(m("unavailable"))}{!context && <button onClick={onLogin}>{t("signIn")}</button>}</section>;
  return <section ref={panel} className="migration-panel" aria-label={m("title")}>
    <h2>{m("title")}</h2><p>{m("retain")}</p><p className="hint">{m("transport")}</p>
    <button className="secondary" disabled={busy} onClick={reloadPanel}><RefreshCw size={16} />{t("reload")}</button>
    {!context && <button onClick={onLogin}>{t("signIn")}</button>}
    {notice && <p role="status">{notice === "copied" ? t("copied") : m(notice)}</p>}
    {!available && !busy && <p>{m("unavailable")}</p>}
    <p className="hint">{m("secrets")}</p>
    {retryRow && <div className="group-confirm"><p>{m("retry")} · <code>{retryRow.requestId}</code></p><button className="secondary" disabled={busy} onClick={() => setRetryRow(undefined)}>{t("cancel")}</button></div>}
    <h3>{m("target")}</h3>
    <form onSubmit={event => submit(event, "pair")}>
      {proofFields()}<label className="group-consent"><input name="consent" type="checkbox" required />{m("pairConsent")}</label>
      <button disabled={busy || !available || !!retryRow && retryRow.kind !== "pairing"}>{retryRow?.kind === "pairing" ? m("retry") : m("pair")}</button>
    </form>
    {pairing && <div className="migration-pairing"><label>{m("pairCode")}<textarea readOnly value={pairing.pairingProof} autoComplete="off" /></label><time>{pairing.expiresAt}</time><button className="secondary" disabled={busy || Date.parse(pairing.expiresAt) <= Date.now()} onClick={() => { if (Date.parse(pairing.expiresAt) <= Date.now()) setNotice("expired"); else copy(pairing.pairingProof); }}><Copy size={16} />{m("copy")}</button></div>}
    <h3>{m("source")}</h3><p>{m("choose")} · {m("selection")} {retryRow?.kind === "create" ? retryRow.selection.length : selected.length}/256</p>
    {candidates.length === 0 ? empty() : <>
      <div className="migration-choices">{candidates.slice(0, shown).map(message => <label key={message.id}><input type="checkbox" checked={selected.includes(message.id)} disabled={busy || !!retryRow || selected.length >= 256 && !selected.includes(message.id)} onChange={event => setSelected(values => event.target.checked ? [...values, message.id] : values.filter(id => id !== message.id))} /><span>{message.text.slice(0, 120) || t(message.type)}<small>{message.createdAt}</small></span></label>)}</div>
      {shown < candidates.length && <button className="secondary" disabled={busy} onClick={() => setShown(value => value + 50)}>{t("loadMore")}</button>}
    </>}
    <label className="group-consent"><input type="checkbox" checked={full} disabled={busy || !!retryRow} onChange={event => setFull(event.target.checked)} />{m("full")} · {m("metadata")}</label>
    <form onSubmit={event => submit(event, "source")}>
      <label>{m("pairCode")}<textarea name="pairingProof" required autoComplete="off" maxLength={16384} /></label>
      {proofFields()}<label className="group-consent"><input name="consent" type="checkbox" required />{m("consent")}</label>
      <button disabled={busy || !available || !!retryRow && retryRow.kind !== "create" || (retryRow?.kind !== "create" && !selected.length)}>{retryRow?.kind === "create" ? m("retry") : m("create")}</button>
    </form>
    <h3>{m("capsule")}</h3>
    <label>{m("migrationId")}<input value={targetId} autoComplete="off" maxLength={36} onChange={event => { setTargetId(event.target.value); setTarget(undefined); setFile(undefined); }} /></label>
    <button className="secondary" disabled={busy || !actions || !targetId} onClick={() => void run(async value => { const result = await value.observeTarget(targetId.trim()); if (fresh(value)) { setTarget(result); setTargetId(result.id); setNotice(result.status); } })}>{m("check")}</button>
    {target && <div className="migration-summary"><p>{m("selection")} {target.manifest.messages.length} · {m("full")} {target.manifest.objects.length}</p><time>{target.expiresAt}</time><button className="secondary" disabled={busy} onClick={() => copy(target.id)}>{m("copyID")}</button><button className="secondary" disabled={busy} onClick={() => void run(async value => { const bytes = await value.download(target.id); if (fresh(value)) { save(new Blob([bytes], { type: "application/octet-stream" }), target.id + ".jimmig"); setNotice("saved"); } })}>{m("download")}</button></div>}
    <div className="migration-file"><input ref={fileInput} type="file" aria-label={m("importFile")} accept=".jimmig,application/octet-stream" hidden disabled={busy || !target || target.status !== "ready"} onChange={event => { const next = event.target.files?.[0]; event.target.value = ""; if (next && next.size <= migrationLimit) setFile(next); else { setFile(undefined); if (next) setNotice("invalid"); } }} /><button className="secondary" disabled={busy || !target || target.status !== "ready"} onClick={() => fileInput.current?.click()}>{m("importFile")}</button></div>
    {file && <p>{file.name} · {file.size} <button className="secondary" disabled={busy} onClick={() => setFile(undefined)}>{t("remove")}</button></p>}
    <form onSubmit={event => submit(event, "install")}>
      {proofFields()}<label className="group-consent"><input name="consent" type="checkbox" required />{m("consent")}</label><button disabled={busy || !available || !target}>{m("install")}</button>
    </form>
    <h3>{m("originals")}</h3>
    {rows.length === 0 ? empty() : <div className="migration-originals">{rows.slice(0, originalsShown).map(row => <article key={row.requestId}>
      <p>{m(row.kind === "pairing" ? "pair" : row.kind === "target" ? "target" : "source")} · {m(row.state === "unsent" ? "staging" : row.state === "blocked" ? "sourceChanged" : row.state)}</p><time>{row.createdAt}</time><code>{row.requestId}</code><small>{m("selection")} {row.selection.length}</small>
      <button className="secondary" disabled={busy} onClick={() => void run(async value => accept(await value.checkOriginal(row.requestId)))}>{m("check")}</button>
      {row.kind !== "target" && ["unsent", "unknown"].includes(row.state) && <button className="secondary" disabled={busy || !canRetry(row)} onClick={() => setRetryRow(row)}>{m("retry")}</button>}
      {row.migrationId && <><code>{row.migrationId}</code><button className="secondary" disabled={busy} onClick={() => copy(row.migrationId!)}>{m("copyID")}</button><button className="secondary" disabled={busy || !canRetry(row)} onClick={() => void run(async value => { const result = await value.cancel(row.migrationId!); if (fresh(value)) setNotice(result.status); })}>{t("cancel")}</button></>}
    </article>)}</div>}
    {rows.length > originalsShown && <button className="secondary" disabled={busy} onClick={() => setOriginalsShown(value => value + 20)}>{t("loadMore")}</button>}
    <h3>{m("history")}</h3>
    {history.length === 0 ? empty() : <div className="migration-history">{history.slice(0, historyShown).map(entry => <article key={entry.message.id}>
      <p>{entry.unavailable ? t("contentUnavailable") : entry.message.text || t(entry.message.type)}</p><time>{entry.message.createdAt}</time><small>{m(entry.message.objectMode === "full" ? "full" : "metadata")}</small>
      {entry.object && <button className="secondary" disabled={busy} onClick={() => void run(async value => { const current = (await value.history()).find(item => item.message.id === entry.message.id && item.message.contentVersion === entry.message.contentVersion); if (!current?.object || !fresh(value)) return; save(current.object.bytes, current.object.objectId + ".bin"); })}>{t("download")}</button>}
    </article>)}</div>}
    {history.length > historyShown && <button className="secondary" disabled={busy} onClick={() => setHistoryShown(value => value + 20)}>{t("loadMore")}</button>}
  </section>;
}
export default function MigrationEntry(props: Props) {
  const [open, setOpen] = useState(false);
  const identity = props.context?.identity;
  useEffect(() => { setOpen(false); }, [identity?.namespace, identity?.familyId]);
  useEffect(() => { const hide = () => { if (document.visibilityState === "hidden") setOpen(false); }; const close = () => setOpen(false); document.addEventListener("visibilitychange", hide); window.addEventListener("pagehide", close); return () => { document.removeEventListener("visibilitychange", hide); window.removeEventListener("pagehide", close); }; }, []);
  return <section className="migration-entry"><button className="setting-line" aria-expanded={open} onClick={() => setOpen(value => !value)}><span><ArrowDownToLine size={18} />{migrationLabel(props.locale, "title")}</span><ChevronRight size={17} /></button>{open && <MigrationPanel {...props} />}</section>;
}
