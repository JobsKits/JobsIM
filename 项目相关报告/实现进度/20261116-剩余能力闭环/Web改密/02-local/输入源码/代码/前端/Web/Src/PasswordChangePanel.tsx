// Created by Jobs on 2026年11月16日，星期一.
import { useEffect, useRef, useState, type FormEvent } from "react";
import { LockKeyhole, RefreshCw } from "lucide-react";
import { ApiError } from "./api";
import { SyncProtocolError } from "./SyncProtocol";
import { PasswordChangeActions, type PasswordChangeContext } from "./PasswordChangeActions";
import { passwordChangeLabel } from "./PasswordChangeLabels";
import type { PasswordChangeIntent } from "./PasswordChangeProtocol";

export default function PasswordChangePanel({ context, enabled, administrator, locale, t, onLogin }: {
  context?: PasswordChangeContext; enabled: boolean; administrator: boolean; locale: string;
  t: (key: string) => string; onLogin: () => void;
}) {
  const label = (key: string) => passwordChangeLabel(locale, key);
  const [rows, setRows] = useState<PasswordChangeIntent[]>([]), [selected, setSelected] = useState<string | null>(null);
  const [username, setUsername] = useState(context?.identity?.username ?? ""), [busy, setBusy] = useState(false), [failure, setFailure] = useState("");
  const [shown, setShown] = useState(10);
  const form = useRef<HTMLFormElement>(null), mounted = useRef(false), generation = useRef(0), working = useRef(false);
  const live = useRef(context), controller = useRef<AbortController | undefined>(undefined);
  live.current = context;
  const capture = () => {
    const original = live.current, ownGeneration = generation.current;
    return original ? new PasswordChangeActions({ ...original, signal: controller.current?.signal, isCurrent: () => mounted.current && ownGeneration === generation.current && original.isCurrent() }) : undefined;
  };
  const load = async (source: PasswordChangeActions, account: string) => {
    const values = await source.rows();
    if (source.context.isCurrent()) setRows(values.filter(row => row.username === account.toLowerCase().trim()));
  };
  const errorLabel = (error: unknown) => error instanceof ApiError && error.status === 429 ? "limited"
    : error instanceof SyncProtocolError && error.code === "PASSWORD_CHANGE_PROOF_INVALID" ? "invalid" : "failed";
  const run = async (action: (source: PasswordChangeActions) => Promise<void>) => {
    const source = capture();
    if (!source || working.current) return;
    const ownGeneration = generation.current;
    working.current = true; setBusy(true); setFailure("");
    try { await action(source); }
    catch (error) { if (source.context.isCurrent()) setFailure(errorLabel(error)); }
    finally { if (source.context.isCurrent()) { setBusy(false); await load(source, username).catch(() => setFailure("failed")); } if (ownGeneration === generation.current) working.current = false; }
  };
  const reload = () => void run(source => load(source, username));
  useEffect(() => {
    mounted.current = true; generation.current++; controller.current = new AbortController(); working.current = false;
    form.current?.reset(); setBusy(false); setFailure(""); setSelected(null); setRows([]);
    const account = context?.identity?.username ?? username;
    if (context?.identity?.username) setUsername(context.identity.username);
    const source = capture(); if (source && account) void load(source, account).catch(() => { if (source.context.isCurrent()) setFailure("failed"); });
    return () => { mounted.current = false; generation.current++; controller.current?.abort(); form.current?.reset(); };
  }, [context?.scopeId, context?.identity?.familyId]);
  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const values = new FormData(event.currentTarget), action = ((event.nativeEvent as SubmitEvent).submitter as HTMLButtonElement | null)?.value ?? "check";
    const proof = { currentPassword: String(values.get("currentPassword") ?? ""), newPassword: String(values.get("newPassword") ?? ""), otp: String(values.get("otp") ?? "") };
    const confirmation = String(values.get("confirmPassword") ?? ""), consent = values.get("consent") === "on";
    event.currentTarget.reset();
    if (proof.newPassword.normalize("NFC") !== confirmation.normalize("NFC") || (action === "create" || action === "retry") && !consent) { setFailure("invalid"); return; }
    void run(async source => {
      if (selected) await source.checkOriginal(selected, proof, action === "retry");
      else if (action === "create" && enabled && source.context.identity) await source.create(proof);
      else throw Error("PASSWORD_CHANGE_SELECTION_REQUIRED");
    });
  };
  const choose = (id: string | null) => { form.current?.reset(); setFailure(""); setSelected(id); };
  const original = rows.find(row => row.requestId === selected), canCreate = enabled && !!context?.identity;
  return <section className="settings-section password-change-panel" aria-label={label("title")}>
    <div className="self-state-heading"><h3>{label("title")}</h3><button type="button" className="icon-button" disabled={busy || !context} aria-label={t("reload")} onClick={reload}><RefreshCw size={17} /></button></div>
    <p className="self-state-hint">{label("summary")}</p>
    {!canCreate && <div className="empty"><LockKeyhole size={28} /><h4>{label("unavailable")}</h4><button type="button" className="secondary" onClick={onLogin}>{t("signIn")}</button></div>}
    {canCreate && <button type="button" className="secondary" disabled={busy} onClick={() => choose(null)}>{label("submit")}</button>}
    <h4>{label("originals")}</h4>
    <div className="password-change-account"><label>{t("username")}<input type="text" value={username} autoComplete="username" onChange={event => { setUsername(event.target.value); setRows([]); choose(null); }} /></label><button type="button" className="secondary" disabled={busy || !context || !username.trim()} onClick={reload}>{t("reload")}</button></div>
    {!rows.length ? <div className="empty"><LockKeyhole size={28} /><h4>{failure ? label(failure) : label("empty")}</h4><button type="button" className="secondary" disabled={busy || !context} onClick={reload}>{t("reload")}</button></div> : rows.slice(0, shown).map(row => <article className="self-state-panel" key={row.requestId}>
      <div className="self-state-heading"><h4>{row.username} · {row.createdAt.slice(0, 16).replace("T", " ")} UTC</h4><span role="status">{label(row.state)}</span></div>
      <p className="self-state-hint">{row.requestId}</p>
      <button type="button" className="secondary" disabled={busy} aria-pressed={selected === row.requestId} onClick={() => choose(row.requestId)}>{label("select")}</button>
      {row.result?.requiresNewLogin && <button type="button" className="secondary" onClick={onLogin}>{t("signIn")}</button>}
    </article>)}
    {rows.length > shown && <button type="button" className="secondary" onClick={() => setShown(shown + 10)}>{t("loadMore")}</button>}
    {(canCreate && selected === null || original) && <form ref={form} className="password-change-form" onSubmit={submit}>
      <p className="self-state-hint">{t("username")} · {original?.username ?? context?.identity?.username}</p>
      {original && <p className="self-state-hint">{label("originalHint")}</p>}
      <label>{label("current")}<input name="currentPassword" type="password" autoComplete="current-password" required maxLength={512} /></label>
      <label>{label("new")}<input name="newPassword" type="password" autoComplete="new-password" required maxLength={512} /></label>
      <label>{label("confirm")}<input name="confirmPassword" type="password" autoComplete="new-password" required maxLength={512} /></label>
      {administrator && context?.identity && (!original || original.familyId === context.identity.familyId) && <label>{label("otp")}<input name="otp" type="text" inputMode="numeric" autoComplete="one-time-code" pattern="[0-9]{6}" maxLength={6} required={!original} /></label>}
      <label className="password-change-consent"><input name="consent" type="checkbox" />{label("consent")}</label>
      <div className="self-state-buttons">{original ? <><button type="submit" className="primary" value="check" disabled={busy}>{busy ? t("working") : label("check")}</button><button type="submit" className="secondary" value="retry" disabled={busy}>{label("retry")}</button></> : <button type="submit" className="primary" value="create" disabled={busy || !canCreate}>{busy ? t("working") : label("submit")}</button>}</div>
    </form>}
    {failure && <p role="alert">{label(failure)}</p>}
  </section>;
}
