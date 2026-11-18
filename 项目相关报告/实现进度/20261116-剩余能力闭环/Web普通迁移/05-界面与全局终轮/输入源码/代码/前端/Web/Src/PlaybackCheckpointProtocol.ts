// Created by Jobs on 2026年11月17日，星期二.
export const playbackCheckpointLimit = 256;
export type PlaybackBinding = { namespace: string; messageId: string; conversationId: string; contentVersion: string; mediaDigest: string; kind: "audio" | "video" };
export type PlaybackCheckpoint = Omit<PlaybackBinding, "namespace"> & { position: number; duration: number; heard: boolean; updatedAt: number };
export type PlaybackCheckpointHolder = { schema: "1"; entries: PlaybackCheckpoint[] };
const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/;
const counter = /^[1-9][0-9]{0,18}$/;
export const playbackScope = (namespace: string) => namespace.split("|").slice(0, 4).join("|");
export function validPlaybackNamespace(namespace: string): boolean {
  const p = namespace.split("|");
  if (p.length !== 6 || p[3] !== "personal" || !uuid.test(p[1]) || !uuid.test(p[2]) || !uuid.test(p[5]) || !counter.test(p[4]) || BigInt(p[4]) > 9223372036854775807n) return false;
  try { const u = new URL(p[0]); return u.origin === p[0] && ["http:", "https:"].includes(u.protocol); } catch { return false; }
}
export function validPlaybackBinding(value: PlaybackBinding): boolean {
  return validPlaybackNamespace(value.namespace) && uuid.test(value.messageId) && uuid.test(value.conversationId)
    && counter.test(value.contentVersion) && BigInt(value.contentVersion) <= 9223372036854775807n
    && /^[0-9a-f]{64}$/.test(value.mediaDigest) && ["audio", "video"].includes(value.kind);
}
export function checkpointMatches(value: PlaybackCheckpoint, binding: PlaybackBinding): boolean {
  return validPlaybackBinding(binding) && value.messageId === binding.messageId && value.conversationId === binding.conversationId
    && value.contentVersion === binding.contentVersion && value.mediaDigest === binding.mediaDigest && value.kind === binding.kind;
}
export function pruneCheckpointEntries(value: unknown, globals: ReadonlySet<string>): PlaybackCheckpointHolder {
  if (value === undefined) return { schema: "1", entries: [] };
  const holder = value as PlaybackCheckpointHolder;
  if (holder?.schema !== "1" || !Array.isArray(holder.entries) || holder.entries.length > playbackCheckpointLimit) throw Error("PLAYBACK_CHECKPOINT_INVALID");
  const ids = new Set<string>();
  const entries = holder.entries.filter(entry => {
    if (!entry || !uuid.test(entry.messageId) || !uuid.test(entry.conversationId) || ids.has(entry.messageId)
      || !counter.test(entry.contentVersion) || BigInt(entry.contentVersion) > 9223372036854775807n || !/^[0-9a-f]{64}$/.test(entry.mediaDigest)
      || !["audio", "video"].includes(entry.kind) || !Number.isFinite(entry.position) || !Number.isFinite(entry.duration)
      || entry.duration <= 0 || entry.duration > 86400 || entry.position < 0 || entry.position > entry.duration
      || typeof entry.heard !== "boolean" || !Number.isSafeInteger(entry.updatedAt) || entry.updatedAt < 0) throw Error("PLAYBACK_CHECKPOINT_INVALID");
    ids.add(entry.messageId); return !globals.has(entry.messageId);
  }).map(entry => ({ messageId: entry.messageId, conversationId: entry.conversationId, contentVersion: entry.contentVersion, mediaDigest: entry.mediaDigest, kind: entry.kind, position: entry.position, duration: entry.duration, heard: entry.heard, updatedAt: entry.updatedAt }));
  return { schema: "1", entries };
}
export function nextCheckpoint(binding: PlaybackBinding, position: number, duration: number, heard: boolean, now: number): PlaybackCheckpoint {
  if (!validPlaybackBinding(binding) || !Number.isFinite(position) || !Number.isFinite(duration) || duration <= 0 || duration > 86400 || position < 0 || !Number.isSafeInteger(now) || now < 0) throw Error("PLAYBACK_CHECKPOINT_INVALID");
  const { namespace: _namespace, ...identity } = binding;
  return { ...identity, position: position >= duration - 0.25 ? 0 : Math.min(position, duration), duration, heard, updatedAt: now };
}
export function upsertCheckpoint(holder: PlaybackCheckpointHolder, checkpoint: PlaybackCheckpoint): PlaybackCheckpointHolder {
  return { schema: "1", entries: [...holder.entries.filter(entry => entry.messageId !== checkpoint.messageId), checkpoint].sort((a, b) => b.updatedAt - a.updatedAt).slice(0, playbackCheckpointLimit) };
}
export function transitionCheckpoint(holder: PlaybackCheckpointHolder, binding: PlaybackBinding, action: "read" | "save" | "forget", next?: PlaybackCheckpoint): PlaybackCheckpointHolder {
  const previous = holder.entries.find(entry => entry.messageId === binding.messageId);
  if (action === "read") return holder;
  if (action === "forget") return { schema: "1", entries: holder.entries.filter(entry => !checkpointMatches(entry, binding)) };
  if (!next || !checkpointMatches(next, binding)) throw Error("PLAYBACK_CHECKPOINT_INVALID");
  if (previous && (BigInt(previous.contentVersion) > BigInt(binding.contentVersion) || previous.contentVersion === binding.contentVersion && !checkpointMatches(previous, binding))) throw Error("PLAYBACK_VERSION_CHANGED");
  if (previous && checkpointMatches(previous, binding)) {
    if (previous.updatedAt > next.updatedAt) return holder;
    next = { ...next, heard: previous.heard || next.heard };
  }
  return upsertCheckpoint(holder, next);
}
