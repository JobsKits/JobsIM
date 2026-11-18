import type { PublicAuth } from "./AuthSession";
export type User = {
  id: string;
  username: string;
  displayName: string;
  admin?: boolean;
  version?: string;
  avatarURL?: string;
  avatarThumbnailURL?: string;
  profileVersion?: string;
  membershipGeneration?: string;
  roleGeneration?: string;
  state?: string;
  role?: string;
};
export type Conversation = {
  id: string;
  kind: "direct" | "group" | "self";
  title: string;
  ownerId: string;
  memberIds: string[];
  updatedAt: string;
  cursor: string;
  state?: string;
  readOnly?: boolean;
  membershipGeneration?: string;
  roleGeneration?: string;
  sendAdmission?: string;
};
export type Message = {
  id: string;
  conversationId: string;
  senderId: string;
  clientMsgId: string;
  type: string;
  text: string;
  replyToId?: string;
  attachmentId?: string;
  createdAt: string;
  updatedAt: string;
  version: string;
  contentVersion?: string;
  sequence: string;
  creationEpoch?: string;
  creationSequence?: string;
  deleted: boolean;
  contentUnavailable?: boolean;
  reactions: { emoji: string; userId?: string; count?: number }[];
  pinned: boolean;
  favorite: boolean;
  pending?: boolean;
  frozen?: boolean;
  freezeReason?: string;
  sendAdmission?: string;
  mentionedUserIds?: string[];
  mentionAll?: boolean;
  mentionedMe?: boolean;
  forwarded?: boolean;
  forwardUnavailable?: boolean;
  attachmentURL?: string;
  forwardAdmission?: string;
  forwardOfId?: string;
  forwardSourceVersion?: string;
  sendAttempted?: boolean;
  attachmentName?: string;
  restoreEpoch?: string;
  originFamilyId?: string;
  attachmentRequestId?: string;
};
export type Session = { user: User; deviceId: string; restoreEpoch?: string } & Partial<PublicAuth>;
export type Space = {
  syncRevision?: string;
  conversations: Conversation[];
  messages: Record<string, Message[]>;
  cursors: Record<string, string>;
  drafts: Record<string, string>;
  outbox: Message[];
  restoreEpoch?: string;
  unread?: Record<string, string[]>;
  mentionUnread?: Record<string, string[]>;
  mentionDrafts?: Record<string, { userIds: string[]; all: boolean }>;
  profiles?: Record<string, User>;
};
export const emptySpace = (): Space => ({
  conversations: [],
  messages: {},
  cursors: {},
  drafts: {},
  outbox: [],
});
