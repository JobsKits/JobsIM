// Created by Jobs on 2026年11月17日，星期二.
import { useEffect, useRef, useState, type FormEvent } from "react";
import { ChevronRight, ShieldCheck } from "lucide-react";
import { ReceiptContextActions, type ReceiptBinding, type ReceiptContext, type ReceiptOriginal, type ReceiptTarget } from "./ReceiptContext";
import { receiptLabel } from "./ReceiptLabels";
import { migrationJSON } from "./MigrationArchive";
type Props = { context?: ReceiptBinding; enabled: boolean; locale: string; t: (key: string) => string; onLogin: () => void };
export function ReceiptPanel({ context, enabled, locale, t, onLogin }: Props) {
  const label = (key: string) => receiptLabel(locale, key), root = useRef<HTMLElement>(null);
  const [actions, setActions] = useState<ReceiptContextActions>(), [value, setValue] = useState<ReceiptContext>();
  const [rows, setRows] = useState<ReceiptOriginal[]>([]), [target, setTarget] = useState<ReceiptTarget>();
  const [busy, setBusy] = useState(false), [notice, setNotice] = useState(""), [reload, setReload] = useState(0), [shown, setShown] = useState(20);
  const identity = context?.identity, key = identity?.namespace + "|" + identity?.familyId;
  useEffect(() => {
    let live = true; const abort = new AbortController(), current = () => live && !!context?.isCurrent();
    setActions(undefined); setValue(undefined); setRows([]); setTarget(undefined); setNotice(""); root.current?.querySelectorAll("form").forEach(form => form.reset());
    if (!context || !enabled) { setBusy(false); return () => { live = false; abort.abort(); }; }
    const next = new ReceiptContextActions({ ...context, isCurrent: current, signal: abort.signal }); setBusy(true); setActions(next);
    void (async () => { const originals = await next.rows(); if (current()) setRows(originals); const status = await next.load(); if (current()) setValue(status); })()
      .catch(() => { if (current()) setNotice("invalid"); }).finally(() => { if (current()) setBusy(false); });
    return () => { live = false; abort.abort(); root.current?.querySelectorAll("form").forEach(form => form.reset()); };
  }, [key, enabled, reload]);
  const run = async (work: (next: ReceiptContextActions) => Promise<void>) => {
    const next = actions; if (!next || busy || !next.context.isCurrent()) return; setBusy(true); setNotice("");
    try { await work(next); } catch { if (next.context.isCurrent()) setNotice("unknown"); }
    finally { if (next.context.isCurrent()) { try { const originals = await next.rows(); if (next.context.isCurrent()) setRows(originals); } catch { if (next.context.isCurrent()) setNotice("invalid"); } if (next.context.isCurrent()) setBusy(false); } }
  };
  const submitOriginal = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault(); const form = event.currentTarget, text = String(new FormData(form).get("original") ?? ""); form.reset();
    void run(async next => { if (new TextEncoder().encode(text).length > 32768) throw Error(); const found = await next.checkOriginal(migrationJSON(text)); if (next.context.isCurrent()) setNotice(found ? "found" : "not_found"); });
  };
  return <section className="receipt-panel" ref={root} aria-label={label("title")}>
    <p>{label("help")}</p><p className="migration-warning">{label("local")}</p><p>{label("noUpgrade")}</p>
    <p>{label("server")}: {value ? label(value.storage.qualified ? "yes" : "no") : label("unknown")}</p>
    <p>{value?.readiness.ready ? label("ready") : label("blocked")}</p>
    {notice && <p role="alert">{label(notice)}</p>}
    <button className="secondary" disabled={busy} onClick={() => setReload(n => n + 1)}>{t("reload")}</button>
    {!context && <button className="secondary" onClick={onLogin}>{t("signIn")}</button>}
    {value ? <dl className="receipt-readiness">{Object.entries(value.readiness).filter(([name]) => !["ready", "blockers"].includes(name)).map(([name, ready]) => <div key={name}><dt>{label(name)}</dt><dd>{label(ready ? "yes" : "no")}</dd></div>)}</dl> : <div className="inline-empty"><ShieldCheck aria-hidden="true" /><p>{label("empty")}</p><button className="secondary" disabled={busy} onClick={() => setReload(n => n + 1)}>{t("reload")}</button></div>}
    {value?.readiness.blockers.length ? <details><summary>{label("blocked")}</summary><ul>{value.readiness.blockers.map(item => <li key={item}><code>{item}</code></li>)}</ul></details> : null}
    <details><summary>{label("originals")}</summary><form onSubmit={submitOriginal}>
      <label>{label("originalBody")}<textarea name="original" required maxLength={32768} autoComplete="off" spellCheck={false} /></label>
      <button className="secondary" disabled={busy || !enabled || !actions}>{label("check")}</button>
    </form><div>{rows.length ? rows.slice(0, shown).map(row => <article key={row.body.requestId}><p>{label(row.state)} · {new Date(row.createdAt).toLocaleString(locale === "bo" ? "en" : locale)}</p><details><summary>{label("originalBody")}</summary><pre>{JSON.stringify(row.body, null, 2)}</pre></details><button className="secondary" disabled={busy} onClick={() => void run(async next => { const found = await next.checkOriginal(row.body); if (next.context.isCurrent()) setNotice(found ? "found" : "not_found"); })}>{label("check")}</button></article>) : <div className="inline-empty"><ShieldCheck aria-hidden="true" /><p>{label("empty")}</p><button className="secondary" disabled={busy} onClick={() => setReload(n => n + 1)}>{t("reload")}</button></div>}</div>{rows.length > shown && <button className="secondary" onClick={() => setShown(n => n + 20)}>{t("loadMore")}</button>}</details>
    <details><summary>{label("target")}</summary><form onSubmit={event => { event.preventDefault(); const form = event.currentTarget, id = String(new FormData(form).get("messageId") ?? "").trim(); form.reset(); setTarget(undefined); void run(async next => { const fact = await next.target(id); if (next.context.isCurrent()) setTarget(fact); }); }}>
      <label>{label("messageId")}<input name="messageId" required autoComplete="off" /></label><button className="secondary" disabled={busy || !enabled || !actions}>{label("check")}</button>
    </form>{target ? <details open><summary>{label("range")}</summary><pre>{JSON.stringify(target, null, 2)}</pre></details> : <div className="inline-empty"><ShieldCheck aria-hidden="true" /><p>{label("empty")}</p><button className="secondary" disabled={busy} onClick={() => setReload(n => n + 1)}>{t("reload")}</button></div>}</details>
  </section>;
}
export default function ReceiptEntry(props: Props) {
  const [open, setOpen] = useState(false), identity = props.context?.identity;
  useEffect(() => { setOpen(false); }, [identity?.namespace, identity?.familyId]);
  useEffect(() => { const hide = () => { if (document.visibilityState === "hidden") setOpen(false); }, close = () => setOpen(false); document.addEventListener("visibilitychange", hide); window.addEventListener("pagehide", close); return () => { document.removeEventListener("visibilitychange", hide); window.removeEventListener("pagehide", close); }; }, []);
  return <section><button className="setting-line" aria-expanded={open} onClick={() => setOpen(value => !value)}><span><ShieldCheck size={18} />{receiptLabel(props.locale, "title")}</span><ChevronRight size={17} /></button>{open && <ReceiptPanel {...props} />}</section>;
}
