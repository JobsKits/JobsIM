// Created by Jobs on 2026年11月17日，星期二.
import { useEffect, useRef, useState } from "react";
import { Mic, Square, X, Send, RotateCcw } from "lucide-react";
import { AudioRecorderController, type RecordingState } from "./AudioRecorderController";
import { audioLabel } from "./AudioMessageLabels";
import { authSnapshot, authSnapshotCurrent } from "./AuthSession";
import "./AudioMedia.css";

export default function AudioRecorderPanel({ contextKey, disabled, locale, onSend }: {
  contextKey: string; disabled: boolean; locale: string; onSend: (file: File, isCurrent: () => boolean) => Promise<boolean>;
}) {
  const [state, setState] = useState<RecordingState>({ phase: "idle", elapsedMS: 0 }), [url, setURL] = useState(""), [sending, setSending] = useState(false), [sendFailed, setSendFailed] = useState(false);
  const controller = useRef<AudioRecorderController | undefined>(undefined), current = useRef({ contextKey, disabled }), mounted = useRef(false), previewElement = useRef<HTMLAudioElement>(null);
  const ownership = useRef<() => boolean>(() => false);
  current.current = { contextKey, disabled };
  const label = (key: string) => audioLabel(locale, key);
  const supported = typeof MediaRecorder !== "undefined" && !!navigator.mediaDevices?.getUserMedia && window.isSecureContext;
  useEffect(() => {
    mounted.current = true;
    const owner = contextKey, snapshot = authSnapshot();
    const belongs = () => mounted.current && current.current.contextKey === owner && (owner.startsWith("demo|") || authSnapshotCurrent(snapshot)) && document.visibilityState === "visible";
    ownership.current = belongs;
    const recorder = new AudioRecorderController({
      open: () => navigator.mediaDevices.getUserMedia({ audio: true, video: false }),
      recorder: (stream, mimeType) => new MediaRecorder(stream, { mimeType }),
      supported: mime => supported && MediaRecorder.isTypeSupported(mime),
      isCurrent: () => belongs() && !current.current.disabled,
      now: () => Date.now(), changed: value => { if (mounted.current && current.current.contextKey === owner) setState(value); },
      every: callback => { const timer = window.setInterval(callback, 200); return () => window.clearInterval(timer); },
    });
    controller.current = recorder; setState(recorder.state); setSending(false); setSendFailed(false);
    const interrupt = () => {
      previewElement.current?.pause();
      if (["requesting", "recording", "stopping"].includes(recorder.state.phase)) recorder.cancel("interrupted");
    };
    const visibility = () => { if (document.visibilityState !== "visible") interrupt(); };
    const authChanged = () => { if (!authSnapshotCurrent(snapshot)) { previewElement.current?.pause(); recorder.cancel("interrupted"); } };
    document.addEventListener("visibilitychange", visibility); window.addEventListener("pagehide", interrupt);
    window.addEventListener("jobsim-auth-required", authChanged);
    return () => { mounted.current = false; recorder.cancel(); previewElement.current?.pause(); document.removeEventListener("visibilitychange", visibility); window.removeEventListener("pagehide", interrupt); window.removeEventListener("jobsim-auth-required", authChanged); };
  }, [contextKey, supported]);
  useEffect(() => {
    if (disabled && ["requesting", "recording", "stopping"].includes(controller.current?.state.phase ?? "")) controller.current?.cancel("interrupted");
  }, [disabled]);
  useEffect(() => {
    if (!state.preview) { setURL(""); return; }
    const value = URL.createObjectURL(state.preview.blob); setURL(value);
    return () => URL.revokeObjectURL(value);
  }, [state.preview]);
  const cancel = () => { controller.current?.cancel(); previewElement.current?.pause(); setSendFailed(false); };
  const send = async () => {
    const source = controller.current, preview = source?.state.preview, owner = contextKey;
    if (!source || !preview || sending || disabled || !ownership.current()) return;
    previewElement.current?.pause(); setSending(true); setSendFailed(false);
    const belongs = ownership.current;
    const isCurrent = () => belongs() && mounted.current && current.current.contextKey === owner && source.state.preview === preview;
    try {
      const suffix = preview.blob.type === "audio/mp4" ? "m4a" : "webm";
      const queued = await onSend(new File([preview.blob], "voice-" + preview.createdAt + "." + suffix, { type: preview.blob.type }), isCurrent);
      if (!isCurrent()) return;
      if (queued) source.cancel(); else setSendFailed(true);
    } catch { if (isCurrent()) setSendFailed(true); }
    finally { if (mounted.current && current.current.contextKey === owner) setSending(false); }
  };
  const active = ["requesting", "recording", "stopping"].includes(state.phase);
  return <div className="audio-recorder" aria-label={label("record")}>
    {!active && !state.preview && <button className="secondary" type="button" disabled={disabled || !supported} onClick={() => { setSendFailed(false); void controller.current?.start(); }}><Mic size={17} />{label(state.phase === "error" ? "retry" : "record")}</button>}
    {!supported && <p className="self-state-hint">{label("unsupported")}</p>}
    {active && <div className="audio-recorder-actions">
      <span role="status" aria-live="polite">{label(state.phase === "requesting" ? "requesting" : "recording")} · {Math.floor(state.elapsedMS / 1000)}s</span>
      {state.phase === "recording" && <button className="secondary" type="button" onClick={() => controller.current?.stop()}><Square size={16} />{label("stop")}</button>}
      <button className="secondary" type="button" onClick={cancel}><X size={16} />{label("cancel")}</button>
    </div>}
    {state.preview && <div className="audio-recording-preview">
      <strong>{label("preview")}</strong>
      <audio ref={previewElement} controls preload="metadata" src={url || undefined} aria-label={label("preview")} />
      <div className="audio-recorder-actions">
        <button className="primary" type="button" disabled={disabled || sending} onClick={() => void send()}><Send size={16} />{label("send")}</button>
        <button className="secondary" type="button" disabled={sending || disabled} onClick={() => void controller.current?.start()}><RotateCcw size={16} />{label("retry")}</button>
        <button className="secondary" type="button" onClick={cancel}><X size={16} />{label("cancel")}</button>
      </div>
    </div>}
    {state.failure && <p role="alert">{label(state.failure)}</p>}
    {sendFailed && <p role="alert">{label("uploadFailed")} <button className="secondary" type="button" disabled={disabled || sending} onClick={() => void send()}>{label("send")}</button></p>}
    {(active || state.preview) && <p className="self-state-hint">{label("limits")}</p>}
  </div>;
}
