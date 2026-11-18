// Created by Jobs on 2026年11月17日，星期二.
import { useEffect, useRef, useState } from "react";
import { Download, Paperclip, RefreshCw } from "lucide-react";
import { AttachmentSendActions, type AttachmentSendContext } from "./AttachmentSendActions";
import { attachmentLabel } from "./AttachmentSendLabels";
import type { AttachmentSendIntent } from "./AttachmentSendProtocol";
import "./AudioMedia.css";

export default function AttachmentSendPanel({ context, revision, locale, conversationName }: {
  context?: AttachmentSendContext; revision: number; locale: string; conversationName: (id: string) => string;
}) {
  const [rows, setRows] = useState<AttachmentSendIntent[]>([]), [busy, setBusy] = useState(false), [failed, setFailed] = useState(false), [shown, setShown] = useState(8);
  const mounted = useRef(false), generation = useRef(0), working = useRef(false), owner = useRef<AbortController | undefined>(undefined), live = useRef(context); live.current = context;
  const label = (key: string) => attachmentLabel(locale, key);
  const capture = () => {
    const original = live.current, own = generation.current;
    return original ? new AttachmentSendActions({ ...original, signal: owner.current?.signal, isCurrent: () => mounted.current && own === generation.current && original.isCurrent() }) : undefined;
  };
  const load = async (source: AttachmentSendActions) => { const values = await source.rows(); if (source.context.isCurrent()) setRows(values); };
  const run = async (action: (source: AttachmentSendActions) => Promise<void>) => {
    const source = capture(), own = generation.current;
    if (!source || working.current) return;
    working.current = true; setBusy(true); setFailed(false);
    try { await action(source); }
    catch { if (source.context.isCurrent()) setFailed(true); }
    finally {
      if (source.context.isCurrent()) { await load(source).catch(() => setFailed(true)); setBusy(false); }
      if (own === generation.current) working.current = false;
    }
  };
  const reload = () => void run(async source => { await load(source); });
  useEffect(() => {
    mounted.current = true; generation.current++; owner.current = new AbortController(); working.current = false;
    setRows([]); setBusy(false); setFailed(false);
    const source = capture(); if (source) void load(source).catch(() => { if (source.context.isCurrent()) setFailed(true); });
    return () => { mounted.current = false; generation.current++; owner.current?.abort(); };
  }, [context?.identity.namespace, context?.identity.familyId]);
  useEffect(() => { const source = capture(); if (source) void load(source).catch(() => { if (source.context.isCurrent()) setFailed(true); }); }, [revision]);
  const save = (id: string) => void run(async source => {
    const entry = (await source.rows()).find(value => value.requestId === id);
    if (!entry?.blob || !source.context.isCurrent()) throw Error("ATTACHMENT_BLOB_UNAVAILABLE");
    const url = URL.createObjectURL(entry.blob), anchor = document.createElement("a"); anchor.href = url; anchor.download = entry.manifest.name; anchor.click(); window.setTimeout(() => URL.revokeObjectURL(url), 1000);
  });
  if (!context) return null;
  return <section className="attachment-send-panel" aria-label={label("title")}>
    <div className="self-state-heading"><h4>{label("title")}</h4><button className="icon-button" type="button" aria-label={label("reload")} disabled={busy} onClick={reload}><RefreshCw size={16} /></button></div>
    <p className="self-state-hint">{label("hint")}</p>
    {!rows.length ? <div className="empty"><Paperclip size={26} /><h4>{label(failed ? "failed" : "empty")}</h4><button className="secondary" type="button" disabled={busy} onClick={reload}>{label("reload")}</button></div> : rows.slice(0, shown).map(entry => {
      const current = entry.identity.namespace === context.identity.namespace && entry.identity.familyId === context.identity.familyId, terminal = ["accepted", "cancelled", "retired"].includes(entry.state);
      return <article className="attachment-send-original" key={entry.requestId}>
        <div className="self-state-heading"><strong>{entry.manifest.name}</strong><span role="status">{label(entry.state)}</span></div>
        <p className="self-state-hint">{label("conversation")} · {conversationName(entry.message.conversationId)}</p>
        {!current && <p role="status">{label("frozen")}</p>}
        {entry.failure && <p role="status">{label(entry.failure)}</p>}
        <div className="audio-recorder-actions">
          <button className="secondary" type="button" disabled={busy || !current || terminal} onClick={() => void run(source => source.run(entry.requestId, false))}>{label("check")}</button>
          <button className="secondary" type="button" disabled={busy || !current || terminal || entry.state === "enqueued"} onClick={() => void run(source => source.run(entry.requestId, true))}>{label("retry")}</button>
          {entry.blob && <button className="secondary" type="button" disabled={busy} onClick={() => save(entry.requestId)}><Download size={15} />{label("save")}</button>}
          {entry.blob && <button className="secondary" type="button" disabled={busy || !current || entry.state === "enqueued"} onClick={() => { if (window.confirm(label("confirm"))) void run(source => source.cancel(entry.requestId)); }}>{label("cancel")}</button>}
        </div>
      </article>;
    })}
    {rows.length > shown && <button className="secondary" type="button" onClick={() => setShown(shown + 8)}>{label("reload")}</button>}
    {failed && rows.length > 0 && <p role="alert">{label("failed")} <button className="secondary" type="button" disabled={busy} onClick={reload}>{label("reload")}</button></p>}
  </section>;
}
