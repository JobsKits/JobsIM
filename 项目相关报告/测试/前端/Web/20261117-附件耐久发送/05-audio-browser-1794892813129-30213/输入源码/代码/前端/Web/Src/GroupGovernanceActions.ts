// Created by Jobs on 2026年11月17日，星期二.
import { authSnapshot } from "./AuthSession";
import { ApiError } from "./api";
import { groupBase, groupFailure, groupTarget, parseGroupGovernance, parseGroupInvite, parseGroupJoin, parseGroupPage, parseGroupShare, type GroupGovernance, type GroupInvite, type GroupMutation } from "./GroupGovernanceProtocol";
import { groupOperations, stageGroupOperation, updateGroupOperation, type GroupOperation, type GroupStoreContext } from "./GroupGovernanceStore";
import { syncCounter, syncID, syncRecord } from "./SyncProtocol";
import type { User } from "./types";

type Request = <T>(path: string, options?: RequestInit) => Promise<T>;
export type GroupGovernanceContext = GroupStoreContext & { request: Request };
const rootPath = (conversation: string) => "/conversations/" + encodeURIComponent(syncID(conversation));
function managerResult(raw: unknown, row: GroupOperation): Record<string, unknown> {
  const result = syncRecord(raw), body = syncRecord(JSON.parse(row.originalBody));
  const action = row.kind === "revoke" ? "invite_revoke" : row.kind === "decide" ? "join_decide" : body.action;
  if (result.requestId !== row.requestId || result.status !== "applied" || result.action !== action) groupFailure();
  if (row.kind === "governance") { if (syncCounter(result.version) === "0" || BigInt(String(result.version)) <= BigInt(String(body.version))) groupFailure(); return { status: "applied", requestId: row.requestId, action, version: result.version }; }
  if (result.targetId !== row.targetId || row.kind === "revoke" && result.state !== "revoked" || row.kind === "decide" && result.state !== (body.approve ? "approved" : "rejected")) groupFailure();
  if (result.membershipGeneration != null && syncCounter(result.membershipGeneration) === "0") groupFailure();
  return { status: "applied", requestId: row.requestId, action, targetId: row.targetId, state: result.state, ...(result.membershipGeneration !== undefined ? { membershipGeneration: result.membershipGeneration } : {}) };
}
export class GroupGovernanceActions {
  constructor(readonly context: GroupGovernanceContext) {}
  private check(write = false, row?: GroupOperation) {
    if (!this.context.isCurrent()) groupFailure("GROUP_CONTEXT_CHANGED");
    const auth = authSnapshot().auth, identity = this.context.identity;
    if (!auth || auth.familyId !== identity.familyId || auth.userId !== identity.userId || auth.deviceId !== identity.deviceId || auth.restoreEpoch !== identity.epoch || auth.deploymentId !== identity.scopeId || write && (!row || row.identity.familyId !== identity.familyId || row.identity.namespace !== identity.namespace)) groupFailure("GROUP_CONTEXT_CHANGED");
  }
  private async request<T>(path: string, options?: RequestInit): Promise<T> { this.check(); const result = await this.context.request<T>(path, options); this.check(); return result; }
  private async supported() { const live = await this.request<{ deploymentId: string; restoreEpoch: string; features?: Record<string, boolean> }>("/bootstrap"); if (live.deploymentId !== this.context.identity.scopeId || live.restoreEpoch !== this.context.identity.epoch || live.features?.groups !== true) groupFailure("GROUP_UNAVAILABLE"); }
  rows() { this.check(); return groupOperations(this.context); }
  async governance(conversation: string) { return parseGroupGovernance(await this.request(rootPath(conversation) + "/governance")); }
  async invites(conversation: string, after = "") { return parseGroupPage(await this.request(rootPath(conversation) + "/invites?limit=50" + (after ? "&after=" + encodeURIComponent(syncID(after)) : "")), value => parseGroupInvite(value), after); }
  async applications(conversation: string, after = "") { return parseGroupPage(await this.request(rootPath(conversation) + "/join-requests?limit=50" + (after ? "&after=" + encodeURIComponent(syncID(after)) : "")), parseGroupJoin, after); }
  async members(conversation: string, after = "") {
    return parseGroupPage(await this.request(rootPath(conversation) + "/members?limit=50" + (after ? "&after=" + encodeURIComponent(syncID(after)) : "")), raw => {
      const value = syncRecord(raw); if (typeof value.username !== "string" || typeof value.displayName !== "string" || !["owner", "admin", "member"].includes(String(value.role)) || syncCounter(value.membershipGeneration) === "0" || syncCounter(value.roleGeneration) === "0") groupFailure();
      return { id: syncID(value.id), username: value.username, displayName: value.displayName, role: value.role, membershipGeneration: value.membershipGeneration, roleGeneration: value.roleGeneration, state: value.state, avatarURL: value.avatarURL, avatarThumbnailURL: value.avatarThumbnailURL } as User;
    }, after);
  }
  private path(row: GroupOperation) { return rootPath(row.conversationId) + (row.kind === "governance" ? "/governance" : row.kind === "invite" ? "/invites" : row.kind === "join" ? "/join-requests" : row.kind === "revoke" ? "/invites/" + encodeURIComponent(row.targetId!) : "/join-requests/" + encodeURIComponent(row.targetId!)); }
  private lookup(row: GroupOperation) { return rootPath(row.conversationId) + (row.kind === "invite" ? "/invites" : row.kind === "join" ? "/join-requests" : "/governance") + "/operations/" + encodeURIComponent(row.requestId); }
  private parse(raw: unknown, row: GroupOperation) {
    if (row.kind === "invite") { const invite = parseGroupInvite(raw); if (invite.requestId !== row.requestId) groupFailure(); return invite as unknown as Record<string, unknown>; }
    if (row.kind === "join") { const joined = parseGroupJoin(raw); if (joined.requestId !== row.requestId) groupFailure(); return joined as unknown as Record<string, unknown>; }
    return managerResult(raw, row);
  }
  private async remember(row: GroupOperation, result: Record<string, unknown>) {
    const clean = row.kind === "invite" ? parseGroupInvite(result, false) as unknown as Record<string, unknown> : result;
    const saved = await updateGroupOperation(this.context, row, { state: "applied", result: clean });
    return { row: saved, result };
  }
  async resume(requestId: string, retry = false, shareText?: string, otp?: string) {
    this.check(); const row = (await this.rows()).find(value => value.requestId === requestId); if (!row) groupFailure("GROUP_STORE_INVALID");
    let found: Record<string, unknown> | undefined;
    try { found = this.parse(await this.request(this.lookup(row)), row); }
    catch (error) { if (!(error instanceof ApiError && error.status === 404 && error.code === "GROUP_OPERATION_NOT_FOUND")) throw error; }
    if (found) return this.remember(row, found);
    if (!retry || row.state === "blocked" || row.state === "applied") return { row, result: undefined };
    await this.supported(); this.check(true, row);
    const body = syncRecord(JSON.parse(row.originalBody));
    let wireBody = body;
    if (row.kind === "join") { const share = parseGroupShare(shareText ?? "", this.context.identity.scopeId); if (share.conversationId !== row.conversationId || share.inviteId !== body.inviteId) groupFailure("GROUP_INVITE_REQUIRED"); wireBody = { ...body, token: share.token }; }
    if (row.kind === "governance" && body.action === "recover") {
      if (otp && !/^\d{6}$/.test(otp)) groupFailure("GROUP_PROOF_REQUIRED");
      const proof = await this.request<{ authorizationId: string; expiresAt: string }>("/auth/step-up", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ action: "group.recover", targetId: row.conversationId, requestId: row.requestId, payload: body, otp: otp ?? "" }) });
      syncID(proof.authorizationId);
      if (!Number.isFinite(Date.parse(proof.expiresAt)) || Date.parse(proof.expiresAt) <= Date.now() || Date.parse(proof.expiresAt) > Date.now() + 130000) groupFailure("GROUP_PROOF_REQUIRED");
      wireBody = { ...body, stepUpId: proof.authorizationId };
    }
    const saved = await updateGroupOperation(this.context, row, { attempt: true }); this.check(true, saved);
    try { return await this.remember(saved, this.parse(await this.request(this.path(saved), { method: saved.kind === "revoke" ? "DELETE" : "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(wireBody) }), saved)); }
    catch (error) {
      if (error instanceof ApiError && ["GROUP_PERMISSION_CHANGED", "GROUP_TARGET_CHANGED", "GROUP_READ_ONLY", "GROUP_ADMIN_REQUIRED", "OWNER_REQUIRED", "GROUP_OPERATION_CONFLICT", "GROUP_JOIN_ALREADY_DECIDED", "STEP_UP_CONFLICT", "GROUP_INVITE_ISSUER_CHANGED"].includes(error.code)) await updateGroupOperation(this.context, saved, { state: "blocked" });
      throw error;
    }
  }
  private async create(conversationId: string, kind: GroupOperation["kind"], body: Record<string, unknown>, targetId?: string, share?: string, otp?: string) {
    this.check(); await this.supported();
    const row = await stageGroupOperation(this.context, { conversationId: syncID(conversationId), kind, requestId: syncID(body.requestId), originalBody: JSON.stringify(body), ...(targetId ? { targetId: syncID(targetId) } : {}) });
    return this.resume(row.requestId, true, share, otp);
  }
  mutate(conversation: string, snapshot: GroupGovernance, changes: Omit<GroupMutation, "requestId" | "version" | "membershipGeneration" | "roleGeneration">) {
    const body: GroupMutation = { requestId: crypto.randomUUID(), ...groupBase(snapshot), ...changes };
    return this.create(conversation, "governance", body);
  }
  target(conversation: string, snapshot: GroupGovernance, action: "mute" | "role" | "transfer", target: User, change: { role?: "admin" | "member"; mutedUntil?: string } = {}) { return this.mutate(conversation, snapshot, { action, ...groupTarget(target), ...change }); }
  invite(conversation: string, snapshot: GroupGovernance, expiresAt: string, maxUses: number, approvalRequired: boolean) { return this.create(conversation, "invite", { requestId: crypto.randomUUID(), ...groupBase(snapshot), expiresAt, maxUses, approvalRequired }); }
  revoke(conversation: string, snapshot: GroupGovernance, inviteId: string) { return this.create(conversation, "revoke", { requestId: crypto.randomUUID(), ...groupBase(snapshot) }, inviteId); }
  decide(conversation: string, snapshot: GroupGovernance, applicationId: string, approve: boolean) { return this.create(conversation, "decide", { requestId: crypto.randomUUID(), ...groupBase(snapshot), approve }, applicationId); }
  join(shareText: string) { const share = parseGroupShare(shareText, this.context.identity.scopeId); return this.create(share.conversationId, "join", { requestId: crypto.randomUUID(), inviteId: share.inviteId }, undefined, shareText); }
  recover(conversation: string, snapshot: GroupGovernance, target: User, otp: string) {
    if (!snapshot.canRecover || !/^\d{6}$/.test(otp)) groupFailure("GROUP_PROOF_REQUIRED");
    return this.create(conversation,"governance",{requestId:crypto.randomUUID(),...groupBase(snapshot),action:"recover",targetId:syncID(target.id),targetMembershipGeneration:syncCounter(target.membershipGeneration)},undefined,undefined,otp);
  }
}
