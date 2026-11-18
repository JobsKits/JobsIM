import { useEffect, useRef, useState } from "react";
import { RefreshCw } from "lucide-react";
import { SelfStateActions, type LiveSelfState, type SelfResult, type ViewChanges } from "./SelfStateActions";
import { selfLabel } from "./SelfStateLabels";
import type { SelfOperation } from "./SyncStore";
import type { Message } from "./types";

export default function SelfStatePanel({ actions, conversationId, locale, localDraft, selectedReply, cloudEnabled, t, onAdopt, onChanged, onError }: {
  actions: SelfStateActions; conversationId: string; locale: string; localDraft: string; selectedReply: Message | null; cloudEnabled: boolean;
  t: (key: string) => string; onAdopt: (text: string, expectedLocal: string) => Promise<void>;
  onChanged: () => void; onError: (error: unknown) => void;
}) {
  const label = (key: string) => selfLabel(locale, key);
  const [view, setView] = useState<LiveSelfState>(), [draft, setDraft] = useState<LiveSelfState>();
  const [operations, setOperations] = useState<SelfOperation[]>([]), [busy, setBusy] = useState(false), [message, setMessage] = useState("");
  const mounted = useRef(false), working = useRef(false), sequence = useRef(0);
  const current = () => mounted.current && actions.context.isCurrent();
  const reload = async () => {
    const read = ++sequence.current;
    const jobs = await Promise.allSettled([actions.get("view", conversationId), cloudEnabled ? actions.get("draft", conversationId) : Promise.resolve(undefined), actions.operations()]);
    if (!current() || sequence.current !== read) return;
    if (jobs[0].status === "fulfilled") setView(jobs[0].value); else { setView(undefined); onError(jobs[0].reason); }
    if (jobs[1].status === "fulfilled") setDraft(jobs[1].value); else { setDraft(undefined); onError(jobs[1].reason); }
    if (jobs[2].status === "fulfilled") setOperations(jobs[2].value); else onError(jobs[2].reason);
  };
  useEffect(() => {
    mounted.current = true;
    void reload();
    return () => { mounted.current = false; sequence.current++; };
  }, [conversationId, actions.context.namespace]);
  const run = async (action: () => Promise<SelfResult | void>) => {
    if (working.current || !current()) return;
    working.current = true; setBusy(true); setMessage("");
    try {
      const result = await action();
      if (!current()) return;
      if (result) setMessage(result.status === "retired" ? label("retiredState") : result.status === "prepared" ? label("pendingState") : !result.current ? label("oldResult") : t("saved"));
      onChanged();
    } catch (error) { if (current()) onError(error); }
    finally {
      if (current()) { await reload(); setBusy(false); }
      working.current = false;
    }
  };
  const change = (changes: ViewChanges) => { const version = view?.version; void run(() => actions.view(conversationId, changes, version)); };
  const pending = operations.filter(row => row.channel === "preferences|" || row.channel.endsWith("|" + conversationId)).filter(row => ["pending", "prepared"].includes(row.status));
  const copy = async (text: string) => { await navigator.clipboard.writeText(text); if (current()) setMessage(t("copied")); };
  const textBlock = (text: string, recovery = false) => <div className="cloud-draft-text">
    {recovery && <strong>{label("recoveredText")}</strong>}
    <p className="draft-preview">{text || label("emptyDraft")}</p>
    <div className="self-state-buttons">
      <button className="secondary" disabled={busy || !text.length} onClick={() => void run(() => onAdopt(text, localDraft))}>{label("useText")}</button>
      <button className="secondary" disabled={busy || !text.length} onClick={() => void copy(text).catch(onError)}>{label("copyText")}</button>
    </div>
  </div>;
  return <section className="self-state-panel" aria-label={label("myState")}>
    <div className="self-state-heading"><h3>{label("myState")}</h3><button className="icon-button" disabled={busy} aria-label={t("reload")} onClick={() => void run(reload)}><RefreshCw size={17} /></button></div>
    {!view ? <div className="empty"><p>{t("connectionError")}</p><button className="secondary" disabled={busy} onClick={() => void run(reload)}>{t("reload")}</button></div> : <div className="self-state-options">
      <label><input type="checkbox" checked={view.pinnedRank !== null} disabled={busy} onChange={e => change({ pinnedRank: e.target.checked ? 0 : null })} />{t("pin")}</label>
      <label><input type="checkbox" checked={typeof view.mutedUntil === "string" && Date.parse(view.mutedUntil) > Date.now()} disabled={busy} onChange={e => change({ mutedUntil: e.target.checked ? new Date(Date.now() + 86400000).toISOString().replace(/\.000Z$/, "Z") : null })} />{label("mute")} · {t("oneDay")}</label>
      <label><input type="checkbox" checked={view.archived === true} disabled={busy} onChange={e => change({ archived: e.target.checked })} />{label("archived")}</label>
      <label><input type="checkbox" checked={view.hidden === true} disabled={busy} onChange={e => change({ hidden: e.target.checked })} />{label("hidden")}</label>
      <label><input type="checkbox" checked={view.manualUnread === true} disabled={busy} onChange={e => change({ manualUnread: e.target.checked })} />{label("manualUnread")}</label>
    </div>}
    {cloudEnabled && <div className="cloud-draft">
      <h3>{label("cloudDraft")}</h3><p className="self-state-hint">{label("localPreserved")}</p>
      {draft ? <>
        {draft.contentState === "unavailable" ? <p>{t("contentUnavailable")}</p> : textBlock(typeof draft.ownedText === "string" ? draft.ownedText : "")}
        {draft.reply && <p>{label("replyReference")}</p>}
        {draft.replyState === "unavailable" && <p>{t("contentUnavailable")} · {label("replyReference")}</p>}
        {draft.recovery && textBlock(draft.recovery.ownedText, true)}
        <div className="self-state-buttons">
          <button className="secondary" disabled={busy || draft.writable !== true} onClick={() => {
            const text = localDraft, reply = selectedReply ? { id: selectedReply.id, version: selectedReply.contentVersion ?? selectedReply.version } : null;
            void run(async () => {
              const reference = reply ? await actions.selectReply(conversationId, reply.id, reply.version) : undefined;
              return actions.saveDraft(conversationId, text, reference, draft.version);
            });
          }}>{label("saveCloud")}{selectedReply ? " · " + t("reply") : ""}</button>
          <button className="secondary" disabled={busy} onClick={() => void run(() => actions.clearDraft(conversationId, draft.version))}>{label("clearCloud")}</button>
        </div>
      </> : <div className="empty"><p>{t("connectionError")}</p><button className="secondary" disabled={busy} onClick={() => void run(reload)}>{t("reload")}</button></div>}
    </div>}
    {!!pending.length && <div className="self-state-pending"><h4>{label("pendingState")}</h4>{pending.map(row => <button className="secondary" disabled={busy} key={row.requestId} onClick={() => void run(() => actions.resume(row.requestId, true))}>{label("retryOriginal")} · {row.kind === "draft" ? label("cloudDraft") : t(row.kind === "preferences" ? "font" : row.kind === "read" ? "unread" : "info")}</button>)}</div>}
    {message && <p role="status">{message}</p>}
  </section>;
}
