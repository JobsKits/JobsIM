// Created by Jobs on 2026年11月15日，星期日.
import { useEffect, useRef, useState, type FormEvent } from "react";
import { RefreshCw, ShieldCheck } from "lucide-react";
import { api, ApiError, json } from "./api";
import { loadIntent, saveIntent, clearIntent } from "./storage";
export type ReportTarget = {
  targetType: "message" | "user" | "conversation";
  targetId: string;
  label: string;
};
export type UserReport = {
  id: string;
  reporterId: string;
  targetType: ReportTarget["targetType"];
  targetId: string;
  reason: string;
  state: string;
  version: string;
  createdAt: string;
  updatedAt: string;
};
type Intent = Omit<ReportTarget, "label"> & {
  requestId: string;
  reason: string;
  restoreEpoch: string;
};

export default function ReportPanel({
  namespace,
  restoreEpoch,
  target,
  t,
  onNotice,
}: {
  namespace: string;
  restoreEpoch: string;
  target: ReportTarget | null;
  t: (key: string) => string;
  onNotice: (text: string) => void;
}) {
  const [items, setItems] = useState<UserReport[]>([]),
    [pending, setPending] = useState<Intent | null>(null),
    [busy, setBusy] = useState(false),
    [ready, setReady] = useState(false);
  const alive = useRef(true),
    working = useRef(false);
  const fail = (error: unknown) =>
    onNotice(
      error instanceof ApiError
        ? error.status === 401
          ? t("sessionExpired")
          : t("failure")
        : t("unknownResult"),
    );
  const load = async () => {
    try {
      const result = await api<{ items: UserReport[] }>("/reports");
      if (alive.current) setItems(result.items ?? []);
    } catch (error) {
      if (alive.current) fail(error);
    }
  };
  useEffect(() => {
    alive.current = true;
    void load();
    void loadIntent<Intent>(namespace, "report")
      .then(async (intent) => {
        if (intent && intent.restoreEpoch !== restoreEpoch) {
          await saveIntent(
            namespace,
            "report-archive-" + intent.restoreEpoch,
            intent,
          );
          await clearIntent(namespace, "report", intent.requestId);
          if (alive.current) onNotice(t("restoreChanged"));
        } else if (alive.current) setPending(intent ?? null);
        if (alive.current) setReady(true);
      })
      .catch((error) => {
        if (alive.current) fail(error);
      });
    return () => {
      alive.current = false;
    };
  }, [namespace, restoreEpoch]);
  const submit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (working.current || !ready) return;
    const form = event.currentTarget;
    let intent = pending;
    if (!intent) {
      if (!target) return;
      intent = {
        targetType: target.targetType,
        targetId: target.targetId,
        reason: String(new FormData(form).get("reason") ?? "").trim(),
        requestId: crypto.randomUUID(),
        restoreEpoch,
      };
      if (!intent.reason) return;
    }
    if (intent.restoreEpoch !== restoreEpoch) {
      onNotice(t("restoreChanged"));
      return;
    }
    working.current = true;
    setBusy(true);
    try {
      await saveIntent(namespace, "report", intent);
      if (!alive.current) return;
      setPending(intent);
      await api<UserReport>(
        "/reports",
        json("POST", {
          targetType: intent.targetType,
          targetId: intent.targetId,
          reason: intent.reason,
          requestId: intent.requestId,
        }),
      );
      await clearIntent(namespace, "report", intent.requestId);
      if (!alive.current) return;
      setPending(null);
      form.reset();
      onNotice(t("saved"));
      await load();
    } catch (error) {
      if (alive.current) fail(error);
    } finally {
      working.current = false;
      if (alive.current) setBusy(false);
    }
  };
  const abandon = async () => {
    if (!pending || busy || !confirm(t("unknownResult"))) return;
    await saveIntent(namespace, "report-archive-" + pending.requestId, pending);
    await clearIntent(namespace, "report", pending.requestId);
    if (alive.current) {
      setPending(null);
      void load();
    }
  };
  const candidate = pending
    ? {
        ...pending,
        label:
          target?.targetId === pending.targetId
            ? target.label
            : pending.targetId,
      }
    : target;
  const status = (value: string) =>
    t(
      (
        {
          reviewed: "reviewedReport",
          dismissed: "rejected",
          action_required: "reportActionRequired",
          submitted: "pendingReview",
        } as Record<string, string>
      )[value] ?? "pendingReview",
    );
  return (
    <>
      <h2 id="modal-title">
        <ShieldCheck size={23} /> {t(candidate ? "report" : "myReports")}
      </h2>
      {candidate && (
        <form className="report-form" onSubmit={(event) => void submit(event)}>
          <p>{candidate.label || candidate.targetId}</p>
          <label>
            {t("reportReason")}
            <textarea
              key={pending?.requestId ?? "new"}
              name="reason"
              defaultValue={pending?.reason ?? ""}
              required
              maxLength={1000}
              rows={3}
              disabled={!!pending || busy || !ready}
            />
          </label>
          <div className="admin-actions">
            {pending && (
              <button
                type="button"
                className="secondary"
                onClick={() => void abandon()}
                disabled={busy}
              >
                {t("stopRetry")}
              </button>
            )}
            <button className="primary" disabled={busy || !ready}>
              {t(busy ? "working" : pending ? "retry" : "submit")}
            </button>
          </div>
        </form>
      )}
      <div className="report-heading">
        <h3>{t("myReports")}</h3>
        <button
          className="icon-button"
          aria-label={t("reload")}
          onClick={() => void load()}
        >
          <RefreshCw size={18} />
        </button>
      </div>
      <div className="report-list">
        {items.map((item) => (
          <article className="report-card" key={item.id}>
            <div>
              <strong>{status(item.state)}</strong>
              <small>{item.targetId.slice(0, 8)}</small>
            </div>
            <p>{item.reason}</p>
          </article>
        ))}
      </div>
      {!items.length && (
        <div className="empty">
          <div className="empty-icon">
            <ShieldCheck />
          </div>
          <h2>{t("noResults")}</h2>
          <button className="secondary" onClick={() => void load()}>
            <RefreshCw size={16} />
            {t("reload")}
          </button>
        </div>
      )}
    </>
  );
}
