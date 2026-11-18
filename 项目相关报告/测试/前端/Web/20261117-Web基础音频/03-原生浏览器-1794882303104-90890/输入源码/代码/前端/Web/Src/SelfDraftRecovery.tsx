import { useEffect, useRef, useState } from "react";
import { FileText, RefreshCw } from "lucide-react";
import { SelfStateActions, type LiveSelfState } from "./SelfStateActions";
import { selfLabel } from "./SelfStateLabels";
import { syncID, SyncProtocolError } from "./SyncProtocol";
import { loadPersonalState, type SelfOperation, type SyncIdentity } from "./SyncStore";
import type { PersonalState } from "./SelfStateProtocol";

export type RecoveryCandidate = { conversationId: string; operations: Pick<SelfOperation, "requestId" | "status">[] };

// Private metadata is a discovery hint, never current text or renewed source permission.
export function draftRecoveryCandidates(personal: PersonalState, operations: SelfOperation[], currentConversations: readonly string[], identity: SyncIdentity): RecoveryCandidate[] {
  const active = new Set(currentConversations), candidates = new Map<string, RecoveryCandidate>();
  const add = (id: string) => {
    const found = candidates.get(id) ?? { conversationId: id, operations: [] };
    candidates.set(id, found);
    return found;
  };
  for (const [key, row] of Object.entries(personal.records)) {
    if (!key.startsWith("draft|") || row.removed === true || row.scopeId !== identity.scopeId || row.userId !== identity.principalId || row.sourceContextId !== "personal" || row.revisionEpoch !== identity.restoreEpoch) continue;
    try {
      const id = syncID(row.conversationId);
      if (key !== "draft|" + syncID(row.id)) continue;
      if (!active.has(id) || row.writable === false) add(id);
    } catch { /* Invalid hints never grant a request or disclose their body. */ }
  }
  for (const operation of operations) {
    if (operation.kind !== "draft" || !["pending", "prepared", "retired"].includes(operation.status)) continue;
    try {
      const parts = operation.channel.split("|");
      if (parts.length !== 2 || parts[0] !== "draft") continue;
      const id = syncID(parts[1]);
      const body = JSON.parse(operation.originalBody) as Record<string, unknown>;
      if (body.revisionEpoch !== identity.restoreEpoch || body.requestId !== operation.requestId || operation.path !== "/conversations/" + id + "/draft" || !["PUT", "DELETE"].includes(operation.method)) continue;
      // Only identifiers/status leave this loop. In particular no pending ownedText is rendered.
      add(id).operations.push({ requestId: operation.requestId, status: operation.status });
    } catch { /* Keep corrupt original intents untouched for explicit diagnosis. */ }
  }
  return [...candidates.values()].sort((a, b) => a.conversationId.localeCompare(b.conversationId));
}

export default function SelfDraftRecovery({ actions, conversationIds, locale, t, onChanged, onError }: {
  actions: SelfStateActions; conversationIds: readonly string[]; locale: string;
  t: (key: string) => string; onChanged: () => void; onError: (error: unknown) => void;
}) {
  const label = (key: string) => selfLabel(locale, key);
  const [candidates, setCandidates] = useState<RecoveryCandidate[]>([]);
  const [selected, setSelected] = useState("");
  const [live, setLive] = useState<LiveSelfState>();
  const [busy, setBusy] = useState(false), [failed, setFailed] = useState(false), [notice, setNotice] = useState("");
  const [confirmation, setConfirmation] = useState<{ id: string; version: string }>();
  const mounted = useRef(false), lifecycle = useRef(0), reading = useRef(0), working = useRef(false);
  const selectedRef = useRef(selected), conversations = useRef(conversationIds);
  selectedRef.current = selected; conversations.current = conversationIds;
  const current = () => mounted.current && actions.context.isCurrent();
  const capture = (id?: string) => {
    const generation = lifecycle.current, original = actions;
    return new SelfStateActions({ ...original.context, isCurrent: () => mounted.current && lifecycle.current === generation && original.context.isCurrent() && (id === undefined || selectedRef.current === id) });
  };
  const refreshCandidates = async (source = capture()) => {
    const [personal, originals] = await Promise.all([
      loadPersonalState(source.context.namespace, source.context.identity, source.context.isCurrent), source.operations(),
    ]);
    if (!source.context.isCurrent()) return;
    setCandidates(draftRecoveryCandidates(personal, originals, conversations.current, source.context.identity));
  };
  const readDraft = async (id: string, source: SelfStateActions) => {
    const sequence = ++reading.current;
    setLive(undefined); setConfirmation(undefined);
    const value = await source.get("draft", id);
    if (source.context.isCurrent() && sequence === reading.current) setLive(value);
  };
  useEffect(() => {
    mounted.current = true; lifecycle.current++;
    const generation = lifecycle.current;
    const source = capture();
    setBusy(true); working.current = true;
    void refreshCandidates(source).catch(error => {
      if (source.context.isCurrent()) { setFailed(true); onError(error); }
    }).finally(() => {
      if (source.context.isCurrent()) setBusy(false);
      if (lifecycle.current === generation) working.current = false;
    });
    return () => { mounted.current = false; lifecycle.current++; reading.current++; };
  }, [actions.context.namespace]);
  const run = async (action: (source: SelfStateActions) => Promise<void>, id?: string) => {
    if (working.current || !current()) return;
    const source = capture(id);
    const generation = lifecycle.current;
    working.current = true; setBusy(true); setFailed(false); setNotice("");
    try { await action(source); }
    catch (error) {
      if (source.context.isCurrent()) { setFailed(true); onError(error); }
    } finally {
      if (source.context.isCurrent()) setBusy(false);
      if (lifecycle.current === generation) working.current = false;
    }
  };
  const choose = (id: string) => {
    if (working.current || !current()) return;
    selectedRef.current = id; setSelected(id);
    void run(source => readDraft(id, source), id);
  };
  const reload = () => void run(async source => {
    await refreshCandidates(source);
    const id = selectedRef.current;
    if (id && source.context.isCurrent()) await readDraft(id, capture(id));
  });
  const copy = (recovery = false) => {
    if (!live || !selected) return;
    const id = selected, seen = live;
    const text = recovery ? seen.recovery?.ownedText : seen.contentState === "owned" ? seen.ownedText : undefined;
    if (typeof text !== "string" || !text.length) return;
    void run(async source => {
      const fresh = await source.get("draft", id);
      if (!source.context.isCurrent()) return;
      setLive(fresh);
      const currentText = recovery ? fresh.recovery?.ownedText : fresh.contentState === "owned" ? fresh.ownedText : undefined;
      if (fresh.version !== seen.version || currentText !== text || recovery && fresh.recovery?.operationId !== seen.recovery?.operationId) throw new SyncProtocolError("SELF_STATE_VERSION_CHANGED");
      await navigator.clipboard.writeText(text);
      if (source.context.isCurrent()) setNotice(t("copied"));
    }, id);
  };
  const clear = () => {
    const seen = confirmation;
    if (!seen || seen.id !== selected) return;
    setConfirmation(undefined);
    void run(async source => {
      const result = await source.clearDraft(seen.id, seen.version);
      if (!source.context.isCurrent()) return;
      setNotice(result.status === "prepared" ? label("pendingState") : result.status === "retired" ? label("draftRecoveryStopped") : result.current ? t("saved") : label("oldResult"));
      await readDraft(seen.id, source); await refreshCandidates(source);
      if (source.context.isCurrent()) onChanged();
    }, seen.id);
  };
  const check = (requestId: string) => {
    const id = selected;
    void run(async source => {
      // This control is deliberately read-only, including when the original result is unknown.
      const result = await source.resume(requestId, false);
      if (!source.context.isCurrent()) return;
      setNotice(result.status === "prepared" ? label("pendingState") : result.status === "retired" ? label("draftRecoveryStopped") : result.current ? t("saved") : label("oldResult"));
      await readDraft(id, source); await refreshCandidates(source);
    }, id);
  };
  const selectedCandidate = candidates.find(row => row.conversationId === selected);
  const owned = live?.contentState === "owned" && typeof live.ownedText === "string" ? live.ownedText : undefined;
  if (!actions.context.isCurrent()) return null;
  return <section className="settings-section" aria-label={label("draftRecovery")}>
    <div className="self-state-heading"><h3>{label("draftRecovery")}</h3><button type="button" className="icon-button" disabled={busy} aria-label={t("reload")} onClick={reload}><RefreshCw size={17} /></button></div>
    <p className="self-state-hint">{label("draftRecoveryHint")}</p>
    {!candidates.length ? <div className="empty"><FileText size={25} /><h4>{busy ? t("working") : failed ? t("connectionError") : label("draftRecoveryEmpty")}</h4><button type="button" className="secondary" disabled={busy} onClick={reload}>{t("reload")}</button></div> : <div>
      {candidates.map(row => <button type="button" className="setting-line" key={row.conversationId} disabled={busy} aria-pressed={selected === row.conversationId}
        style={{ paddingInline: 8, borderRadius: 7, borderInlineStart: "3px solid " + (selected === row.conversationId ? "var(--accent)" : "transparent"), background: selected === row.conversationId ? "var(--accent-soft)" : undefined }}
        onClick={() => choose(row.conversationId)}>
        <span><FileText size={17} />{label("draftRecoveryConversation")} · {row.conversationId.slice(-8)}</span><span>{label("draftRecoveryOpen")}</span>
      </button>)}
    </div>}
    {selected && <div className="self-state-panel" aria-label={label("draftRecoveryCurrent")}>
      <h4>{label("draftRecoveryCurrent")}</h4><p className="self-state-hint">{label("draftRecoveryOnlyText")}</p>
      {!live ? <div className="empty"><p>{busy ? t("working") : t("connectionError")}</p><button type="button" className="secondary" disabled={busy} onClick={reload}>{t("reload")}</button></div> : <>
        <p className="draft-preview">{owned === undefined ? live.contentState === "empty" ? label("emptyDraft") : t("contentUnavailable") : owned || label("emptyDraft")}</p>
        <div className="self-state-buttons"><button type="button" className="secondary" disabled={busy || !owned?.length} onClick={() => copy(false)}>{label("copyText")}</button><button type="button" className="secondary" disabled={busy} onClick={() => setConfirmation({ id: selected, version: live.version! })}>{label("clearCloud")}</button></div>
        {live.recovery && <div className="cloud-draft-text"><strong>{label("recoveredText")}</strong><p className="draft-preview">{live.recovery.ownedText || label("emptyDraft")}</p><button type="button" className="secondary" disabled={busy || !live.recovery.ownedText.length} onClick={() => copy(true)}>{label("copyText")} · {label("recoveredText")}</button></div>}
      </>}
      {confirmation && <div role="group" aria-label={label("draftRecoveryClearConfirm")}><p>{label("draftRecoveryClearConfirm")}</p><div className="self-state-buttons"><button type="button" className="secondary" disabled={busy} onClick={clear}>{label("draftRecoveryConfirm")}</button><button type="button" className="secondary" disabled={busy} onClick={() => setConfirmation(undefined)}>{t("cancel")}</button></div></div>}
      {!!selectedCandidate?.operations.length && <div className="self-state-pending"><h4>{label("draftRecoveryCheck")}</h4>{selectedCandidate.operations.map(operation => <button type="button" className="secondary" disabled={busy} key={operation.requestId} onClick={() => check(operation.requestId)}>{label("draftRecoveryCheck")} · {operation.requestId.slice(-8)}</button>)}</div>}
      {failed && <p role="alert">{t("connectionError")}</p>}
    </div>}
    {notice && <p role="status">{notice}</p>}
  </section>;
}
