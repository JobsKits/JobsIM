// Created by Jobs on 2026年11月15日，星期日.
export type PublicAuth = {
  familyId: string;
  refreshGeneration: string;
  accessExpiresAt: string;
  idleExpiresAt: string;
  absoluteExpiresAt: string;
  sourceContextId: string;
  restoreEpoch: string;
  deploymentId?: string;
  userId?: string;
  username?: string;
  deviceId?: string;
};
export type AuthSnapshot = { revision: number; auth: PublicAuth | null };
export type InitialAuth = {
  deploymentId: string;
  deviceName: string;
  kind: "login" | "register" | "reauthenticate";
  username: string;
  requestId: string;
  previousFamilyId?: string;
  previousRequestId?: string;
  previousRequestIds?: string[];
  createdAt: string;
  expired?: boolean;
  receivedFamilyId?: string;
  confirmed?: boolean;
};
export type AuthRecovery = {
  familyId: string;
  state: "pending" | "unknown";
  createdAt: string;
  username?: string;
  deploymentId?: string;
};

const tabKey = "jobsim.auth.context";
const activeKey = "jobsim.auth.active";
const intentKey = "jobsim.auth.initial-intents";
const recoveryPrefix = "jobsim.auth.recovery|";
let storageFault = false;
let revision = 0;
let supported = false;
let retirementChain = false;
let deployment = "";
let blocked = "";
const initialProofs = new Map<string, string>();
const read = <T>(storage: Storage, key: string, fallback: T): T => {
  try {
    const value = storage.getItem(key);
    return value === null ? fallback : JSON.parse(value);
  } catch {
    storageFault = true;
    return fallback;
  }
};
function write(storage: Storage, key: string, value: unknown) {
  try {
    const raw = JSON.stringify(value);
    storage.setItem(key, raw);
    if (storage.getItem(key) !== raw) throw Error("AUTH_STORAGE_UNAVAILABLE");
  } catch {
    storageFault = true;
    throw Error("AUTH_STORAGE_UNAVAILABLE");
  }
}
let active =
  read<PublicAuth | null>(sessionStorage, tabKey, null) ??
  read<PublicAuth | null>(localStorage, activeKey, null);

export function authProtocol() {
  return supported;
}
export function hasAuthResponsibility() {
  if (active?.deploymentId === deployment && active.familyId) return true;
  if (initialAuthIntents().length) return true;
  for (let i = 0; i < localStorage.length; i++) {
    const key = localStorage.key(i);
    if (!key?.startsWith(recoveryPrefix)) continue;
    const recovery = read<AuthRecovery | null>(localStorage, key, null);
    if (recovery?.deploymentId === deployment) return true;
  }
  return false;
}
export function authSnapshot(): AuthSnapshot {
  return { revision, auth: active ? { ...active } : null };
}
export function authRecovery(family = active?.familyId): AuthRecovery | null {
  return family ? read(localStorage, recoveryPrefix + family, null) : null;
}
export function recoveryForUsername(username: string): AuthRecovery | null {
  const records: AuthRecovery[] = [];
  for (let i = 0; i < localStorage.length; i++) {
    const key = localStorage.key(i);
    if (!key?.startsWith(recoveryPrefix)) continue;
    const record = read<AuthRecovery | null>(localStorage, key, null);
    if (record && (!record.deploymentId || record.deploymentId === deployment) && (!record.username || record.username.toLowerCase() === username.toLowerCase())) records.push(record);
  }
  return records.sort((a, b) => a.createdAt.localeCompare(b.createdAt)).at(-1) ?? null;
}
export function initialAuthIntents(): InitialAuth[] {
  return allInitialIntents().filter(record => record.deploymentId === deployment);
}
function allInitialIntents(): InitialAuth[] {
  const records = read<InitialAuth[]>(localStorage, intentKey, []);
  if (!Array.isArray(records)) {
    storageFault = true;
    throw Error("AUTH_STORAGE_UNAVAILABLE");
  }
  return records;
}
export function pendingInitial(username?: string): InitialAuth | undefined {
  return initialAuthIntents()
    .filter(
      (intent) =>
        !username || intent.username.toLowerCase() === username.toLowerCase(),
    )
    .at(-1);
}
export function prepareInitial(kind: "login" | "register", username: string, freshProof = false): InitialAuth {
  username = username.trim().toLowerCase();
  const previous = pendingInitial(username);
  if (previous && !previous.expired && !freshProof) {
    if (previous.previousRequestIds?.length && !retirementChain) throw Error("AUTH_RECOVERY_REQUIRED");
    if (!previous.deviceName) throw Error("AUTH_RECOVERY_REQUIRED");
    persistInitial(previous);
    return previous;
  }
  if (previous && freshProof && !retirementChain) throw Error("AUTH_RECOVERY_REQUIRED");
  const recovery = recoveryForUsername(username);
  const known = active?.deploymentId === deployment && (!active.username || active.username.toLowerCase() === username.toLowerCase()) ? active.familyId : undefined;
  const previousFamilyId = previous?.previousFamilyId ?? previous?.receivedFamilyId ?? recovery?.familyId ?? known;
  const previousRequestIds = previous && (previous.expired || freshProof)
    ? [...new Set([...(previous.previousRequestIds ?? []), ...(previous.previousRequestId ? [previous.previousRequestId] : []), previous.requestId])]
    : [];
  if (previousRequestIds.length > 64) throw Error("AUTH_RECOVERY_REQUIRED");
  const intent: InitialAuth = {
    deploymentId: deployment,
    deviceName: navigator.platform + " Web",
    kind: previous?.expired || previousFamilyId || (freshProof && previous) ? "reauthenticate" : kind,
    username, requestId: crypto.randomUUID(), createdAt: new Date().toISOString(),
    ...(previousFamilyId ? { previousFamilyId } : {}),
    ...(previous && (previous.expired || freshProof) ? { previousRequestId: previous.requestId } : {}),
    ...(retirementChain && previousRequestIds.length ? { previousRequestIds } : {}),
  };
  persistInitial(intent);
  return intent;
}
export function hasInitialProof(requestId: string): boolean {
  return initialProofs.has(requestId);
}
export function retainInitialProof(requestId: string, proof: string) {
  const previous = initialProofs.get(requestId);
  if (previous !== undefined && previous !== proof) throw Error("AUTH_ORIGINAL_PROOF_CHANGED");
  initialProofs.set(requestId, proof);
}
export function clearInitialProofs() {
  initialProofs.clear();
}
export function persistInitial(intent: InitialAuth) {
  if (storageFault) throw Error("AUTH_STORAGE_UNAVAILABLE");
  const records = allInitialIntents().filter(
    (record) => record.requestId !== intent.requestId,
  );
  if (records.length >= 64) throw Error("AUTH_RECOVERY_REQUIRED");
  write(localStorage, intentKey, [...records, intent]);
}
export function expireInitial(requestId: string) {
  const intent = initialAuthIntents().find(
    (record) => record.requestId === requestId,
  );
  if (intent) persistInitial({ ...intent, expired: true });
}
export function receiveInitial(requestId: string, familyId: string) {
  const intent = initialAuthIntents().find(
    (record) => record.requestId === requestId,
  );
  if (intent) persistInitial({ ...intent, receivedFamilyId: familyId });
}
export function confirmInitial(familyId: string) {
  for (const intent of initialAuthIntents().filter(
    (record) => record.receivedFamilyId === familyId,
  ))
    persistInitial({ ...intent, confirmed: true });
}
export function unconfirmedInitial(
  familyId = active?.familyId,
): InitialAuth | undefined {
  return initialAuthIntents().find(
    (record) => record.receivedFamilyId === familyId && !record.confirmed,
  );
}
export function unresolvedInitial(familyId = active?.familyId): boolean {
  return initialAuthIntents().some(
    (record) =>
      !record.receivedFamilyId ||
      (record.receivedFamilyId === familyId && !record.confirmed),
  );
}
export function finishInitial(requestId: string) {
  const records = allInitialIntents();
  const retired = new Set<string>();
  let current = records.find((record) => record.requestId === requestId);
  while (current && !retired.has(current.requestId)) {
    initialProofs.delete(current.requestId);
    retired.add(current.requestId);
    if (current.previousFamilyId)
      localStorage.removeItem(recoveryPrefix + current.previousFamilyId);
    current = records.find(
      (record) =>
        record.requestId === current!.previousRequestId &&
        record.username === current!.username,
    );
  }
  write(
    localStorage,
    intentKey,
    records.filter((record) => !retired.has(record.requestId)),
  );
}
export function finishActivatedInitial(familyId?: string) {
  if (!familyId) return;
  for (const intent of initialAuthIntents().filter(record => record.receivedFamilyId === familyId && record.confirmed)) finishInitial(intent.requestId);
}

export function configureAuthProtocol(enabled: boolean, deploymentId: string, supportsRetirementChain = false) {
  supported = enabled;
  retirementChain = supportsRetirementChain;
  deployment = deploymentId;
  if (active?.deploymentId && active.deploymentId !== deploymentId)
    blockAuth("AUTH_CONTEXT_CHANGED");
}
export function blockAuth(code: string) {
  if (blocked !== code) {
    blocked = code;
    revision++;
    window.dispatchEvent(
      new CustomEvent("jobsim-auth-required", { detail: { code } }),
    );
  }
}
export function authBlocked() {
  return blocked;
}
export function authStorageValid() {
  return !storageFault;
}
export function authSnapshotCurrent(snapshot: AuthSnapshot) {
  return (
    snapshot.revision === revision &&
    snapshot.auth?.familyId === active?.familyId
  );
}
export function authHeaders(
  snapshot: AuthSnapshot,
  learning = false,
): Record<string, string> {
  if (!supported) return {};
  if (snapshot.auth) return { "X-JobsIM-Auth-Family": snapshot.auth.familyId };
  return learning && !pendingInitial() ? { "X-JobsIM-Context-Learn": "1" } : {};
}
export function markRefreshPending(snapshot: AuthSnapshot) {
  if (!snapshot.auth || !authSnapshotCurrent(snapshot) || storageFault)
    throw Error("AUTH_CONTEXT_CHANGED");
  write(localStorage, recoveryPrefix + snapshot.auth.familyId, {
    familyId: snapshot.auth.familyId,
    state: "pending",
    createdAt: new Date().toISOString(),
    username: snapshot.auth.username,
    deploymentId: snapshot.auth.deploymentId,
  } satisfies AuthRecovery);
}
export function markRefreshUnknown(snapshot: AuthSnapshot) {
  if (snapshot.auth) {
    try {
      write(localStorage, recoveryPrefix + snapshot.auth.familyId, {
        familyId: snapshot.auth.familyId,
        state: "unknown",
        createdAt: new Date().toISOString(),
        username: snapshot.auth.username,
        deploymentId: snapshot.auth.deploymentId,
      } satisfies AuthRecovery);
    } finally {
      if (active?.familyId === snapshot.auth.familyId)
        blockAuth("AUTH_REFRESH_UNKNOWN");
    }
  }
}
export function parsePublicAuth(
  value: unknown,
  previous = active,
): PublicAuth | null {
  const data = value as Record<string, any>;
  if (!data) return null;
  if (data.token || data.refreshToken) throw Error("WEB_AUTH_SECRET_EXPOSED");
  if (typeof data.familyId !== "string") return null;
  if (
    !/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(
      data.familyId,
    ) ||
    typeof data.refreshGeneration !== "string" ||
    !/^(0|[1-9][0-9]*)$/.test(data.refreshGeneration)
  )
    throw Error("INVALID_AUTH_METADATA");
  for (const key of ["accessExpiresAt", "idleExpiresAt", "absoluteExpiresAt"])
    if (
      typeof data[key] !== "string" ||
      !Number.isFinite(Date.parse(data[key]))
    )
      throw Error("INVALID_AUTH_METADATA");
  if (
    typeof data.restoreEpoch !== "string" ||
    !/^[1-9][0-9]*$/.test(data.restoreEpoch) ||
    data.sourceContextId !== "personal"
  )
    throw Error("INVALID_AUTH_METADATA");
  const sameFamily = previous?.familyId === data.familyId ? previous : null;
  return {
    familyId: data.familyId,
    refreshGeneration: data.refreshGeneration,
    accessExpiresAt: data.accessExpiresAt,
    idleExpiresAt: data.idleExpiresAt,
    absoluteExpiresAt: data.absoluteExpiresAt,
    sourceContextId: data.sourceContextId,
    restoreEpoch: data.restoreEpoch,
    deploymentId: deployment,
    userId: data.user?.id ?? sameFamily?.userId,
    username: data.user?.username ?? sameFamily?.username,
    deviceId: data.deviceId ?? sameFamily?.deviceId,
  };
}
export function rememberPublicAuth(metadata: PublicAuth) {
  if (active?.familyId === metadata.familyId && BigInt(metadata.refreshGeneration) < BigInt(active.refreshGeneration)) return;
  write(sessionStorage, tabKey, metadata);
  write(localStorage, activeKey, metadata);
  const sameIdentity =
    active?.familyId === metadata.familyId &&
    active.restoreEpoch === metadata.restoreEpoch &&
    active.deploymentId === metadata.deploymentId &&
    active.sourceContextId === metadata.sourceContextId &&
    active.userId === metadata.userId;
  active = metadata;
  blocked = "";
  if (!sameIdentity) revision++;
}
export function completeRefresh(metadata: PublicAuth, snapshot: AuthSnapshot) {
  if (
    !authSnapshotCurrent(snapshot) ||
    metadata.familyId !== snapshot.auth?.familyId
  )
    throw Error("AUTH_CONTEXT_CHANGED");
  rememberPublicAuth(metadata);
  localStorage.removeItem(recoveryPrefix + metadata.familyId);
}
export function clearPublicAuth(snapshot = authSnapshot()) {
  if (snapshot.auth)
    localStorage.removeItem(recoveryPrefix + snapshot.auth.familyId);
  const global = read<PublicAuth | null>(localStorage, activeKey, null);
  if (global?.familyId === snapshot.auth?.familyId)
    localStorage.removeItem(activeKey);
  sessionStorage.removeItem(tabKey);
  active = null;
  blocked = "";
  revision++;
}

window.addEventListener("storage", (event) => {
  if (event.key === activeKey && active) {
    const next = read<PublicAuth | null>(localStorage, activeKey, null);
    if (!next || next.familyId !== active.familyId)
      blockAuth("AUTH_CONTEXT_CHANGED");
  }
});
