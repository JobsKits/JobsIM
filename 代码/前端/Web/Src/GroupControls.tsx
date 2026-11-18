// Created by Jobs on 2026年11月15日，星期日.
import { useEffect, useState, type ReactNode } from "react";
import { RefreshCw, Users } from "lucide-react";
import { type Conversation, type User } from "./types";
export type GroupAction = "rename" | "transfer" | "remove" | "leave";
export default function GroupControls({
  conversation,
  members,
  actorId,
  busy,
  t,
  avatar,
  onReload,
  onAction,
  governance,
  hasMore,
  onLoadMore,
}: {
  conversation: Conversation;
  members: User[];
  actorId: string;
  busy: boolean;
  t: (key: string) => string;
  avatar: (user: User) => ReactNode;
  onReload: () => void;
  onAction: (
    action: GroupAction,
    target?: User,
    title?: string,
  ) => Promise<void>;
  governance?: ReactNode;
  hasMore?: boolean;
  onLoadMore?: () => void;
}) {
  const [title, setTitle] = useState(conversation.title),
    [successor, setSuccessor] = useState("");
  useEffect(() => {
    setTitle(conversation.title);
    setSuccessor("");
  }, [conversation.id, conversation.title, conversation.ownerId]);
  const owner = conversation.ownerId === actorId;
  const available = members.filter(
    (user) => user.id !== actorId && (!user.state || user.state === "active"),
  );
  return (
    <div className="group-controls">
      {owner && !conversation.readOnly && (
        <form
          onSubmit={(event) => {
            event.preventDefault();
            void onAction("rename", undefined, title.trim());
          }}
        >
          <label>
            {t("renameGroup")}
            <input
              aria-label={t("renameGroup")}
              value={title}
              onChange={(event) => setTitle(event.target.value)}
              maxLength={60}
              required
              disabled={busy}
            />
          </label>
          <button
            className="secondary"
            disabled={busy || !title.trim() || title === conversation.title}
          >
            {t("save")}
          </button>
        </form>
      )}
      <div className="report-heading">
        <h3>
          {t("members")} <small>{members.length}</small>
        </h3>
        <button
          className="icon-button"
          aria-label={t("reload")}
          onClick={onReload}
          disabled={busy}
        >
          <RefreshCw size={17} />
        </button>
      </div>
      {members.map((user) => (
        <div className="member-row" key={user.id}>
          {avatar(user)}
          <span>
            {user.displayName || user.username}
            {user.id === conversation.ownerId && <small>{t("owner")}</small>}
          </span>
          {owner && user.id !== actorId && !conversation.readOnly && (
            <button
              className="text-danger"
              disabled={busy || !user.membershipGeneration}
              aria-label={
                t("remove") + " " + (user.displayName || user.username)
              }
              onClick={() => {
                if (
                  confirm(
                    t("remove") +
                      " " +
                      (user.displayName || user.username) +
                      "?",
                  )
                )
                  void onAction("remove", user);
              }}
            >
              {t("remove")}
            </button>
          )}
        </div>
      ))}
      {hasMore && <button className="secondary" disabled={busy} onClick={onLoadMore}>{t("loadMore")}</button>}
      {!members.length && (
        <div className="empty">
          <div className="empty-icon">
            <Users />
          </div>
          <h2>{t("noResults")}</h2>
          <button className="secondary" onClick={onReload}>
            {t("reload")}
          </button>
        </div>
      )}
      {owner && !conversation.readOnly && !governance && (
        <form
          onSubmit={(event) => {
            event.preventDefault();
            const user = available.find((user) => user.id === successor);
            if (
              user &&
              confirm(t("transferOwner") + " " + user.displayName + "?")
            )
              void onAction("transfer", user);
          }}
        >
          <label>
            {t("transferOwner")}
            <select
              value={successor}
              onChange={(event) => setSuccessor(event.target.value)}
              aria-label={t("transferOwner")}
              disabled={busy}
            >
              <option value="">{t("chooseMembers")}</option>
              {available.map((user) => (
                <option key={user.id} value={user.id}>
                  {user.displayName || user.username}
                </option>
              ))}
            </select>
          </label>
          <button className="secondary" disabled={busy || !successor}>
            {t("transferOwner")}
          </button>
        </form>
      )}
      {governance}
      {!owner && !conversation.readOnly && (
        <button
          className="text-danger"
          disabled={
            busy ||
            !members.find((user) => user.id === actorId)?.membershipGeneration
          }
          onClick={() => {
            if (confirm(t("leaveGroup") + "?"))
              void onAction(
                "leave",
                members.find((user) => user.id === actorId),
              );
          }}
        >
          {t("leaveGroup")}
        </button>
      )}
    </div>
  );
}
