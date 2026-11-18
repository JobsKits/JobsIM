// Created by Jobs on 2026年11月15日，星期日.
import { RefreshCw, Users } from "lucide-react";
import { type User } from "./types";
export type MentionDraft = { userIds: string[]; all: boolean };
export default function MentionPicker({
  members,
  actorId,
  owner,
  value,
  t,
  onChange,
  onReload,
}: {
  members: User[];
  actorId: string;
  owner: boolean;
  value: MentionDraft;
  t: (key: string) => string;
  onChange: (value: MentionDraft) => void;
  onReload: () => void;
}) {
  const choices = members.filter(
    (user) => user.id !== actorId && (!user.state || user.state === "active"),
  );
  return (
    <>
      <div className="report-heading">
        <h2 id="modal-title">{t("mentions")}</h2>
        <button
          className="icon-button"
          aria-label={t("reload")}
          onClick={onReload}
        >
          <RefreshCw size={18} />
        </button>
      </div>
      {(owner || value.all) && (
        <label className="mention-choice">
          <input
            type="checkbox"
            checked={value.all}
            onChange={(event) =>
              onChange({ userIds: [], all: event.target.checked })
            }
          />
          {t("mentionAll")}
        </label>
      )}
      {choices.map((user) => (
        <label className="mention-choice" key={user.id}>
          <input
            type="checkbox"
            checked={value.userIds.includes(user.id)}
            disabled={
              value.all ||
              (!value.userIds.includes(user.id) && value.userIds.length >= 20)
            }
            onChange={(event) =>
              onChange({
                all: false,
                userIds: event.target.checked
                  ? [...value.userIds, user.id]
                  : value.userIds.filter((id) => id !== user.id),
              })
            }
          />
          {user.displayName || user.username}
        </label>
      ))}
      {!choices.length && (
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
    </>
  );
}
