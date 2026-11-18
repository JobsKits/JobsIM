// Created by Jobs on 2026年11月15日，星期日.
import { useEffect, useRef, useState } from "react";
import { RefreshCw, UserRound } from "lucide-react";
import { api, ApiError, json } from "./api";
import { clearIntent, loadIntent, saveIntent } from "./storage";

type Relationship = {
  peerId: string;
  friend: boolean;
  blockedByMe: boolean;
  inboundGate: "open" | "denied";
  generation: string;
  inboundReopened: boolean;
};
type Intent = {
  requestId: string;
  action: "request" | "reopen";
  restoreEpoch: string;
};

export default function PeerControls({
  peerId,
  namespace,
  restoreEpoch,
  t,
  onNotice,
  onError,
  onBoundary,
  onChanged,
}: {
  peerId: string;
  namespace: string;
  restoreEpoch: string;
  t: (key: string) => string;
  onNotice: (text: string) => void;
  onError: (error: unknown) => void;
  onBoundary: () => Promise<void>;
  onChanged: () => void;
}) {
  const [relationship, setRelationship] = useState<Relationship | null>(null),
    [pending, setPending] = useState<Intent | null>(null),
    [ready, setReady] = useState(false),
    [busy, setBusy] = useState(false);
  const alive = useRef(true),
    working = useRef(false),
    reads = useRef(0);
  const purpose = "relationship-" + peerId;
  const load = async () => {
    const read = ++reads.current;
    try {
      const value = await api<Relationship>(
        "/contacts/" + encodeURIComponent(peerId) + "/relationship",
      );
      if (
        value.peerId !== peerId ||
        typeof value.friend !== "boolean" ||
        typeof value.blockedByMe !== "boolean" ||
        !["open", "denied"].includes(value.inboundGate) ||
        !/^\d+$/.test(value.generation)
      )
        throw Error("Invalid relationship response");
      if (alive.current && read === reads.current) setRelationship(value);
    } catch (error) {
      if (alive.current && read === reads.current) {
        setRelationship(null);
        onError(error);
      }
    }
  };
  useEffect(() => {
    alive.current = true;
    void load();
    void loadIntent<Intent>(namespace, purpose)
      .then(async (intent) => {
        if (intent && intent.restoreEpoch !== restoreEpoch) {
          await saveIntent(
            namespace,
            purpose + "-archive-" + intent.requestId,
            intent,
          );
          await clearIntent(namespace, purpose, intent.requestId);
          if (alive.current) onNotice(t("restoreChanged"));
        } else if (alive.current) setPending(intent ?? null);
        if (alive.current) setReady(true);
      })
      .catch((error) => {
        if (alive.current) onError(error);
      });
    const timer = setInterval(() => {
      if (!working.current) void load();
    }, 5000);
    return () => {
      alive.current = false;
      clearInterval(timer);
    };
  }, [namespace, peerId, restoreEpoch]);

  const act = async (
    action: "request" | "reopen" | "remove" | "block" | "unblock",
  ) => {
    if (working.current || !ready || !relationship) return;
    if (pending && action !== pending.action) return;
    if (
      !pending &&
      !confirm(
        t(
          action === "request"
            ? "add"
            : action === "reopen"
              ? "reopenRequests"
              : action,
        ) + "?",
      )
    )
      return;
    working.current = true;
    reads.current++;
    setBusy(true);
    let intent = pending;
    try {
      if (action === "request" || action === "reopen") {
        intent ??= { requestId: crypto.randomUUID(), action, restoreEpoch };
        await saveIntent(namespace, purpose, intent);
        if (!alive.current) return;
        setPending(intent);
      }
      if (action === "block" || action === "remove") await onBoundary();
      if (!alive.current) return;
      const peer = encodeURIComponent(peerId);
      if (action === "request")
        await api(
          "/contacts/requests",
          json("POST", { userId: peerId, requestId: intent!.requestId }),
        );
      else if (action === "reopen")
        await api(
          "/contacts/" + peer + "/reopen",
          json("POST", { requestId: intent!.requestId }),
        );
      else if (action === "remove")
        await api("/contacts/" + peer, json("DELETE"));
      else
        await api(
          "/blocks/" + peer,
          json(action === "block" ? "PUT" : "DELETE"),
        );
      if (intent) await clearIntent(namespace, purpose, intent.requestId);
      if (!alive.current) return;
      setPending(null);
      onNotice(t(action === "request" ? "requestSent" : "saved"));
      onChanged();
    } catch (error) {
      if (
        intent &&
        error instanceof ApiError &&
        [400, 403, 404, 409].includes(error.status)
      ) {
        await saveIntent(
          namespace,
          purpose + "-archive-" + intent.requestId,
          intent,
        );
        await clearIntent(namespace, purpose, intent.requestId);
        if (alive.current) setPending(null);
      }
      if (alive.current) onError(error);
    } finally {
      working.current = false;
      if (alive.current) {
        setBusy(false);
        await load();
      }
    }
  };
  const abandon = async () => {
    if (!pending || busy || !confirm(t("unknownResult"))) return;
    try {
      await saveIntent(
        namespace,
        purpose + "-archive-" + pending.requestId,
        pending,
      );
      await clearIntent(namespace, purpose, pending.requestId);
      if (alive.current) {
        setPending(null);
        void load();
      }
    } catch (error) {
      if (alive.current) onError(error);
    }
  };

  return (
    <div className="group-controls peer-controls">
      <div className="report-heading">
        <h3>{t("privacy")}</h3>
        <button
          className="icon-button"
          aria-label={t("reload")}
          disabled={busy}
          onClick={() => void load()}
        >
          <RefreshCw size={17} />
        </button>
      </div>
      {!relationship ? (
        <div className="empty">
          <div className="empty-icon">
            <UserRound />
          </div>
          <h2>{t("noResults")}</h2>
          <button className="secondary" onClick={() => void load()}>
            {t("reload")}
          </button>
        </div>
      ) : (
        <>
          <p>
            {t(
              relationship.inboundGate === "open"
                ? "inboundOpen"
                : "inboundDenied",
            )}
          </p>
          {pending ? (
            <>
              <p role="status">{t("unknownResult")}</p>
              <button
                className="secondary"
                disabled={busy || !ready}
                onClick={() => void act(pending.action)}
              >
                {t("retry")} ·{" "}
                {t(pending.action === "request" ? "add" : "reopenRequests")}
              </button>
              <button
                className="secondary"
                disabled={busy}
                onClick={() => void abandon()}
              >
                {t("stopRetry")}
              </button>
            </>
          ) : (
            <>
              {relationship.friend && (
                <button
                  className="text-danger"
                  disabled={busy || !ready}
                  onClick={() => void act("remove")}
                >
                  {t("remove")}
                </button>
              )}
              {!relationship.friend && !relationship.blockedByMe && (
                <button
                  className="secondary"
                  disabled={busy || !ready}
                  onClick={() => void act("request")}
                >
                  {t("add")}
                </button>
              )}
              <button
                className="text-danger"
                disabled={busy || !ready}
                onClick={() =>
                  void act(relationship.blockedByMe ? "unblock" : "block")
                }
              >
                {t(relationship.blockedByMe ? "unblock" : "block")}
              </button>
              {!relationship.blockedByMe &&
                relationship.inboundGate === "denied" && (
                  <button
                    className="secondary"
                    disabled={busy || !ready}
                    onClick={() => void act("reopen")}
                  >
                    {t("reopenRequests")}
                  </button>
                )}
            </>
          )}
        </>
      )}
    </div>
  );
}
