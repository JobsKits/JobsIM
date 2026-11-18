// Created by Jobs on 2026年11月15日，星期日.
import { useEffect, useRef, useState, type FormEvent } from "react";
import {
  RefreshCw,
  Search,
  ShieldCheck,
  UserRound,
  SlidersHorizontal,
} from "lucide-react";
import { api, json, ApiError } from "./api";
import { type UserReport } from "./ReportPanel";
import AdminReportDetails from "./AdminReportDetail";
import { checkedReportDetail, reportListPath, reportUUID, type AdminReportDetail } from "./AdminReportProtocol";
import { authSnapshot, authSnapshotCurrent, type AuthSnapshot } from "./AuthSession";
type AdminUser = {
  id: string;
  username: string;
  displayName: string;
  state: string;
  version: string;
  admin: boolean;
  stateBeforeTombstone?: string;
  banUntil?: string;
};
type Capabilities = {
  features: Record<string, boolean>;
  versions: Record<string, string>;
};
type Operation = {
  action: "account.state" | "capability.update" | "report.resolve";
  targetId: string;
  path: string;
  method: string;
  label: string;
  version: string;
  enabled?: boolean;
  user?: AdminUser;
};
type Pending = {
  requestId: string;
  payload: Record<string, unknown>;
  authorizationId?: string;
};
export default function AdminPanel({
  t,
  actorId,
  canWrite,
  reportsEnabled,
  onNotice,
  onFeatures,
}: {
  t: (key: string) => string;
  actorId: string;
  canWrite: boolean;
  reportsEnabled: boolean;
  onNotice: (text: string) => void;
  onFeatures: (features: Record<string, boolean>) => void;
}) {
  const [users, setUsers] = useState<AdminUser[]>([]),
    [caps, setCaps] = useState<Capabilities>({ features: {}, versions: {} }),
    [query, setQuery] = useState(""),
    [busy, setBusy] = useState(false),
    [op, setOp] = useState<Operation | null>(null),
    [state, setState] = useState("active"),
    [pendingVersion, setPendingVersion] = useState(0);
  const [reports, setReports] = useState<UserReport[]>([]);
  const [reportOwner, setReportOwner] = useState(actorId);
  const [reporterFilter, setReporterFilter] = useState(""), [targetFilter, setTargetFilter] = useState("");
  const [detail, setDetail] = useState<{ value: AdminReportDetail; owner: AuthSnapshot; actor: string; sequence: number } | null>(null);
  const [selectedReport, setSelectedReport] = useState<UserReport | null>(null), [detailBusy, setDetailBusy] = useState(false);
  const reportFilters = useRef({ reporterId: "", targetUserId: "" });
  const detailGeneration = useRef(0), detailRequest = useRef<AbortController | null>(null), detailOwner = useRef<AuthSnapshot | null>(null);
  const closeDetail = () => {
    detailGeneration.current++; detailRequest.current?.abort(); detailRequest.current = null; detailOwner.current = null;
    setDetail(null); setSelectedReport(null); setDetailBusy(false);
  };

  const reportStatus = (value: string) =>
    t(
      (
        {
          reviewed: "reviewedReport",
          dismissed: "rejected",
          action_required: "reportActionRequired",
        } as Record<string, string>
      )[value] ?? "pendingReview",
    );
  const pending = useRef<Pending | null>(null),
    generation = useRef(0);
  const fail = (error: unknown) =>
    onNotice(
      error instanceof ApiError &&
        (error.status === 401 || error.code.startsWith("MFA_"))
        ? t("otpRequired")
        : error instanceof ApiError && error.status === 409
          ? t("reload")
          : t("failure"),
    );
  const load = async () => {
    closeDetail();
    const current = ++generation.current;
    try {
      const results = await Promise.all([
        api<{ items: AdminUser[] }>(
          "/admin/users?q=" + encodeURIComponent(query),
        ),
        api<Capabilities>("/capabilities"),
        reportsEnabled
          ? api<{ items: UserReport[] }>(reportListPath(reportFilters.current.reporterId, reportFilters.current.targetUserId), { cache: "no-store", redirect: "error" })
          : Promise.resolve({ items: [] as UserReport[] }),
      ]);
      if (current !== generation.current) return;
      setUsers(results[0].items ?? []);
      setCaps(results[1]);
      setReports(results[2].items ?? []);
      setReportOwner(actorId);
      onFeatures(results[1].features);
    } catch (error) {
      if (current === generation.current) {
        setReports([]);
        closeDetail();
        fail(error);
      }
    }
  };
  useEffect(() => {
    void load();
    return () => {
      generation.current++;
    };
  }, [actorId, reportsEnabled]);
  useEffect(() => {
    closeDetail();
    const changed = () => { if (detailOwner.current && !authSnapshotCurrent(detailOwner.current)) closeDetail(); };
    const hidden = () => { if (document.hidden) closeDetail(); else changed(); };
    window.addEventListener("jobsim-auth-required", changed);
    window.addEventListener("storage", changed);
    document.addEventListener("visibilitychange", hidden);
    return () => {
      detailGeneration.current++; detailRequest.current?.abort(); detailRequest.current = null;
      window.removeEventListener("jobsim-auth-required", changed);
      window.removeEventListener("storage", changed);
      document.removeEventListener("visibilitychange", hidden);
    };
  }, [actorId, reportsEnabled]);
  const openReport = async (report: UserReport) => {
    closeDetail(); setSelectedReport(report); setDetailBusy(true);
    const current = detailGeneration.current, owner = authSnapshot(), controller = new AbortController();
    detailRequest.current = controller; detailOwner.current = owner;
    try {
      const value = checkedReportDetail(await api<AdminReportDetail>("/admin/reports/" + encodeURIComponent(report.id),
        { signal: controller.signal, cache: "no-store", redirect: "error" }), report.id);
      if (current !== detailGeneration.current || controller.signal.aborted || !authSnapshotCurrent(owner)) return;
      setDetail({ value, owner, actor: actorId, sequence: current });
    } catch (error) {
      if (current === detailGeneration.current && !controller.signal.aborted) { setDetail(null); fail(error); }
    } finally {
      if (current === detailGeneration.current) setDetailBusy(false);
    }
  };
  const filterReports = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    try {
      reportListPath(reporterFilter, targetFilter);
      reportFilters.current = { reporterId: reporterFilter.trim(), targetUserId: targetFilter.trim() };
      closeDetail(); setReports([]); void load();
    } catch (error) { fail(error); }
  };
  useEffect(() => {
    if (!op) return;
    const frame = requestAnimationFrame(() => {
      const form = document.querySelector<HTMLElement>(".admin-operation");
      form?.scrollIntoView({ block: "nearest" });
      form?.querySelector<HTMLElement>("textarea,input[name=otp]")?.focus();
    });
    return () => cancelAnimationFrame(frame);
  }, [op]);
  const select = (operation: Operation) => {
    if (pending.current && !confirm(t("unknownResult"))) return;
    pending.current = null;
    setPendingVersion((v) => v + 1);
    setOp(operation);
    setState(
      operation.action === "report.resolve"
        ? "reviewed"
        : operation.user?.state === "tombstoned"
          ? "active"
          : (operation.user?.state ?? "active"),
    );
  };
  const apply = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!op || busy || !canWrite) return;
    const form = event.currentTarget,
      data = new FormData(form);
    setBusy(true);
    try {
      let operation = pending.current;
      if (!operation) {
        const payload: Record<string, unknown> =
          op.action === "capability.update"
            ? { enabled: op.enabled, expectedVersion: op.version }
            : {
                state,
                reason: String(data.get("reason") ?? ""),
                expectedVersion: op.version,
              };
        if (state === "banned" && op.action === "account.state") {
          if (data.get("permanent")) payload.permanent = true;
          else {
            const until = new Date(String(data.get("banUntil") ?? ""));
            if (!Number.isFinite(until.valueOf()) || until <= new Date())
              throw Error("duration");
            payload.banUntil = until.toISOString();
          }
        }
        operation = { requestId: crypto.randomUUID(), payload };
        pending.current = operation;
        setPendingVersion((v) => v + 1);
      }
      if (!operation.authorizationId) {
        const proof = await api<{ authorizationId: string }>(
          "/auth/step-up",
          json("POST", {
            action: op.action,
            targetId: op.targetId,
            requestId: operation.requestId,
            otp: String(data.get("otp") ?? ""),
            payload: operation.payload,
          }),
        );
        operation.authorizationId = proof.authorizationId;
        const input = form.elements.namedItem("otp") as HTMLInputElement | null;
        if (input) input.value = "";
        setPendingVersion((v) => v + 1);
      }
      await api(
        op.path,
        json(op.method, {
          ...operation.payload,
          requestId: operation.requestId,
          stepUpId: operation.authorizationId,
        }),
      );
      pending.current = null;
      setOp(null);
      onNotice(t("saved"));
      await load();
    } catch (error) {
      if (!(error instanceof ApiError)) onNotice(t("unknownResult"));
      else fail(error);
    } finally {
      setBusy(false);
    }
  };
  const empty = (action: () => void) => (
    <div className="empty">
      <div className="empty-icon">
        <UserRound />
      </div>
      <h2>{t("noResults")}</h2>
      <button className="secondary" onClick={action}>
        <RefreshCw size={16} />
        {t("reload")}
      </button>
    </div>
  );
  const names: Record<string, string> = {
    groups: "group",
    media: "attachment",
    search: "search",
    reactions: "react",
    favorites: "favorites",
    pins: "pin",
    registration: "register",
  };
  return (
    <>
      <h2 id="modal-title">
        <ShieldCheck size={23} /> {t("admin")}
      </h2>
      {!canWrite && <p>{t("notSupported")}</p>}
      <div className="admin-columns">
        <section>
          <h3>{t("accountState")}</h3>
          <form
            className="admin-search"
            onSubmit={(event) => {
              event.preventDefault();
              void load();
            }}
          >
            <label className="search">
              <Search size={16} />
              <input
                value={query}
                onChange={(event) => setQuery(event.target.value)}
                aria-label={t("searchPeople")}
                placeholder={t("searchPeople")}
              />
            </label>
            <button className="icon-button" aria-label={t("reload")}>
              <RefreshCw size={18} />
            </button>
          </form>
          <div className="admin-users">
            {users.map((user) => (
              <button
                key={user.id}
                className="setting-line admin-user"
                disabled={!canWrite || user.id === actorId || busy}
                onClick={() =>
                  select({
                    action: "account.state",
                    targetId: user.id,
                    path:
                      "/admin/users/" + encodeURIComponent(user.id) + "/state",
                    method: "PATCH",
                    label: user.displayName || user.username,
                    version: user.version,
                    user,
                  })
                }
              >
                <span>
                  <UserRound size={18} />
                  <span>
                    {user.displayName || user.username}
                    <small>@{user.username}</small>
                  </span>
                </span>
                <span>{t(user.state)}</span>
              </button>
            ))}
            {!users.length && empty(() => void load())}
          </div>
        </section>
        <section>
          <h3>
            <SlidersHorizontal size={17} /> {t("capabilities")}
          </h3>
          {Object.entries(names).map(([key, label]) => (
            <button
              className="setting-line"
              key={key}
              disabled={!canWrite || busy}
              onClick={() =>
                select({
                  action: "capability.update",
                  targetId: key,
                  path: "/capabilities/" + key,
                  method: "PUT",
                  label: t(label),
                  version: caps.versions?.[key] ?? "0",
                  enabled: !caps.features[key],
                })
              }
            >
              <span>{t(label)}</span>
              <span>
                {caps.features[key] ? t("active") : t("notSupported")}
              </span>
            </button>
          ))}
        </section>
        {reportsEnabled && (
          <section className="admin-reports">
            <h3>{t("reports")}</h3>
            <form className="admin-report-filters" onSubmit={filterReports}>
              <label>{t("reporterId")}<input value={reporterFilter} onChange={event => setReporterFilter(event.target.value)} pattern={reportUUID.source.replace(/a-f/g, "a-fA-F")} maxLength={36} aria-label={t("reporterId")} /></label>
              <label>{t("targetUserId")}<input value={targetFilter} onChange={event => setTargetFilter(event.target.value)} pattern={reportUUID.source.replace(/a-f/g, "a-fA-F")} maxLength={36} aria-label={t("targetUserId")} /></label>
              <div className="admin-report-filter-actions">
                <button className="secondary" type="submit">{t("filterReports")}</button>
                <button className="secondary" type="button" onClick={() => { setReporterFilter(""); setTargetFilter(""); reportFilters.current = { reporterId: "", targetUserId: "" }; closeDetail(); setReports([]); void load(); }}>{t("clearReportFilters")}</button>
              </div>
            </form>
            {(reportOwner === actorId ? reports : []).map((item) => (
              <button className="setting-line" key={item.id} disabled={busy} onClick={() => void openReport(item)}>
                <span><span>{item.reason}<small>{item.targetId.slice(0, 8)}</small></span></span>
                <span>{reportStatus(item.state)}</span>
              </button>
            ))}
            {(!reports.length || reportOwner !== actorId) && empty(() => void load())}
          </section>
        )}
      </div>
      {reportsEnabled && selectedReport && !detail && <section className="admin-report-detail" aria-live="polite">
        <header><h3>{t("reportDetail")}</h3><button type="button" className="secondary" onClick={closeDetail}>{t("close")}</button></header>
        <p>{t(detailBusy ? "working" : "contentUnavailable")}</p>
        {!detailBusy && <button type="button" className="secondary" onClick={() => void openReport(selectedReport)}>{t("reload")}</button>}
      </section>}
      {reportsEnabled && detail && detail.actor === actorId && <AdminReportDetails key={detail.value.report.id + "|" + detail.owner.revision + "|" + detail.sequence} detail={detail.value} owner={detail.owner} t={t} canWrite={canWrite && !busy}
        onClose={closeDetail} onInvalid={() => { closeDetail(); onNotice(t("contentUnavailable")); }} onResolve={() => select({ action: "report.resolve", targetId: detail.value.report.id,
          path: "/admin/reports/" + encodeURIComponent(detail.value.report.id), method: "PATCH", label: detail.value.report.reason, version: detail.value.report.version })} />}
      {op && (
        <form
          className="admin-operation"
          onSubmit={(event) => void apply(event)}
          data-pending={pendingVersion}
        >
          <h3>{op.label}</h3>
          <fieldset disabled={!!pending.current || busy}>
            {op.action === "account.state" || op.action === "report.resolve" ? (
              <>
                <label>
                  {t(op.action === "account.state" ? "accountState" : "report")}
                  <select
                    value={state}
                    onChange={(event) => setState(event.target.value)}
                  >
                    {(op.action === "report.resolve"
                      ? ["reviewed", "dismissed", "action_required"]
                      : ["active", "banned", "tombstoned"]
                    ).map((value) => (
                      <option key={value} value={value}>
                        {op.action === "report.resolve"
                          ? reportStatus(value)
                          : t(value)}
                      </option>
                    ))}
                  </select>
                </label>
                <label>
                  {t("reason")}
                  <textarea name="reason" required maxLength={1000} rows={2} />
                </label>
                {state === "banned" && op.action === "account.state" && (
                  <>
                    <label className="inline-check">
                      <input type="checkbox" name="permanent" />
                      {t("permanent")}
                    </label>
                    <input
                      type="datetime-local"
                      name="banUntil"
                      aria-label={t("banned")}
                    />
                  </>
                )}
              </>
            ) : (
              <p>{t(op.enabled ? "active" : "notSupported")}</p>
            )}
          </fieldset>
          <label>
            {t("otp")}
            <input
              name="otp"
              required={!pending.current?.authorizationId}
              inputMode="numeric"
              pattern="[0-9]{6}"
              minLength={6}
              maxLength={6}
              autoComplete="one-time-code"
              placeholder={t("otpHint")}
            />
          </label>
          <div className="admin-actions">
            <button
              type="button"
              className="secondary"
              onClick={() => {
                if (pending.current && !confirm(t("unknownResult"))) return;
                pending.current = null;
                setOp(null);
              }}
            >
              {t("cancel")}
            </button>
            <button className="primary" disabled={busy || !canWrite}>
              {t(
                busy
                  ? "working"
                  : pending.current?.authorizationId
                    ? "retry"
                    : "submit",
              )}
            </button>
          </div>
        </form>
      )}
    </>
  );
}
