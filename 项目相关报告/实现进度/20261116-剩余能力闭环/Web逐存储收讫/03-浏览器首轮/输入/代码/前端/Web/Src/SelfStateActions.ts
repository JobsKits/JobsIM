import { ApiError } from "./api";
import { SyncProtocolError, syncCounter, syncID, syncRecord } from "./SyncProtocol";
import { parsePersonalState, type SelfStateKind, type SelfStateRow } from "./SelfStateProtocol";
import { ensureSyncState, finishSelfOperation, loadSelfOperations, stageSelfOperation, type SelfOperation, type SyncIdentity } from "./SyncStore";

type Request = <T>(path: string, options?: RequestInit) => Promise<T>;
export type SelfStateContext = { namespace: string; identity: SyncIdentity; isCurrent: () => boolean; request: Request };
export type LiveSelfState = SelfStateRow & {
  draftAdmission?: string; recovery?: { operationId: string; ownedText: string };
  unreadCount?: string; countSourceWatermark?: string;
};
export type SelfResult = { operationId: string; status: "prepared" | "applied" | "retired"; current: boolean; result: Record<string, unknown> };
export type ViewChanges = Partial<{ pinnedRank: number | null; mutedUntil: string | null; archived: boolean; hidden: boolean; manualUnread: boolean }>;
function error(code: string): never { throw new SyncProtocolError(code); }
const sha = async (value: string) => [...new Uint8Array(await crypto.subtle.digest("SHA-256", new TextEncoder().encode(value)))].map(byte => byte.toString(16).padStart(2, "0")).join("");
const draftLimit = (text: unknown): text is string => typeof text === "string" && new TextEncoder().encode(text).length <= 16384;
const pathFor = (kind: SelfOperation["kind"], conversation?: string) => kind === "preferences" ? "/me/preferences" : "/conversations/" + encodeURIComponent(syncID(conversation)) + "/" + kind;

export function parseLiveSelfState(value: unknown, kind: SelfOperation["kind"], context: Pick<SelfStateContext, "identity">, conversation?: string): LiveSelfState {
  const row = syncRecord(value), identity = context.identity;
  if (row.scopeId !== identity.scopeId || row.userId !== identity.principalId || row.sourceContextId !== "personal" || row.revisionEpoch !== identity.restoreEpoch || kind !== "preferences" && row.conversationId !== conversation) error("SYNC_PERSONAL_IDENTITY_MISMATCH");
  const projection = { ...row };
  const extras: Partial<LiveSelfState> = {};
  if (kind === "read") {
    if (row.countStatus !== "exact_current_source") error("SELF_READ_COUNT_UNAVAILABLE");
    extras.unreadCount = syncCounter(row.unreadCount);
    extras.countSourceWatermark = syncCounter(row.countSourceWatermark);
    delete projection.unreadCount;
    delete projection.countSourceWatermark;
    projection.countStatus = "not_computed";
  }
  if (kind === "draft") {
    if (row.draftAdmission !== undefined) {
      if (typeof row.draftAdmission !== "string" || !row.draftAdmission.startsWith("da1.") || row.draftAdmission.length > 16384) error("SELF_DRAFT_ADMISSION_INVALID");
      extras.draftAdmission = row.draftAdmission;
    }
    if (row.recovery !== undefined) {
      const recovery = syncRecord(row.recovery);
      if (!draftLimit(recovery.ownedText) || Object.keys(recovery).some(key => !["operationId", "ownedText"].includes(key))) error("SELF_DRAFT_RECOVERY_INVALID");
      extras.recovery = { operationId: syncID(recovery.operationId), ownedText: recovery.ownedText };
    }
    delete projection.draftAdmission;
    delete projection.recovery;
  }
  const projectionKind: SelfStateKind = kind === "view" ? "conversation_view" : kind === "read" ? "read_state" : kind;
  const parsed = parsePersonalState(projection, {
    owner: kind === "preferences" ? "directory" : "sync", kind: projectionKind,
    id: kind === "preferences" ? identity.principalId : syncID(row.id),
    subjectUserId: identity.principalId, conversationId: conversation,
    revision: "1", revisionEpoch: identity.restoreEpoch, state: "active",
  });
  return { ...parsed, ...extras };
}

export function validateSelfResult(value: unknown, operation: SelfOperation): SelfResult {
  const row = syncRecord(value), result = syncRecord(row.result), body = syncRecord(JSON.parse(operation.originalBody));
  const operationId = syncID(row.operationId);
  if (typeof row.current !== "boolean" || result.revisionEpoch !== body.revisionEpoch || !["prepared", "applied", "retired"].includes(String(row.status))) error("SELF_STATE_RESULT_INVALID");
  if (operation.kind !== "draft" && row.status !== "applied") error("SELF_STATE_RESULT_INVALID");
  syncCounter(result.version);
  if (operation.kind === "preferences" && (operationId !== operation.requestId || result.fontPreset !== body.fontPreset)) error("SELF_STATE_RESULT_INVALID");
  if (operation.kind === "view" && (operationId !== operation.requestId || result.conversationId !== operation.channel.slice("view|".length))) error("SELF_STATE_RESULT_INVALID");
  if (operation.kind === "read") {
    const ids = result.observedMessageIds;
    const expected = body.observedMessageIds as string[];
    if (result.membershipGeneration !== body.membershipGeneration || result.countStatus !== "not_computed" || !Array.isArray(ids) || ids.length !== expected.length || ids.some((id, index) => syncID(id) !== expected[index])) error("SELF_STATE_RESULT_INVALID");
    const point = syncRecord(result.readPoint);
    const epoch = syncCounter(point.epoch), sequence = syncCounter(point.sequence);
    if ((epoch === "0") !== (sequence === "0")) error("SELF_STATE_RESULT_INVALID");
  }
  if (operation.kind === "draft" && (!["empty", "owned", "unavailable"].includes(String(result.contentState)) || !["none", "available", "unavailable"].includes(String(result.replyState)))) error("SELF_STATE_RESULT_INVALID");
  return { operationId, status: row.status as SelfResult["status"], current: row.current, result };
}

// A context is captured by the user action before it waits for a lock. It never
// borrows a later account, source admission, CAS version or editor value.
export class SelfStateActions {
  constructor(readonly context: SelfStateContext) {}
  private check() { if (!this.context.isCurrent()) error("SYNC_CONTEXT_CHANGED"); }
  private async request<T>(path: string, init?: RequestInit): Promise<T> {
    this.check();
    const response = await this.context.request<T>(path, init);
    this.check();
    return response;
  }
  private async supported(draft = false) {
    const live = await this.request<{ deploymentId: string; features?: Record<string, boolean> }>("/bootstrap");
    if (live.deploymentId !== this.context.identity.scopeId || live.features?.sync_self_state !== true || draft && live.features?.cloud_drafts !== true) error("SYNC_SELF_STATE_UNAVAILABLE");
  }
  async get(kind: SelfOperation["kind"], conversation?: string) {
    await this.supported(kind === "draft");
    return parseLiveSelfState(await this.request(pathFor(kind, conversation)), kind, this.context, conversation);
  }
  async operations() {
    this.check();
    return loadSelfOperations(this.context.namespace, this.context.identity, this.context.isCurrent);
  }
  private async locked<T>(action: () => Promise<T>) {
    this.check();
    if (!navigator.locks) error("SYNC_BROWSER_UNSUPPORTED");
    return navigator.locks.request("jobsim-self-state|" + this.context.namespace, async () => { this.check(); return action(); });
  }
  private async complete(operation: SelfOperation, allowOriginalWrite: boolean): Promise<SelfResult> {
    this.check();
    if (await sha(operation.originalBody) !== operation.bodySHA256) error("SYNC_ORIGINAL_INTENT_CHANGED");
    const body = syncRecord(JSON.parse(operation.originalBody));
    const conversation = operation.kind === "preferences" ? undefined : operation.channel.slice(operation.kind.length + 1);
    if (operation.path !== pathFor(operation.kind, conversation) || body.requestId !== operation.requestId || body.revisionEpoch !== this.context.identity.restoreEpoch) error("SYNC_ORIGINAL_INTENT_CHANGED");
    const expectedMethod = operation.kind === "preferences" || operation.kind === "view" ? "PATCH" : "PUT";
    if (operation.method !== expectedMethod && !(operation.kind === "draft" && operation.method === "DELETE")) error("SYNC_ORIGINAL_INTENT_CHANGED");
    const state = await ensureSyncState(this.context.namespace, this.context.identity, this.context.isCurrent);
    if (state.protocolSchema !== "2" || state.restriction || state.storeGeneration !== operation.storeGeneration) error("SYNC_SELF_STATE_UNAVAILABLE");
    await this.supported(operation.kind === "draft");
    const proofBody = operation.kind === "draft" ? JSON.stringify({ method: operation.method, body }) : operation.originalBody;
    let raw: unknown;
    try {
      raw = await this.request(operation.path + "-results", { method: "POST", body: proofBody });
    } catch (failure) {
      const missing = { preferences: "PREFERENCE_OPERATION_NOT_FOUND", view: "VIEW_OPERATION_NOT_FOUND", read: "READ_OPERATION_NOT_FOUND", draft: "DRAFT_OPERATION_NOT_FOUND" }[operation.kind];
      if (!(failure instanceof ApiError && failure.status === 404 && failure.code === missing) || !allowOriginalWrite || ["applied", "retired", "rejected"].includes(operation.status)) throw failure;
      await this.supported(operation.kind === "draft");
      const beforeWrite = await ensureSyncState(this.context.namespace, this.context.identity, this.context.isCurrent);
      if (beforeWrite.restriction || beforeWrite.guardGeneration !== state.guardGeneration || beforeWrite.storeGeneration !== operation.storeGeneration) error("SYNC_LOCAL_RESTRICTION_CHANGED");
      try {
        raw = await this.request(operation.path, { method: operation.method, body: operation.originalBody });
      } catch (rejection) {
        const definite = ["PREFERENCE_VERSION_CONFLICT", "VIEW_VERSION_CONFLICT", "MANUAL_UNREAD_CHANGED", "DRAFT_VERSION_CONFLICT", "QUALIFICATION_CHANGED", "DRAFT_ADMISSION_CHANGED", "DRAFT_REPLY_ADMISSION_CHANGED", "DRAFT_REPLY_SOURCE_UNAVAILABLE", "DRAFT_SCOPE_CHANGED", "RESTORE_EPOCH_CHANGED"];
        if (!(rejection instanceof ApiError && rejection.status === 409 && definite.includes(rejection.code))) throw rejection;
        await this.supported(operation.kind === "draft");
        try { raw = await this.request(operation.path + "-results", { method: "POST", body: proofBody }); }
        catch (proofFailure) {
          if (!(proofFailure instanceof ApiError && proofFailure.status === 404 && proofFailure.code === missing)) throw proofFailure;
          await finishSelfOperation(this.context.namespace, this.context.identity, operation, state.guardGeneration, "rejected", { errorCode: rejection.code, originalResultNotFound: true }, this.context.isCurrent);
          throw rejection;
        }
      }
    }
    const result = validateSelfResult(raw, operation);
    if (operation.kind === "view") parseLiveSelfState(result.result, "view", this.context, conversation);
    await finishSelfOperation(this.context.namespace, this.context.identity, operation, state.guardGeneration, result.status, result, this.context.isCurrent);
    this.check();
    return result;
  }
  async resume(requestId: string, retryOriginal = false) {
    return this.locked(async () => {
      const operation = (await this.operations()).find(row => row.requestId === requestId);
      if (!operation) error("SELF_STATE_OPERATION_MISSING");
      return this.complete(operation, retryOriginal);
    });
  }
  private async start(kind: SelfOperation["kind"], conversation: string | undefined, make: (view: LiveSelfState) => Record<string, unknown>, method?: "DELETE", displayedVersion?: string) {
    const requestId = crypto.randomUUID();
    return this.locked(async () => {
      const channel = kind + "|" + (conversation ?? "");
      for (const old of (await this.operations()).filter(row => row.channel === channel && ["pending", "prepared"].includes(row.status))) {
        let outcome: SelfResult;
        try { outcome = await this.complete(old, false); }
        catch (failure) {
          if (failure instanceof ApiError && failure.status === 404 && ["PREFERENCE_OPERATION_NOT_FOUND", "VIEW_OPERATION_NOT_FOUND", "READ_OPERATION_NOT_FOUND", "DRAFT_OPERATION_NOT_FOUND"].includes(failure.code)) error("SELF_STATE_PENDING_OPERATION");
          throw failure;
        }
        if (outcome.status === "prepared") error("SELF_STATE_PENDING_OPERATION");
      }
      const view = await this.get(kind, conversation);
      if (displayedVersion !== undefined && view.version !== syncCounter(displayedVersion)) error("SELF_STATE_VERSION_CHANGED");
      const state = await ensureSyncState(this.context.namespace, this.context.identity, this.context.isCurrent);
      const originalBody = JSON.stringify({ requestId, revisionEpoch: view.revisionEpoch, ...make(view) });
      const operation: SelfOperation = {
        requestId, kind, channel, path: pathFor(kind, conversation),
        method: method ?? (kind === "preferences" || kind === "view" ? "PATCH" : "PUT"),
        originalBody, bodySHA256: await sha(originalBody), storeGeneration: state.storeGeneration, guardGeneration: state.guardGeneration, status: "pending",
      };
      const staged = await stageSelfOperation(this.context.namespace, this.context.identity, operation, this.context.isCurrent);
      return this.complete(staged, true);
    });
  }
  font(fontPreset: "small" | "standard" | "large") {
    return this.start("preferences", undefined, view => ({ expectedVersion: view.version, fontPreset }));
  }
  view(conversation: string, proposed: ViewChanges, displayedVersion?: string) {
    const captured = structuredClone(proposed);
    return this.start("view", conversation, view => ({ expectedVersion: view.version, changes: {
      ...captured, ...(captured.manualUnread === undefined ? {} : { expectedManualUnreadGeneration: view.manualUnreadGeneration }),
    } }), undefined, displayedVersion);
  }
  observe(conversation: string, observed: readonly string[], observedMembership: string) {
    const ids = [...new Set(observed.map(syncID))];
    if (!ids.length || ids.length > 200) error("SELF_STATE_OBSERVATION_INVALID");
    return this.start("read", conversation, view => {
      if (view.writable !== true) error("SELF_STATE_READ_ONLY");
      if (view.membershipGeneration !== syncCounter(observedMembership) || observedMembership === "0") error("SELF_STATE_MEMBERSHIP_CHANGED");
      return { membershipGeneration: view.membershipGeneration, observedMessageIds: ids };
    });
  }
  saveDraft(conversation: string, ownedText: string, selectedReply?: { messageId: string; sourceVersion: string; replyAdmission: string }, displayedVersion?: string) {
    if (!draftLimit(ownedText)) error("SELF_DRAFT_TOO_LARGE");
    const reply = selectedReply ? structuredClone(selectedReply) : null;
    if (reply && (!reply.replyAdmission.startsWith("dr1.") || syncCounter(reply.sourceVersion) === "0")) error("SELF_DRAFT_ADMISSION_INVALID");
    if (reply) syncID(reply.messageId);
    return this.start("draft", conversation, view => {
      if (view.writable !== true || !view.draftAdmission) error("SELF_STATE_READ_ONLY");
      return { expectedVersion: view.version, draftAdmission: view.draftAdmission, ownedText, reply };
    }, undefined, displayedVersion);
  }
  async selectReply(conversation: string, messageId: string, expectedContentVersion: string) {
    await this.supported(true);
    const row = syncRecord(await this.request("/messages/" + encodeURIComponent(syncID(messageId))));
    if (row.id !== messageId || row.conversationId !== conversation || row.contentVersion !== expectedContentVersion || row.deleted !== false || row.contentUnavailable === true || row.forwardUnavailable === true || typeof row.replyAdmission !== "string" || !row.replyAdmission.startsWith("dr1.")) error("SELF_DRAFT_REPLY_CHANGED");
    return { messageId, sourceVersion: syncCounter(expectedContentVersion), replyAdmission: row.replyAdmission };
  }
  clearDraft(conversation: string, displayedVersion?: string) {
    return this.start("draft", conversation, view => {
      if (!view.draftAdmission) error("SELF_STATE_READ_ONLY");
      return { expectedVersion: view.version, draftAdmission: view.draftAdmission };
    }, "DELETE", displayedVersion);
  }
}
