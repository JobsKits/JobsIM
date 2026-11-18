// Created by Jobs on 2026年11月17日，星期二.
import { useEffect, useRef, useState, type FormEvent } from "react";
import { RefreshCw, ShieldCheck } from "lucide-react";
import { ApiError } from "./api";
import { SyncProtocolError } from "./SyncProtocol";
import { AccountDeletionActions, type AccountDeletionContext } from "./AccountDeletionActions";
import { accountDeletionLabel } from "./AccountDeletionLabels";
import type { DeletionCase, DeletionCredential } from "./AccountDeletionProtocol";

export default function AccountDeletionPanel({ context, enabled, administrator, locale, t, onLogin }: {
  context?: AccountDeletionContext; enabled: boolean; administrator: boolean; locale: string; t: (key: string) => string; onLogin: () => void;
}) {
  const label = (key: string) => accountDeletionLabel(locale, key);
  const [rows, setRows] = useState<DeletionCase[]>([]), [selected, setSelected] = useState<string | null>(null), [username, setUsername] = useState(context?.identity?.username ?? "");
  const [busy, setBusy] = useState(false), [failure, setFailure] = useState(""), [showProof, setShowProof] = useState(false), [kind, setKind] = useState<"password" | "recoveryCode">("password"), [shown, setShown] = useState(10);
  const startForm = useRef<HTMLFormElement>(null), rightsForm = useRef<HTMLFormElement>(null), mounted = useRef(false), working = useRef(false), generation = useRef(0), controller = useRef<AbortController | undefined>(undefined), live = useRef(context);
  live.current = context;
  const capture = () => {
    const original = live.current, ownGeneration = generation.current;
    return original ? new AccountDeletionActions({ ...original, signal: controller.current?.signal, isCurrent: () => mounted.current && ownGeneration === generation.current && original.isCurrent() }) : undefined;
  };
  const load = async (source: AccountDeletionActions, account: string) => {
    const values = await source.rows();
    if (source.context.isCurrent()) setRows(values.filter(row => row.username === account.trim().toLowerCase()));
  };
  const code = (error: unknown) => error instanceof ApiError && error.status === 429 ? "limited" : error instanceof SyncProtocolError && error.code === "ACCOUNT_DELETION_PROOF_INVALID" ? "invalid" : error instanceof SyncProtocolError && error.code === "ACCOUNT_DELETION_CHECK_CANCEL_FIRST" ? "checkFirst" : "failed";
  const run = async (action: (source: AccountDeletionActions) => Promise<void>) => {
    const source = capture(), ownGeneration = generation.current;
    if (!source || working.current) return;
    working.current = true; setBusy(true); setFailure("");
    try { await action(source); }
    catch (error) { if (source.context.isCurrent()) setFailure(code(error)); }
    finally { if (source.context.isCurrent()) { setBusy(false); await load(source, username).catch(() => setFailure("failed")); } if (ownGeneration === generation.current) working.current = false; }
  };
  const reload = () => void run(source => load(source, username));
  const choose = (id: string | null) => { startForm.current?.reset(); rightsForm.current?.reset(); setShowProof(false); setSelected(id); setFailure(""); };
  useEffect(() => {
    mounted.current = true; generation.current++; controller.current = new AbortController(); working.current = false;
    startForm.current?.reset(); rightsForm.current?.reset(); setBusy(false); setFailure(""); setRows([]); setSelected(null); setShowProof(false);
    const account = context?.identity?.username ?? username;
    if (context?.identity?.username) setUsername(context.identity.username);
    const source = capture(); if (source && account) void load(source, account).catch(() => { if (source.context.isCurrent()) setFailure("failed"); });
    return () => { mounted.current = false; generation.current++; controller.current?.abort(); startForm.current?.reset(); rightsForm.current?.reset(); };
  }, [context?.scopeId, context?.identity?.familyId]);
  const begin = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const data = new FormData(event.currentTarget), proof = { password: String(data.get("password") ?? ""), otp: String(data.get("otp") ?? "") }, consent = data.get("consent") === "on";
    event.currentTarget.reset(); setShowProof(false);
    if (!consent || !proof.password || administrator && !/^\d{6}$/.test(proof.otp)) { setFailure("invalid"); return; }
    void run(async source => { if (!enabled) throw Error("ACCOUNT_DELETION_UNAVAILABLE"); await source.create(proof); });
  };
  const verify = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const data = new FormData(event.currentTarget), action = ((event.nativeEvent as SubmitEvent).submitter as HTMLButtonElement | null)?.value ?? "query";
    const proof: DeletionCredential = { [kind]: String(data.get("credential") ?? ""), otp: String(data.get("otp") ?? "") }, consent = data.get("consent") === "on";
    event.currentTarget.reset(); setShowProof(false);
    if (!selected || ["retry", "cancel"].includes(action) && !consent) { setFailure("invalid"); return; }
    void run(async source => {
      if (action === "check" || action === "retry") await source.checkOriginal(selected, proof, action === "retry");
      else if (action === "cancel") await source.cancel(selected, proof);
      else await source.query(selected, proof);
    });
  };
  const imported = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault(); const data = new FormData(event.currentTarget), id = String(data.get("deletionId") ?? "").trim(), version = String(data.get("version") ?? "").trim();
    void run(async source => { const row = await source.import(username, id, version); if (source.context.isCurrent()) choose(row.caseId); });
  };
  const row = rows.find(value => value.caseId === selected), canCreate = enabled && !!context?.identity;
  return <section className="settings-section account-deletion-panel" aria-label={label("title")}>
    <div className="self-state-heading"><h3>{label("title")}</h3><button className="icon-button" type="button" disabled={busy || !context} onClick={reload} aria-label={t("reload")}><RefreshCw size={17} /></button></div>
    <p className="self-state-hint">{label("summary")}</p>
    {canCreate ? <form ref={startForm} className="password-change-form" onSubmit={begin} noValidate>
      <p className="self-state-hint">{t("username")} · {context?.identity?.username}</p>
      <label>{t("password")}<input name="password" type="password" autoComplete="current-password" maxLength={512} /></label>
      {administrator && <label>{t("otp")}<input name="otp" inputMode="numeric" autoComplete="one-time-code" maxLength={6} /></label>}
      <label className="password-change-consent"><input name="consent" type="checkbox" />{label("consent")}</label>
      <button className="secondary deletion-danger" type="submit" disabled={busy}>{busy ? t("working") : label("start")}</button>
    </form> : <div className="empty"><ShieldCheck size={28} /><h4>{label("unavailable")}</h4><button className="secondary" type="button" onClick={onLogin}>{t("signIn")}</button></div>}
    <h4>{label("requests")}</h4>
    <div className="password-change-account"><label>{t("username")}<input value={username} autoComplete="username" onChange={event => { setUsername(event.target.value); setRows([]); choose(null); }} /></label><button className="secondary" type="button" disabled={busy || !context || !username.trim()} onClick={reload}>{t("reload")}</button></div>
    {!rows.length ? <div className="empty"><ShieldCheck size={28} /><h4>{failure ? label(failure) : label(username.trim() ? "empty" : "loadHint")}</h4><button className="secondary" type="button" disabled={busy || !context} onClick={reload}>{t("reload")}</button></div> : rows.slice(0, shown).map(value => <article className="self-state-panel" key={value.caseId}>
      <div className="self-state-heading"><h4>{value.username}</h4><span role="status">{label(value.result?.status ?? value.original?.state ?? "unknown")}</span></div>
      {value.original && <p className="self-state-hint">{label("originalNonce")} · {value.original.requestId}</p>}
      {value.observed && <><p className="self-state-hint">{label("deletionId")} · {value.observed.deletionId}</p><p className="self-state-hint">{label("version")} · {value.observed.requestVersion}</p></>}
      {value.result?.cancelUntil && <p className="self-state-hint">{label("cancelUntil")} · {value.result.cancelUntil.replace("T", " ").replace("Z", " UTC")}</p>}
      {value.cancellation && <p className="self-state-hint">{label("cancelNonce")} · {value.cancellation.requestId}</p>}
      {value.cancellation?.state === "unknown" && <p role="status">{label("cancelUnknown")}</p>}
      <button className="secondary" type="button" disabled={busy} aria-pressed={selected === value.caseId} onClick={() => choose(value.caseId)}>{label("choose")}</button>
      {value.result?.requiresNewLogin && <button className="secondary" type="button" onClick={onLogin}>{t("signIn")}</button>}
    </article>)}
    {rows.length > shown && <button type="button" className="secondary" onClick={() => setShown(shown + 10)}>{t("loadMore")}</button>}
    {row && <form ref={rightsForm} className="password-change-form" onSubmit={verify} noValidate>
      <h4>{row.username} · {row.observed?.deletionId ?? row.original?.requestId}</h4>
      <p className="self-state-hint">{label("proofHint")}</p>
      {row.original && <p className="self-state-hint">{label("originalHint")}</p>}
      <label>{t("password")} / {t("recoveryCode")}<select value={kind} onChange={event => { rightsForm.current?.reset(); setShowProof(false); setKind(event.target.value as typeof kind); }}><option value="password">{t("password")}</option><option value="recoveryCode">{t("recoveryCode")}</option></select></label>
      <label>{t(kind === "password" ? "password" : "recoveryCode")}<input key={kind} name="credential" type={showProof ? "text" : "password"} autoComplete={kind === "password" ? "current-password" : "one-time-code"} maxLength={kind === "password" ? 512 : 128} /></label>
      <label className="password-change-consent"><input type="checkbox" checked={showProof} onChange={event => setShowProof(event.target.checked)} />{t("showPassword")}</label>
      <label>{t("otp")}<input name="otp" inputMode="numeric" autoComplete="one-time-code" maxLength={6} /></label>
      <label className="password-change-consent"><input name="consent" type="checkbox" />{label("consent")}</label>
      <div className="self-state-buttons">
        {row.original && <><button type="submit" value="check" className="secondary" disabled={busy || kind !== "password"}>{label("checkOriginal")}</button><button type="submit" value="retry" className="secondary" disabled={busy || kind !== "password"}>{label("retryOriginal")}</button></>}
        <button type="submit" value="query" className="primary" disabled={busy || !row.observed}>{busy ? t("working") : label("query")}</button>
        <button type="submit" value="cancel" className="secondary deletion-danger" disabled={busy || row.result?.status !== "requested" || row.cancellation?.state === "unknown" || row.cancellation?.state === "closed" || row.cancellation?.state === "confirmed"}>{label("cancel")}</button>
      </div>
    </form>}
    <form className="password-change-form" onSubmit={imported} noValidate>
      <p className="self-state-hint">{label("loadHint")}</p>
      <label>{label("deletionId")}<input name="deletionId" autoComplete="off" spellCheck={false} /></label>
      <label>{label("version")}<input name="version" inputMode="numeric" autoComplete="off" defaultValue="1" /></label>
      <button type="submit" className="secondary" disabled={busy || !context || !username.trim()}>{label("import")}</button>
    </form>
    {failure && <p role="alert">{label(failure)}</p>}
  </section>;
}
