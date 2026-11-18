// Created by Jobs on 2026年11月15日，星期日.
import { useEffect, useRef, useState } from "react";
import { Megaphone, RefreshCw } from "lucide-react";
import { api, ApiError, json } from "./api";
import { saveIntent, loadIntent, clearIntent } from "./storage";
import { type Conversation, type Message } from "./types";

type Announcement = {
  conversationId: string;
  version: string;
  state: "empty" | "active" | "expired" | "unavailable";
  canManage: boolean;
  messageId?: string;
  sourceVersion?: string;
  expiresAt?: string;
  message?: Message;
};
type Payload = {
  expectedVersion: string;
  membershipGeneration: string;
  roleGeneration?: string;
  messageId?: string;
  sourceVersion?: string;
  expiresAt?: string;
};
type Intent = {
  requestId: string;
  restoreEpoch: string;
  method: "PUT" | "DELETE";
  payload: Payload;
};
export default function AnnouncementPanel({
  conversation,
  namespace,
  restoreEpoch,
  source,
  t,
  onError,
  onNotice,
  onChoose,
}: {
  conversation: Conversation;
  namespace: string;
  restoreEpoch: string;
  source?: Message | null;
  t: (key: string) => string;
  onError: (error: unknown) => void;
  onNotice: (text: string) => void;
  onChoose: (message: Message) => Promise<void>;
}) {
  const [current, setCurrent] = useState<Announcement | null>(null),
    [pending, setPending] = useState<Intent | null>(null),
    [expiry, setExpiry] = useState("0"),
    [ready, setReady] = useState(false),
    [busy, setBusy] = useState(false);
  const alive = useRef(true),
    working = useRef(false),
    reads = useRef(0);
  const path =
    "/conversations/" + encodeURIComponent(conversation.id) + "/announcement";
  const purpose = "announcement-" + conversation.id;
  const load = async () => {
    const read = ++reads.current;
    try {
      const value = await api<Announcement>(path);
      if (
        value.conversationId !== conversation.id ||
        !/^\d+$/.test(value.version) ||
        !["empty", "active", "expired", "unavailable"].includes(value.state)
      )
        throw Error("Invalid announcement response");
      if (alive.current && read === reads.current) setCurrent(value);
    } catch (error) {
      if (alive.current && read === reads.current) {
        setCurrent(null);
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
    }, 3000);
    return () => {
      alive.current = false;
      reads.current++;
      clearInterval(timer);
    };
  }, [namespace, conversation.id, restoreEpoch]);
  useEffect(() => {
    if (current?.state !== "active" || !current.expiresAt) return;
    const version = current.version;
    const timer = setTimeout(
      () =>
        setCurrent((value) =>
          value?.version === version
            ? {
                ...value,
                state: "expired",
                message: undefined,
                messageId: undefined,
              }
            : value,
        ),
      Math.min(
        2147483647,
        Math.max(0, Date.parse(current.expiresAt) - Date.now()),
      ),
    );
    return () => clearTimeout(timer);
  }, [current?.version, current?.state, current?.expiresAt]);
  const execute = async (clear: boolean) => {
    if (working.current || !ready || !current) return;
    let intent = pending;
    if (!intent) {
      if (
        !current.canManage ||
        conversation.readOnly ||
        !conversation.membershipGeneration ||
        !conversation.roleGeneration ||
        (!clear && !source)
      )
        return;
      if (!confirm(t(clear ? "clearAnnouncement" : "setAnnouncement") + "?"))
        return;
      const payload: Payload = {
        expectedVersion: current.version,
        membershipGeneration: conversation.membershipGeneration,
        roleGeneration: conversation.roleGeneration,
      };
      if (!clear) {
        payload.messageId = source!.id;
        payload.sourceVersion = source!.contentVersion ?? source!.version;
        if (expiry !== "0")
          payload.expiresAt = new Date(
            Date.now() + Number(expiry) * 3600000,
          ).toISOString();
      }
      intent = {
        requestId: crypto.randomUUID(),
        restoreEpoch,
        method: clear ? "DELETE" : "PUT",
        payload,
      };
    }
    if (intent.restoreEpoch !== restoreEpoch) {
      onNotice(t("restoreChanged"));
      return;
    }
    working.current = true;
    reads.current++;
    setBusy(true);
    try {
      await saveIntent(namespace, purpose, intent);
      if (!alive.current) return;
      setPending(intent);
      let applied = false;
      try {
        await api(path + "/operations/" + encodeURIComponent(intent.requestId));
        applied = true;
      } catch (error) {
        if (!(error instanceof ApiError && error.status === 404)) throw error;
      }
      if (!applied) {
        const support = await api<{ features: Record<string, boolean> }>(
          "/bootstrap",
        );
        if (!alive.current) return;
        if (support.features?.role_admission !== true) {
          onNotice(t("notSupported"));
          return;
        }
        await api(
          path,
          json(intent.method, {
            ...intent.payload,
            requestId: intent.requestId,
          }),
        );
      }
      await clearIntent(namespace, purpose, intent.requestId);
      if (alive.current) {
        setPending(null);
        onNotice(t("saved"));
      }
    } catch (error) {
      if (error instanceof ApiError && [400, 403, 409].includes(error.status)) {
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
  const state = current?.state;
  const shownMessage = state === "active" ? current?.message : undefined;
  return (
    <section className="announcement-panel group-controls">
      <div className="report-heading">
        <h3>
          <Megaphone size={17} />
          {t("announcement")}
        </h3>
        <button
          className="icon-button"
          aria-label={t("reload")}
          onClick={() => void load()}
          disabled={busy}
        >
          <RefreshCw size={17} />
        </button>
      </div>
      {shownMessage ? (
        <button
          className="announcement-source"
          onClick={() => void onChoose(shownMessage).catch(onError)}
        >
          {shownMessage.contentUnavailable
            ? t("contentUnavailable")
            : shownMessage.text}
        </button>
      ) : (
        <p>
          {t(
            state === "expired"
              ? "announcementExpired"
              : state === "unavailable"
                ? "announcementUnavailable"
                : current
                  ? "noAnnouncement"
                  : "connectionError",
          )}
        </p>
      )}
      {pending ? (
        <>
          <p role="status">{t("unknownResult")}</p>
          <button
            className="secondary"
            disabled={busy || !ready}
            onClick={() => void execute(pending.method === "DELETE")}
          >
            {t("retry")}
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
          {source && current?.canManage && !conversation.readOnly && (
            <>
              <p>
                {source.contentUnavailable
                  ? t("contentUnavailable")
                  : source.text}
              </p>
              <label>
                {t("expiry")}
                <select
                  value={expiry}
                  onChange={(event) => setExpiry(event.target.value)}
                  disabled={busy}
                >
                  <option value="0">{t("noExpiry")}</option>
                  <option value="1">{t("oneHour")}</option>
                  <option value="24">{t("oneDay")}</option>
                  <option value="168">{t("oneWeek")}</option>
                </select>
              </label>
              <button
                className="primary"
                disabled={
                  busy || !ready || source.deleted || source.contentUnavailable
                }
                onClick={() => void execute(false)}
              >
                {t("setAnnouncement")}
              </button>
            </>
          )}
          {current?.canManage &&
            state !== "empty" &&
            !conversation.readOnly && (
              <button
                className="text-danger"
                disabled={busy || !ready}
                onClick={() => void execute(true)}
              >
                {t("clearAnnouncement")}
              </button>
            )}
        </>
      )}
    </section>
  );
}
