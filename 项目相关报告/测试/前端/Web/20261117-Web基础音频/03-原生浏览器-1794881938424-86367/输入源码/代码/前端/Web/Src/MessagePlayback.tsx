// Created by Jobs on 2026年11月17日，星期二.
import { useEffect, useRef, useState } from "react";
import { api, ApiError } from "./api";
import { authSnapshot, authSnapshotCurrent } from "./AuthSession";
import { attachmentPath } from "./forward";
import { playbackScope, validPlaybackBinding, type PlaybackBinding } from "./PlaybackCheckpointProtocol";
import { forgetPlaybackCheckpoint, loadPlaybackCheckpoint, savePlaybackCheckpoint } from "./PlaybackCheckpointStore";
import { refreshMessageFacts, subscribeGlobalMessages } from "./MessageFacts";
import { audioLabel } from "./AudioMessageLabels";
import type { Message } from "./types";
import "./AudioMedia.css";

export type MessagePlaybackIdentity = { namespace?: string; messageId?: string; conversationId?: string; contentVersion?: string; familyId?: string; forwarded?: boolean; locale?: string };
const clock = (seconds: number) => Math.floor(seconds / 60) + ":" + String(Math.floor(seconds % 60)).padStart(2, "0");
export default function MessagePlayback({ url, path, kind, name, version, digest, identity, onUnavailable }: {
  url: string; path: string; kind: "audio" | "video"; name: string; version: string; digest: string; identity: MessagePlaybackIdentity; onUnavailable: () => void;
}) {
  const player = useRef<HTMLMediaElement | null>(null), owner = useRef<AbortController | undefined>(undefined), allowed = useRef(false), valid = useRef(false), checking = useRef(false), heardRef = useRef(false), checkpointBusy = useRef(false), lastSaved = useRef(0);
  const [heard, setHeard] = useState(false), [resume, setResume] = useState(0), [speed, setSpeed] = useState(1), [warning, setWarning] = useState("");
  const live = useRef({ identity, digest, path, contentVersion: identity.contentVersion ?? version, kind }); live.current = { identity, digest, path, contentVersion: identity.contentVersion ?? version, kind };
  const label = (key: string) => audioLabel(identity.locale ?? "en", key);
  const binding: PlaybackBinding = { namespace: identity.namespace ?? "", messageId: identity.messageId ?? "", conversationId: identity.conversationId ?? "", contentVersion: identity.contentVersion ?? version, mediaDigest: digest, kind };
  const persistent = !identity.forwarded && !path.startsWith("blob:") && validPlaybackBinding(binding);
  const context = useRef<{ binding: PlaybackBinding; persistent: boolean; isCurrent: () => boolean }>({ binding, persistent, isCurrent: () => false });
  const persist = async (finished = false) => {
    const source = context.current, element = player.current;
    if (!source.persistent || !valid.current || !source.isCurrent() || !element || !Number.isFinite(element.duration) || element.duration <= 0 || checkpointBusy.current) return;
    checkpointBusy.current = true;
    try {
      const saved = await savePlaybackCheckpoint(source.binding, finished ? element.duration : element.currentTime, element.duration, heardRef.current, source.isCurrent);
      if (source.isCurrent() && !saved) unavailable();
    }
    catch { if (source.isCurrent()) setWarning("progressFailed"); }
    finally { checkpointBusy.current = false; }
  };
  const unavailable = () => {
    valid.current = false; allowed.current = false; player.current?.pause();
    const source = context.current;
    if (source.persistent) void forgetPlaybackCheckpoint(source.binding, source.isCurrent).catch(() => {});
    onUnavailable();
  };
  useEffect(() => {
    const controller = new AbortController(), snapshot = authSnapshot(), original = { namespace: identity.namespace, familyId: identity.familyId, messageId: identity.messageId, contentVersion: binding.contentVersion, kind, digest, path };
    const isCurrent = () => !controller.signal.aborted && live.current.identity.namespace === original.namespace && live.current.identity.familyId === original.familyId && live.current.identity.messageId === original.messageId && live.current.contentVersion === original.contentVersion && live.current.kind === original.kind && live.current.digest === original.digest && live.current.path === original.path && (!original.familyId || authSnapshotCurrent(snapshot));
    owner.current = controller; context.current = { binding, persistent, isCurrent }; valid.current = true; allowed.current = false; checking.current = false; heardRef.current = false;
    setHeard(false); setResume(0); setWarning("");
    const unsubscribe = subscribeGlobalMessages(notice => {
      if (isCurrent() && persistent && notice.scope === playbackScope(binding.namespace) && notice.ids.includes(binding.messageId)) unavailable();
    });
    const interrupt = () => {
      if (player.current && !player.current.paused) { player.current.pause(); if (isCurrent()) setWarning("playbackInterrupted"); }
      allowed.current = false; void persist();
    };
    const visibility = () => { if (document.visibilityState !== "visible") interrupt(); };
    const authChanged = () => { if (!isCurrent()) unavailable(); };
    document.addEventListener("visibilitychange", visibility); window.addEventListener("pagehide", interrupt); window.addEventListener("jobsim-auth-required", authChanged); navigator.mediaDevices?.addEventListener("devicechange", interrupt);
    return () => {
      // Only a still-owned namespace can save; auth changes cannot write as the new user.
      void persist(); valid.current = false; allowed.current = false; player.current?.pause(); controller.abort(); unsubscribe();
      document.removeEventListener("visibilitychange", visibility); window.removeEventListener("pagehide", interrupt); window.removeEventListener("jobsim-auth-required", authChanged); navigator.mediaDevices?.removeEventListener("devicechange", interrupt);
    };
  }, [url, path, kind, version, digest, identity.namespace, identity.familyId, identity.messageId, identity.contentVersion, identity.forwarded]);
  const metadata = async () => {
    const source = context.current, element = player.current;
    if (element) element.playbackRate = speed;
    if (!source.persistent || !element) return;
    if (!Number.isFinite(element.duration) || element.duration <= 0) { setWarning("progressFailed"); return; }
    try {
      const row = await loadPlaybackCheckpoint(source.binding, source.isCurrent);
      if (!source.isCurrent() || !valid.current || !row || player.current !== element) return;
      heardRef.current = row.heard; setHeard(row.heard);
      if (Number.isFinite(element.duration) && row.position > 0 && row.position < element.duration - 0.25) { element.currentTime = row.position; setResume(row.position); }
    } catch { if (source.isCurrent()) setWarning("progressFailed"); }
  };
  const authorize = async () => {
    const source = context.current, element = player.current, controller = owner.current;
    if (!element || !controller || checking.current || !valid.current || !source.isCurrent() || document.visibilityState !== "visible") return;
    checking.current = true; setWarning("");
    try {
      if (!path.startsWith("blob:")) {
        if (!identity.messageId || !identity.conversationId) throw Error("MEDIA_IDENTITY_REQUIRED");
        const message = await api<Message>("/messages/" + identity.messageId, { signal: controller.signal });
        if (!source.isCurrent()) return;
        if (message.id !== identity.messageId || message.conversationId !== identity.conversationId || message.deleted || message.contentUnavailable || message.forwardUnavailable || (message.contentVersion ?? message.version) !== source.binding.contentVersion || attachmentPath(message) !== path || message.type !== kind) { unavailable(); return; }
        if (source.persistent && (await refreshMessageFacts(source.binding.namespace, source.isCurrent)).has(source.binding.messageId)) { unavailable(); return; }
      }
      if (!source.isCurrent() || !valid.current || document.visibilityState !== "visible") return;
      allowed.current = true;
      await element.play();
      if (!source.isCurrent() || !valid.current) element.pause();
    } catch (error) {
      allowed.current = false; element.pause();
      if (source.isCurrent()) {
        if (error instanceof ApiError && [403, 404, 410].includes(error.status)) unavailable();
        else setWarning("playbackCheckFailed");
      }
    } finally { if (source === context.current) checking.current = false; }
  };
  const onPlay = () => {
    if (!valid.current || !context.current.isCurrent() || !allowed.current) { player.current?.pause(); void authorize(); }
  };
  const onPlaying = () => {
    if (!allowed.current || !valid.current || !context.current.isCurrent()) { player.current?.pause(); return; }
    heardRef.current = true; setHeard(true); setResume(0); void persist();
  };
  const props = {
    controls: true, preload: "metadata", src: url, "aria-label": name,
    onLoadedMetadata: () => void metadata(), onPlay, onPlaying,
    onPause: () => { if (player.current?.paused) { allowed.current = false; void persist(); } },
    onEnded: () => { allowed.current = false; setResume(0); void persist(true); },
    onTimeUpdate: () => {
      if (!context.current.isCurrent()) { unavailable(); return; }
      if (Date.now() - lastSaved.current >= 2000) { lastSaved.current = Date.now(); void persist(); }
    },
    onError: unavailable,
  };
  return <div className="message-playback">
    {kind === "audio" ? <audio ref={element => { player.current = element; }} {...props} /> : <video ref={element => { player.current = element; }} {...props} />}
    <div className="message-playback-tools">
      <span>{label(heard ? "heard" : "unheard")}</span>
      <label>{label("speed")} <select value={speed} onChange={event => { const value = Number(event.target.value); setSpeed(value); if (player.current) player.current.playbackRate = value; }}>{[0.75, 1, 1.25, 1.5, 2].map(value => <option value={value} key={value}>{value}×</option>)}</select></label>
      {resume > 0 && <button type="button" className="secondary" onClick={() => void authorize()}>{label("resume")} · {clock(resume)}</button>}
    </div>
    {warning && <div className="message-playback-warning" role="status"><span>{label(warning)}</span><button type="button" className="secondary" onClick={() => { if (warning === "progressFailed") { setWarning(""); void persist(); } else void authorize(); }}>{label("resume")}</button></div>}
  </div>;
}
