import { useEffect, useRef, useState } from "react";
import { Download, FileArchive, RefreshCw } from "lucide-react";
import { ApiError } from "./api";
import { DataExportFileError } from "./DataExportArchive";
import { DataExportActions, type DataExportContext } from "./DataExportActions";
import { dataExportLabel } from "./DataExportLabels";
import type { ExportIntent, ExportView } from "./DataExportProtocol";

export default function DataExportPanel({ context, enabled, locale, t, onConnect, onReload }: {
  context?: DataExportContext; enabled: boolean; locale: string;
  t: (key: string) => string; onConnect: () => void; onReload: () => void;
}) {
  const label = (key: string) => dataExportLabel(locale, key);
  const [rows, setRows] = useState<ExportIntent[]>([]), [views, setViews] = useState<Record<string, ExportView>>({});
  const [busy, setBusy] = useState(false), [failed, setFailed] = useState(false), [notice, setNotice] = useState("");
  const [failureMessage, setFailureMessage] = useState("");
  const [confirming, setConfirming] = useState(""), [shown, setShown] = useState(20);
  const mounted = useRef(false), lifecycle = useRef(0), reading = useRef(0), working = useRef(false);
  const liveContext = useRef(context), urls = useRef(new Set<string>()), controller = useRef<AbortController | undefined>(undefined);
  liveContext.current = context;
  const current = () => mounted.current && !!liveContext.current?.isCurrent();
  const capture = () => {
    const original = liveContext.current, generation = lifecycle.current;
    if (!original) return undefined;
    return new DataExportActions({ ...original, signal: controller.current?.signal,
      isCurrent: () => mounted.current && generation === lifecycle.current && original.isCurrent(),
    });
  };
  const refresh = async (source: DataExportActions) => {
    const sequence = ++reading.current;
    // Old status must not keep a download enabled after an unsuccessful refresh.
    if (source.context.isCurrent()) setViews({});
    const originals = await source.rows();
    const results = await Promise.allSettled(originals.slice(0, shown).map(row => source.status(row.requestId)));
    if (!source.context.isCurrent() || sequence !== reading.current) return;
    const next: Record<string, ExportView> = {};
    results.forEach((result, i) => { if (result.status === "fulfilled" && result.value) next[originals[i].requestId] = result.value; });
    setRows(originals); setViews(next);
    if (results.some(result => result.status === "rejected")) setFailed(true);
  };
  const run = async (action: (source: DataExportActions) => Promise<void>) => {
    if (working.current || !current()) return;
    const source = capture(), generation = lifecycle.current;
    if (!source) return;
    working.current = true; setBusy(true); setFailed(false); setFailureMessage(""); setNotice("");
    try { await action(source); }
    catch (error) {
      if (source.context.isCurrent()) {
        setFailed(true);
        setFailureMessage((error instanceof ApiError || error instanceof DataExportFileError) && error.code === "DATA_EXPORT_FILE_LIMIT" ? label("browserLimit") : error instanceof ApiError && error.status === 429 ? label("limited") : label("notConfirmed"));
        // A failed write can still have committed; only read its durable original next.
        await refresh(source).catch(() => {});
      }
    } finally {
      if (source.context.isCurrent()) setBusy(false);
      if (generation === lifecycle.current) working.current = false;
    }
  };
  const reload = () => void run(refresh);
  useEffect(() => {
    mounted.current = true; lifecycle.current++; controller.current = new AbortController();
    setRows([]); setViews({}); setConfirming(""); setNotice(""); setFailed(false);
    if (context) reload();
    return () => {
      mounted.current = false; lifecycle.current++; reading.current++; working.current = false;
      controller.current?.abort(); urls.current.forEach(value => URL.revokeObjectURL(value)); urls.current.clear();
    };
  }, [context?.namespace, enabled]);
  useEffect(() => { if (context && mounted.current) reload(); }, [shown]);
  useEffect(() => {
    if (!context || !rows.some(row => ["requested", "building"].includes(views[row.requestId]?.job.state ?? ""))) return;
    const timer = setInterval(reload, 5000);
    return () => clearInterval(timer);
  }, [rows, views, enabled]);
  const create = () => void run(async source => { await source.create(); await refresh(source); });
  const check = (id: string, retry = false) => void run(async source => {
    const result = await source.check(id, retry);
    if (source.context.isCurrent() && !result) setNotice(label("pendingHint"));
    await refresh(source);
  });
  const cancel = (id: string) => {
    setConfirming("");
    void run(async source => { await source.cancel(id); await refresh(source); });
  };
  const download = (id: string) => void run(async source => {
    const file = await source.download(id);
    if (!source.context.isCurrent()) return;
    const url = URL.createObjectURL(file.blob), anchor = document.createElement("a");
    urls.current.add(url); anchor.href = url; anchor.download = file.fileName;
    document.body.append(anchor); anchor.click(); anchor.remove();
    setTimeout(() => { URL.revokeObjectURL(url); urls.current.delete(url); }, 1000);
    setNotice(label("saved")); await refresh(source);
  });
  const time = (value: string) => {
    const date = new Date(value);
    try {
      if (Intl.DateTimeFormat.supportedLocalesOf([locale]).length) {
        return new Intl.DateTimeFormat(locale, { dateStyle: "medium", timeStyle: "short" }).format(date);
      }
    } catch {}
    // Partial Intl builds must not silently replace the selected language with the host language.
    const pad = (number: number) => String(number).padStart(2, "0");
    return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}`;
  };
  const available = !!context;
  if (context && !context.isCurrent()) return null;
  return <section className="settings-section data-export-panel" aria-label={label("title")}>
    <div className="self-state-heading"><h3 style={{ minWidth: 0 }}>{label("title")}</h3><button type="button" className="icon-button" disabled={busy} aria-label={t("reload")} onClick={available ? reload : onReload}><RefreshCw size={17} /></button></div>
    <p className="self-state-hint">{label("summary")}</p>
    {!available ? <div className="empty"><FileArchive size={28} /><h4>{label("connect")}</h4><p>{label("unavailable")}</p><div className="self-state-buttons"><button type="button" className="secondary" onClick={onConnect}>{t("connect")}</button><button type="button" className="secondary" onClick={onReload}>{t("reload")}</button></div></div> : <>
      <p className="self-state-hint">{label("limits")}</p>
      {!enabled && <p className="self-state-hint" role="status">{label("restricted")}</p>}
      <button type="button" className="primary" disabled={!enabled || busy || rows.some(row => !row.jobId)} onClick={create}>{busy ? t("working") : label("create")}</button>
      <h4>{label("localList")}</h4><p className="self-state-hint">{label("localOnly")}</p>
      {!rows.length ? <div className="empty"><FileArchive size={28} /><h4>{busy ? t("working") : failed ? label("notConfirmed") : label("empty")}</h4><button type="button" className="secondary" disabled={busy} onClick={reload}>{t("reload")}</button></div> : rows.slice(0, shown).map(row => {
        const view = views[row.requestId], job = view?.job;
        const terminal = !!job && ["cancelled", "expired", "failed"].includes(job.state);
        const downloadable = enabled && !!job && job.state === "ready" && job.cleanupState === "published" && Date.parse(job.expiresAt) > Date.now() && !row.cancelBody;
        return <article className="self-state-panel" key={row.requestId} aria-label={label("localList") + " · " + time(row.createdAt)}>
          <div className="self-state-heading" style={{ flexWrap: "wrap" }}><h4>{time(row.createdAt)}</h4><span role="status">{job ? label(job.state) : label("unknown")}</span></div>
          {job && <p className="self-state-hint">{label("deadline")} · {time(job.expiresAt)}{job.cleanupState !== "published" && job.cleanupState !== "cleared" ? " · " + label("cleanup") : ""}</p>}
          {!job && <p className="self-state-hint">{label("pendingHint")}</p>}
          <div className="self-state-buttons">
            <button type="button" className="secondary" disabled={busy} onClick={() => check(row.requestId)}>{label("check")}</button>
            {!row.jobId && <button type="button" className="secondary" disabled={!enabled || busy} onClick={() => check(row.requestId, true)}>{label("retry")}</button>}
            {downloadable && <button type="button" className="secondary" disabled={busy} onClick={() => download(row.requestId)}><Download size={16} />{label("download")}</button>}
            {row.jobId && !terminal && <button type="button" className="secondary" disabled={busy} onClick={() => setConfirming(row.requestId)}>{label(row.cancelBody ? "retryCancel" : "cancel")}</button>}
          </div>
          {confirming === row.requestId && <div role="group" aria-label={label("cancelConfirm")}><p>{label("cancelConfirm")}</p><div className="self-state-buttons"><button type="button" className="secondary" disabled={busy} onClick={() => cancel(row.requestId)}>{label("confirmCancel")}</button><button type="button" className="secondary" disabled={busy} onClick={() => setConfirming("")}>{label("keep")}</button></div></div>}
        </article>;
      })}
      {rows.length > shown && <button type="button" className="secondary" disabled={busy} onClick={() => { setShown(shown + 20); }}>{label("more")}</button>}
      <p className="self-state-hint">{label("downloadHint")}</p>
    </>}
    {failed && <p role="alert">{failureMessage || label("notConfirmed")}</p>}{notice && <p role="status">{notice}</p>}
  </section>;
}
