// Created by Jobs on 2026年11月15日，星期日.
import type { Message } from "./types";

// A cached projection is not evidence that the source is still readable.
export function unavailableForward(message: Message): Message {
  return {
    ...message,
    text: "",
    attachmentId: undefined,
    attachmentURL: undefined,
    attachmentName: undefined,
    forwardAdmission: undefined,
    forwardUnavailable: true,
  };
}

export function attachmentPath(message: Message): string {
  if (
    message.deleted ||
    message.contentUnavailable ||
    message.forwardUnavailable
  )
    return "";
  if (message.forwarded) {
    const path =
      "/api/messages/" + encodeURIComponent(message.id) + "/attachment";
    return message.attachmentURL === path ? path : "";
  }
  return message.attachmentId?.startsWith("blob:")
    ? message.attachmentId
    : message.attachmentId
      ? "/api/media/" + message.attachmentId
      : "";
}

// History and write replies cannot reopen an unknown projection. Only the
// serialized exact-read pass establishes fresh source visibility.
export function mergeMessageProjection(
  previous: Message | undefined,
  incoming: Message,
): Message {
  if (previous && BigInt(previous.version) > BigInt(incoming.version))
    return previous;
  if (!incoming.forwarded) return incoming;
  if (
    incoming.deleted ||
    incoming.forwardUnavailable ||
    incoming.contentUnavailable
  )
    return unavailableForward(incoming);
  if (
    previous &&
    !previous.pending &&
    !previous.forwardUnavailable &&
    !previous.contentUnavailable &&
    previous.text === incoming.text &&
    previous.type === incoming.type &&
    previous.attachmentURL === incoming.attachmentURL
  )
    return incoming;
  return unavailableForward(incoming);
}

export const messagePayload = (message: Message) =>
  message.type === "forward"
    ? {
        clientMsgId: message.clientMsgId,
        type: "forward",
        forwardOfId: message.forwardOfId,
        forwardSourceVersion: message.forwardSourceVersion,
        forwardAdmission: message.forwardAdmission,
        restoreEpoch: message.restoreEpoch ?? "1",
        sendAdmission: message.sendAdmission,
      }
    : {
        clientMsgId: message.clientMsgId,
        type: message.type,
        text: message.text,
        replyToId: message.replyToId,
        attachmentId: message.attachmentId,
        restoreEpoch: message.restoreEpoch ?? "1",
        sendAdmission: message.sendAdmission,
        mentionedUserIds: message.mentionedUserIds,
        mentionAll: message.mentionAll,
      };
