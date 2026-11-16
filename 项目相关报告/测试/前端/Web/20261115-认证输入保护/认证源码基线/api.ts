import {
  authBlocked,
  authHeaders,
  authProtocol,
  authRecovery,
  authSnapshot,
  authSnapshotCurrent,
  authStorageValid,
  blockAuth,
  clearPublicAuth,
  completeRefresh,
  configureAuthProtocol,
  confirmInitial,
  initialAuthIntents,
  hasAuthResponsibility,
  markRefreshPending,
  markRefreshUnknown,
  parsePublicAuth,
  pendingInitial,
  receiveInitial,
  rememberPublicAuth,
  type AuthSnapshot,
} from "./AuthSession";

let csrf = "";
let requestContext = new AbortController();
let refreshing: Promise<void> | null = null;
const configuredTimeout = Number(import.meta.env.VITE_API_TIMEOUT_MS);
const defaultTimeout =
  Number.isFinite(configuredTimeout) &&
  configuredTimeout >= 500 &&
  configuredTimeout <= 30000
    ? configuredTimeout
    : 5000;
const initialPaths = new Set([
  "/auth/login",
  "/auth/register",
  "/auth/recover",
  "/auth/reauthenticate",
]);
const publicPaths = new Set(["/bootstrap", "/capabilities"]);
const contextPaths = new Set(["/auth/context", "/auth/refresh-context"]);

export class ApiError extends Error {
  constructor(
    public status: number,
    public code: string,
    message: string,
  ) {
    super(message);
  }
}
function boundary(code: string): ApiError {
  blockAuth(code);
  return new ApiError(
    code === "AUTH_CONTEXT_CHANGED" ? 409 : 401,
    code,
    "Authentication required",
  );
}
function abortedContext() {
  return new ApiError(409, "AUTH_REQUEST_ABORTED", "Request context changed");
}
function requireIdentity(metadata: ReturnType<typeof parsePublicAuth>) {
  const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
  if (!metadata || !uuid.test(metadata.userId ?? "") || !uuid.test(metadata.deviceId ?? "") || !metadata.username)
    throw boundary("AUTH_PROTOCOL_ERROR");
}
async function responseError(response: Response): Promise<ApiError> {
  const data = await response.json().catch(() => ({}));
  return new ApiError(
    response.status,
    data.error?.code ?? "network",
    data.error?.message ?? response.statusText,
  );
}
function checkSnapshot(snapshot: AuthSnapshot, signal: AbortSignal) {
  signal.throwIfAborted();
  if (authProtocol() && !authSnapshotCurrent(snapshot)) throw abortedContext();
}
async function raw(
  path: string,
  options: RequestInit,
  snapshot: AuthSnapshot,
  signal: AbortSignal,
  suppliedCSRF?: string,
): Promise<Response> {
  const method = options.method ?? "GET";
  const headers = new Headers(options.headers);
  headers.set("X-JobsIM-Client", "web");
  if (!initialPaths.has(path))
    for (const [name, value] of Object.entries(
      authHeaders(snapshot, contextPaths.has(path)),
    ))
      headers.set(name, value);
  if (method !== "GET" && !initialPaths.has(path)) {
    let token = suppliedCSRF;
    if (token === undefined) {
      // A sibling tab must not reuse a stale CSRF value after credentials rotate.
      if (!csrf || authProtocol()) {
        const challenge = await raw("/auth/csrf", {}, snapshot, signal);
        if (!challenge.ok) throw await responseError(challenge);
        const value = await challenge.json();
        if (typeof value.csrfToken !== "string" || !value.csrfToken)
          throw new ApiError(
            503,
            "AUTH_PROTOCOL_ERROR",
            "Invalid authentication response",
          );
        checkSnapshot(snapshot, signal);
        csrf = value.csrfToken;
      }
      token = csrf;
    }
    headers.set("X-CSRF-Token", token);
  }
  if (options.body && !(options.body instanceof FormData))
    headers.set("Content-Type", "application/json");
  return fetch("/api" + path, {
    ...options,
    headers,
    credentials: "include",
    signal,
  });
}
async function locked<T>(
  signal: AbortSignal,
  action: () => Promise<T>,
): Promise<T> {
  if (!navigator.locks) throw boundary("AUTH_REFRESH_UNSUPPORTED");
  return navigator.locks.request(
    "jobsim-auth-refresh",
    { mode: "exclusive", signal },
    action,
  );
}
export async function withAuthLifecycle<T>(action: () => Promise<T>): Promise<T> {
  const signal = requestContext.signal;
  if (!navigator.locks) {
    if (authProtocol()) throw boundary("AUTH_REFRESH_UNSUPPORTED");
    return action();
  }
  return navigator.locks.request("jobsim-auth-lifecycle", { mode: "exclusive", signal }, () => {
    signal.throwIfAborted();
    return action();
  });
}
async function waitForRefresh(snapshot: AuthSnapshot, signal: AbortSignal) {
  if (refreshing) {
    await refreshing;
    checkSnapshot(snapshot, signal);
    return;
  }
  if (!authRecovery(snapshot.auth?.familyId)) return;
  await locked(signal, async () => {
    checkSnapshot(snapshot, signal);
    if (authRecovery(snapshot.auth?.familyId))
      throw boundary("AUTH_REFRESH_UNKNOWN");
  });
}
async function refreshSession(original: AuthSnapshot): Promise<void> {
  if (refreshing) return refreshing;
  const ownerContext = requestContext;
  const signal = AbortSignal.any([
    ownerContext.signal,
    AbortSignal.timeout(defaultTimeout),
  ]);
  const task = locked(signal, async () => {
    let snapshot = original;
    checkSnapshot(snapshot, signal);
    if (!authStorageValid())
      throw boundary("AUTH_REAUTH_REQUIRED");
    if (!snapshot.auth) {
      if (pendingInitial()) throw boundary("AUTH_REAUTH_REQUIRED");
      const response = await raw("/auth/refresh-context", {}, snapshot, signal);
      checkSnapshot(snapshot, signal);
      if (!response.ok) throw await responseError(response);
      const learned = parsePublicAuth(await response.json());
      checkSnapshot(snapshot, signal);
      if (!learned) throw boundary("AUTH_PROTOCOL_ERROR");
      rememberPublicAuth(learned);
      snapshot = authSnapshot();
    }
    if (!snapshot.auth) throw boundary("AUTH_REAUTH_REQUIRED");
    if (authRecovery(snapshot.auth.familyId))
      throw boundary("AUTH_REFRESH_UNKNOWN");
    // Another tab may have renewed the same family while this call waited.
    const probe = await raw("/auth/context", {}, snapshot, signal);
    checkSnapshot(snapshot, signal);
    if (probe.ok) {
      const metadata = parsePublicAuth(await probe.json());
      checkSnapshot(snapshot, signal);
      if (!metadata || metadata.familyId !== snapshot.auth.familyId)
        throw boundary("AUTH_CONTEXT_CHANGED");
      rememberPublicAuth(metadata);
      csrf = "";
      return;
    }
    const expired = await responseError(probe);
    if (expired.code !== "ACCESS_EXPIRED")
      throw boundary(
        expired.code === "AUTH_CONTEXT_CHANGED"
          ? expired.code
          : "AUTH_REAUTH_REQUIRED",
      );
    const challenge = await raw("/auth/refresh-context", {}, snapshot, signal);
    checkSnapshot(snapshot, signal);
    if (!challenge.ok) {
      const error = await responseError(challenge);
      throw boundary(
        error.code === "AUTH_CONTEXT_CHANGED"
          ? error.code
          : "AUTH_REAUTH_REQUIRED",
      );
    }
    const context = await challenge.json();
    const metadata = parsePublicAuth(context);
    checkSnapshot(snapshot, signal);
    if (
      !metadata ||
      metadata.familyId !== snapshot.auth.familyId ||
      typeof context.csrfToken !== "string"
    )
      throw boundary("AUTH_CONTEXT_CHANGED");
    markRefreshPending(snapshot);
    try {
      const response = await raw(
        "/auth/refresh",
        json("POST", {
          requestId: crypto.randomUUID(),
          familyId: snapshot.auth.familyId,
          refreshGeneration: metadata.refreshGeneration,
        }),
        snapshot,
        signal,
        context.csrfToken,
      );
      if (!response.ok) throw await responseError(response);
      const next = parsePublicAuth(await response.json());
      checkSnapshot(snapshot, signal);
      if (!next) throw Error("INVALID_AUTH_METADATA");
      completeRefresh(next, snapshot);
      csrf = "";
    } catch (error) {
      const stillCurrent = authSnapshotCurrent(snapshot);
      markRefreshUnknown(snapshot);
      if (!stillCurrent) throw abortedContext();
      if (error instanceof ApiError && error.code === "AUTH_CONTEXT_CHANGED")
        throw error;
      throw boundary("AUTH_REFRESH_UNKNOWN");
    }
  });
  refreshing = task;
  try {
    await task;
  } finally {
    if (refreshing === task) refreshing = null;
  }
}
async function request(
  path: string,
  options: RequestInit,
  timeout: number,
): Promise<{
  response: Response;
  snapshot: AuthSnapshot;
  signal: AbortSignal;
}> {
  const context = requestContext;
  const signal = AbortSignal.any([
    context.signal,
    AbortSignal.timeout(timeout),
    ...(options.signal ? [options.signal] : []),
  ]);
  let snapshot = authSnapshot();
  const initial = initialPaths.has(path);
  if (
    authProtocol() &&
    !initial &&
    !publicPaths.has(path) &&
    path !== "/auth/logout"
  ) {
    if (!navigator.locks) throw boundary("AUTH_REFRESH_UNSUPPORTED");
    if (!authStorageValid()) throw boundary("AUTH_STORAGE_UNAVAILABLE");
    if (authBlocked()) throw boundary(authBlocked());
    await waitForRefresh(snapshot, signal);
    snapshot = authSnapshot();
    if (!snapshot.auth && (!contextPaths.has(path) || pendingInitial()))
      throw boundary("AUTH_REAUTH_REQUIRED");
  }
  let response: Response;
  try {
    response = await raw(path, options, snapshot, signal);
    if (!response.ok) throw await responseError(response);
  } catch (error) {
    if (
      authProtocol() &&
      !initial &&
      !publicPaths.has(path) &&
      path !== "/auth/logout" &&
      error instanceof ApiError &&
      error.code === "ACCESS_EXPIRED"
    ) {
      await refreshSession(snapshot);
      snapshot = authSnapshot();
      signal.throwIfAborted();
      response = await raw(path, options, snapshot, signal);
      if (!response.ok) throw await responseError(response);
    } else {
      if (error instanceof ApiError && error.code === "AUTH_CONTEXT_CHANGED")
        throw authSnapshotCurrent(snapshot)
          ? boundary(error.code)
          : abortedContext();
      throw error;
    }
  }
  if (!initial && !publicPaths.has(path)) checkSnapshot(snapshot, signal);
  return { response, snapshot, signal };
}
async function jsonResponse<T>(
  path: string,
  options: RequestInit = {},
  timeout = defaultTimeout,
): Promise<T> {
  const { response, snapshot, signal } = await request(path, options, timeout);
  const data = response.status === 204 ? undefined : await response.json();
  signal.throwIfAborted();
  if (path === "/bootstrap")
    configureAuthProtocol(
      data?.features?.auth_refresh === true,
      data.deploymentId,
      data?.features?.auth_retirement_chain === true,
    );
  if (initialPaths.has(path) && path !== "/auth/recover") {
    const metadata = parsePublicAuth(data);
    if (authProtocol() && !metadata) throw boundary("AUTH_PROTOCOL_ERROR");
    if (authProtocol()) requireIdentity(metadata);
    if (metadata) {
      rememberPublicAuth(metadata);
      const body =
        typeof options.body === "string" ? JSON.parse(options.body) : {};
      receiveInitial(body.requestId, metadata.familyId);
    }
  } else if (path === "/auth/context") {
    checkSnapshot(snapshot, signal);
    const metadata = parsePublicAuth(data);
    if (authProtocol() && !metadata) throw boundary("AUTH_PROTOCOL_ERROR");
    if (authProtocol()) requireIdentity(metadata);
    if (metadata) {
      if (snapshot.auth && metadata.familyId !== snapshot.auth.familyId)
        throw boundary("AUTH_CONTEXT_CHANGED");
      rememberPublicAuth(metadata);
      confirmInitial(metadata.familyId);
    }
  } else if (path === "/auth/confirm") {
    if (snapshot.auth) confirmInitial(snapshot.auth.familyId);
  } else if (path === "/auth/logout") clearPublicAuth(snapshot);
  else if (!publicPaths.has(path)) checkSnapshot(snapshot, signal);
  return data as T;
}
export async function api<T>(
  path: string,
  options: RequestInit = {},
  timeout = defaultTimeout,
): Promise<T> {
  if (initialPaths.has(path) && path !== "/auth/recover" && !authProtocol() && hasAuthResponsibility())
    throw boundary("AUTH_RECOVERY_REQUIRED");
  if (authProtocol() && initialPaths.has(path) && path !== "/auth/recover") {
    const signal = AbortSignal.any([
      requestContext.signal,
      AbortSignal.timeout(timeout),
    ]);
    return locked(signal, async () => {
      const body = typeof options.body === "string" ? JSON.parse(options.body) : {};
      if (Array.isArray(body.previousRequestIds) && body.previousRequestIds.length) {
        const intent = initialAuthIntents().find(record => record.requestId === body.requestId);
        const live = await jsonResponse<{ deploymentId: string; features?: Record<string, boolean> }>("/bootstrap", {}, timeout);
        signal.throwIfAborted();
        if (!intent || intent.deploymentId !== live.deploymentId) throw boundary("AUTH_CONTEXT_CHANGED");
        if (live.features?.auth_retirement_chain !== true) throw boundary("AUTH_RECOVERY_REQUIRED");
      }
      return jsonResponse<T>(path, options, timeout);
    });
  }
  return jsonResponse<T>(path, options, timeout);
}
export async function apiBytes(
  path: string,
  options: RequestInit = {},
  timeout = 30000,
): Promise<Blob> {
  const { response, snapshot, signal } = await request(path, options, timeout);
  const data = await response.blob();
  checkSnapshot(snapshot, signal);
  return data;
}
export const json = (method: string, data?: unknown): RequestInit => ({
  method,
  body: data === undefined ? undefined : JSON.stringify(data),
});
export function clearCredentials() {
  csrf = "";
}
export function resetRequestContext() {
  requestContext.abort();
  requestContext = new AbortController();
  clearCredentials();
}
