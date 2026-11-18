// Created by Jobs on 2026年11月17日，星期二.
import { api, ApiError, json } from "./api";
import { authSnapshot, authSnapshotCurrent } from "./AuthSession";
import { attachmentAudioBytes, attachmentIdentity, attachmentIntent, uploadManifest, uploadResult, type AttachmentSendIntent } from "./AttachmentSendProtocol";
import { attachmentIntents, prepareAttachmentIntent, updateAttachmentIntent, type AttachmentStoreContext } from "./AttachmentSendStore";
import { messagePayload } from "./forward";
import { validOriginalMessageResult } from "./SyncView";
import type { Message } from "./types";

export type AttachmentSendContext = AttachmentStoreContext & {
  request?: typeof api; signal?: AbortSignal;
  enqueue: (entry: AttachmentSendIntent) => Promise<void>;
  accepted: (entry: AttachmentSendIntent, message: Message) => Promise<void>;
};
export async function attachmentDigest(blob: Blob): Promise<string> {
  return [...new Uint8Array(await crypto.subtle.digest("SHA-256", await blob.arrayBuffer()))].map(value => value.toString(16).padStart(2, "0")).join("");
}
export class AttachmentSendActions {
  context: AttachmentSendContext;
  constructor(context: AttachmentSendContext) { this.context = context; attachmentIdentity(context.identity); }
  private current() {
    const { identity } = this.context, auth = authSnapshot().auth;
    return !this.context.signal?.aborted && this.context.isCurrent() && !!auth && auth.familyId === identity.familyId && auth.userId === identity.userId && auth.deviceId === identity.deviceId && auth.restoreEpoch === identity.restoreEpoch && auth.sourceContextId === "personal" && auth.deploymentId === identity.namespace.split("|")[1];
  }
  private check() { if (!this.current()) throw Error("ATTACHMENT_CONTEXT_CHANGED"); }
  async prepare(file: File, message: Message, replyContentVersion?: string): Promise<AttachmentSendIntent> {
    this.check();
    if (!file.size || file.size > 64 * 1024 * 1024 || message.type === "audio" && file.size > attachmentAudioBytes) throw Error("ATTACHMENT_QUOTA");
    const sha256 = await attachmentDigest(file); this.check();
    const name = file.name.normalize("NFC"), identity = attachmentIdentity(this.context.identity), requestId = crypto.randomUUID(), now = Date.now();
    const manifest = uploadManifest({ requestId, userId: identity.userId, deviceId: identity.deviceId, familyId: identity.familyId, restoreEpoch: identity.restoreEpoch, name, size: String(file.size), sha256 });
    const entry = attachmentIntent({ requestId, clientMsgId: message.clientMsgId, identity, manifest, message: { ...message, text: name }, replyContentVersion, blob: file.slice(0, file.size, file.type), state: "prepared", revision: "1", createdAt: now, updatedAt: now });
    return prepareAttachmentIntent({ identity, isCurrent: () => this.current() }, entry);
  }
  async rows() { return attachmentIntents({ identity: this.context.identity, isCurrent: () => this.current() }); }
  private async original(requestId: string) {
    this.check(); const entry = (await this.rows()).find(entry => entry.requestId === requestId); this.check();
    if (!entry || JSON.stringify(entry.identity) !== JSON.stringify(attachmentIdentity(this.context.identity))) throw Error("ATTACHMENT_CONTEXT_CHANGED");
    return entry;
  }
  async cancel(requestId: string) { const entry = await this.original(requestId); await updateAttachmentIntent({ identity: entry.identity, isCurrent: () => this.current() }, entry, { cancel: true }); }
  async run(requestId: string, explicitRetry: boolean): Promise<void> {
    this.check();
    if (!navigator.locks) throw Error("ATTACHMENT_LOCK_UNAVAILABLE");
    await navigator.locks.request("jobsim-attachment|" + this.context.identity.namespace + "|" + requestId, { signal: this.context.signal }, async () => {
      let entry = await this.original(requestId);
      if (["accepted", "cancelled", "retired"].includes(entry.state)) return;
      const snapshot = authSnapshot(), current = () => this.current() && authSnapshotCurrent(snapshot), request = this.context.request ?? api;
      const guard = () => { if (!current()) throw Error("ATTACHMENT_CONTEXT_CHANGED"); };
      const context = { identity: entry.identity, isCurrent: current };
      try {
        if (entry.state === "enqueued") {
          const result = await request<Message>("/conversations/" + entry.message.conversationId + "/message-results", { ...json("POST", messagePayload(entry.message)), signal: this.context.signal }); guard();
          if (!validOriginalMessageResult(result, entry.identity.userId, entry.message.conversationId, entry.clientMsgId)) throw Error("ATTACHMENT_MESSAGE_BINDING_INVALID");
          await this.context.accepted(entry, result); return;
        }
        if (explicitRetry && (!entry.blob || await attachmentDigest(entry.blob) !== entry.manifest.sha256)) throw Error("ATTACHMENT_BLOB_CHANGED");
        guard();
        const cap = await request<Record<string, unknown>>("/media/capabilities", { signal: this.context.signal }); guard();
        if (cap.protocol !== "stable-upload-v1" || cap.supported !== true || cap.resultPath !== "/api/media/upload-results" || cap.maxBytes !== "67108864" || cap.maxRequestBytes !== "68157440") throw Error("ATTACHMENT_CAPABILITY_REQUIRED");
        let missing = false, result: unknown;
        try { result = await request("/media/upload-results", { ...json("POST", uploadManifest(entry.manifest)), signal: this.context.signal }); }
        catch (error) { if (error instanceof ApiError && error.status === 404 && error.code === "UPLOAD_NOT_FOUND") missing = true; else throw error; }
        guard();
        if (!missing) {
          const parsed = uploadResult(result, entry);
          if (!parsed.prepared) entry = await updateAttachmentIntent(context, entry, { state: "uploaded", receipt: parsed.receipt });
          else {
            entry = await updateAttachmentIntent(context, entry, { state: "unknown", preparedId: parsed.id });
            if (!explicitRetry) return;
          }
        } else if (!explicitRetry) return;
        if (missing || entry.state !== "uploaded") {
          if (!explicitRetry || !entry.blob) return;
          entry = await updateAttachmentIntent(context, entry, { state: "unknown" }); guard();
          const form = new FormData(); for (const [key, value] of Object.entries(uploadManifest(entry.manifest))) form.append(key, value); form.append("file", entry.blob, entry.manifest.name);
          const parsed = uploadResult(await request("/media", { method: "POST", body: form, signal: this.context.signal }, 120000), entry); guard();
          if (parsed.prepared) { await updateAttachmentIntent(context, entry, { state: "unknown", preparedId: parsed.id }); return; }
          entry = await updateAttachmentIntent(context, entry, { state: "uploaded", receipt: parsed.receipt });
        }
        guard();
        // A lookup is read-only, including when an uploaded object is already committed.
        if (!explicitRetry) return;
        if (entry.message.replyToId) {
          const source = await request<Message>("/messages/" + entry.message.replyToId, { signal: this.context.signal }); guard();
          if (source.deleted || source.contentUnavailable || source.forwardUnavailable || source.conversationId !== entry.message.conversationId || (source.contentVersion ?? source.version) !== entry.replyContentVersion) throw Error("ATTACHMENT_REPLY_CHANGED");
        }
        await this.context.enqueue(entry);
      } catch (error) {
        if (current()) {
          const latest = (await this.rows()).find(value => value.requestId === requestId);
          if (latest && !["accepted", "cancelled", "retired", "enqueued"].includes(latest.state)) {
            const retired = error instanceof ApiError && error.status === 410 && error.code === "UPLOAD_RETIRED";
            const failure = retired ? "retired" : String(error) === "Error: ATTACHMENT_CAPABILITY_REQUIRED" ? "capability" : String(error) === "Error: ATTACHMENT_REPLY_CHANGED" ? "replyChanged" : "unknown";
            await updateAttachmentIntent(context, latest, { state: retired ? "retired" : latest.state, failure }).catch(() => {});
          }
        }
        throw error;
      }
    });
  }
}
