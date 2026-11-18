// Created by Jobs on 2026年11月17日，星期二.
export const recordingMinimumMS = 1000;
export const recordingMaximumMS = 300000;
export const recordingMaximumBytes = 16 * 1024 * 1024;
export const recordingMIMEs = ["audio/webm;codecs=opus", "audio/webm", "audio/mp4;codecs=mp4a.40.2", "audio/mp4"] as const;
export type RecordingFailure = "unsupported" | "denied" | "noDevice" | "deviceLost" | "interrupted" | "tooShort" | "tooLarge" | "failed";
export type RecordingPreview = { blob: Blob; durationMS: number; createdAt: number };
export type RecordingState = { phase: "idle" | "requesting" | "recording" | "stopping" | "preview" | "error"; elapsedMS: number; failure?: RecordingFailure; preview?: RecordingPreview };
export type RecordingEnvironment = {
  open: () => Promise<MediaStream>;
  recorder: (stream: MediaStream, mime: string) => MediaRecorder;
  supported: (mime: string) => boolean;
  isCurrent: () => boolean;
  now: () => number;
  changed: (state: RecordingState) => void;
  every: (callback: () => void) => () => void;
};

// The permission promise cannot be cancelled. Its generation owns every late stream.
export class AudioRecorderController {
  state: RecordingState = { phase: "idle", elapsedMS: 0 };
  private environment: RecordingEnvironment;
  private generation = 0;
  private startedAt = 0;
  private stream?: MediaStream;
  private recorder?: MediaRecorder;
  private chunks: Blob[] = [];
  private bytes = 0;
  private cancelTimer?: () => void;
  private removeTrackListeners: (() => void)[] = [];

  constructor(environment: RecordingEnvironment) { this.environment = environment; }
  private update(state: RecordingState) { this.state = state; this.environment.changed(state); }
  private release() {
    this.cancelTimer?.(); this.cancelTimer = undefined;
    for (const remove of this.removeTrackListeners) remove();
    this.removeTrackListeners = [];
    for (const track of this.stream?.getTracks() ?? []) track.stop();
    this.stream = undefined;
  }
  cancel(failure?: RecordingFailure) {
    this.generation++;
    const recorder = this.recorder;
    this.recorder = undefined;
    if (recorder) {
      recorder.ondataavailable = null; recorder.onstop = null; recorder.onerror = null;
      try { if (recorder.state !== "inactive") recorder.stop(); } catch { /* Tracks still close below. */ }
    }
    this.release(); this.chunks = []; this.bytes = 0;
    this.update({ phase: failure ? "error" : "idle", elapsedMS: 0, failure });
  }
  async start() {
    if (["requesting", "recording", "stopping"].includes(this.state.phase)) return;
    this.cancel();
    const own = this.generation;
    const mime = recordingMIMEs.find(value => this.environment.supported(value));
    if (!mime || !this.environment.isCurrent()) { this.cancel("unsupported"); return; }
    this.update({ phase: "requesting", elapsedMS: 0 });
    let stream: MediaStream | undefined;
    try {
      stream = await this.environment.open();
      if (own !== this.generation || !this.environment.isCurrent()) {
        for (const track of stream.getTracks()) track.stop();
        return;
      }
      this.stream = stream;
      if (!stream.getAudioTracks().some(track => track.readyState === "live")) throw Object.assign(Error("No live audio input"), { name: "NotFoundError" });
      const recorder = this.environment.recorder(stream, mime);
      this.recorder = recorder; this.startedAt = this.environment.now();
      for (const track of stream.getTracks()) {
        const lost = () => { if (own === this.generation && this.state.phase === "recording") this.cancel("deviceLost"); };
        track.addEventListener("ended", lost); track.addEventListener("mute", lost);
        this.removeTrackListeners.push(() => { track.removeEventListener("ended", lost); track.removeEventListener("mute", lost); });
      }
      recorder.ondataavailable = event => {
        if (own !== this.generation || !event.data.size) return;
        this.bytes += event.data.size;
        if (this.bytes > recordingMaximumBytes) { this.cancel("tooLarge"); return; }
        this.chunks.push(event.data);
      };
      recorder.onerror = () => { if (own === this.generation) this.cancel("deviceLost"); };
      recorder.onstop = () => {
        if (own !== this.generation) return;
        const durationMS = Math.min(recordingMaximumMS, Math.max(0, this.environment.now() - this.startedAt));
        const intended = this.state.phase === "stopping";
        this.release(); this.recorder = undefined;
        if (!intended || !this.environment.isCurrent()) { this.cancel("interrupted"); return; }
        if (durationMS < recordingMinimumMS || !this.bytes) { this.cancel("tooShort"); return; }
        const actualType = (recorder.mimeType || mime).split(";")[0].toLowerCase();
        if (!["audio/webm", "audio/mp4"].includes(actualType)) { this.cancel("unsupported"); return; }
        const preview = { blob: new Blob(this.chunks, { type: actualType }), durationMS, createdAt: this.environment.now() };
        this.chunks = []; this.bytes = 0;
        this.update({ phase: "preview", elapsedMS: durationMS, preview });
      };
      recorder.start(1000);
      this.update({ phase: "recording", elapsedMS: 0 });
      this.cancelTimer = this.environment.every(() => {
        if (own !== this.generation) return;
        if (!this.environment.isCurrent()) { this.cancel("interrupted"); return; }
        const elapsedMS = this.environment.now() - this.startedAt;
        if (elapsedMS >= recordingMaximumMS) this.stop();
        else this.update({ phase: "recording", elapsedMS });
      });
    } catch (error) {
      if (own !== this.generation) { for (const track of stream?.getTracks() ?? []) track.stop(); return; }
      const name = (error as { name?: string })?.name;
      this.cancel(name === "NotAllowedError" || name === "SecurityError" ? "denied" : name === "NotFoundError" ? "noDevice" : name === "NotReadableError" ? "deviceLost" : "failed");
    }
  }
  stop() {
    if (this.state.phase !== "recording" || !this.recorder) return;
    this.update({ phase: "stopping", elapsedMS: this.environment.now() - this.startedAt });
    try { this.recorder.stop(); this.release(); }
    catch { this.cancel("failed"); }
  }
}
