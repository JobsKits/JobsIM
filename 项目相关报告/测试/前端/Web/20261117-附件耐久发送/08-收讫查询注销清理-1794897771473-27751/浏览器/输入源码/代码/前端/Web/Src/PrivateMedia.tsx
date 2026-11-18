// Created by Jobs on 2026年11月15日，星期日.
import { useEffect, useRef, useState } from "react";
import { Download, FileText, RefreshCw } from "lucide-react";
import { apiBytes } from "./api";
import { authSnapshot, authSnapshotCurrent } from "./AuthSession";
import MessagePlayback, { type MessagePlaybackIdentity } from "./MessagePlayback";

export default function PrivateMedia({
  path,
  type,
  name,
  version,
  t,
  playback,
}: {
  path: string;
  type: string;
  name: string;
  version: string;
  t: (key: string) => string;
  playback?: MessagePlaybackIdentity;
}) {
  const [url, setURL] = useState("");
  const [digest, setDigest] = useState("");
  const [failed, setFailed] = useState(false);
  const [busy, setBusy] = useState(false);
  const [attempt, setAttempt] = useState(0);
  const downloadContext = useRef<AbortController | null>(null);
  const ownedURL = useRef("");
  useEffect(() => {
    const controller = new AbortController();
    downloadContext.current = controller;
    setBusy(false);
    return () => { controller.abort(); };
  }, [path, type, version, playback?.namespace, playback?.familyId]);
  useEffect(() => {
    const controller = new AbortController(), snapshot = authSnapshot();
    let active = true,
      objectURL = "";
    setURL("");
    setDigest("");
    setFailed(false);
    if (type === "file") return () => controller.abort();
    if (path.startsWith("blob:")) {
      setURL(path);
      return () => controller.abort();
    }
    setBusy(true);
    void apiBytes(path.substring(4), { signal: controller.signal })
      .then((blob) => {
        return (async () => {
          const mediaDigest = type === "audio" || type === "video"
            ? [...new Uint8Array(await crypto.subtle.digest("SHA-256", await blob.arrayBuffer()))].map(value => value.toString(16).padStart(2, "0")).join("") : "";
          if (!active || playback?.familyId && !authSnapshotCurrent(snapshot)) return;
          objectURL = URL.createObjectURL(blob); ownedURL.current = objectURL;
          setDigest(mediaDigest); setURL(objectURL);
        })();
      })
      .catch(() => {
        if (active) setFailed(true);
      })
      .finally(() => {
        if (active) setBusy(false);
      });
    return () => {
      active = false;
      controller.abort();
      if (objectURL) URL.revokeObjectURL(objectURL);
      if (ownedURL.current === objectURL) ownedURL.current = "";
    };
  }, [path, type, version, attempt, playback?.namespace, playback?.familyId, playback?.contentVersion]);
  const download = async () => {
    if (busy) return;
    const controller = downloadContext.current;
    if (!controller || controller.signal.aborted) return;
    const isCurrent = () => !controller.signal.aborted && downloadContext.current === controller;
    setBusy(true);
    setFailed(false);
    let downloadURL = "";
    try {
      const blob = path.startsWith("blob:") ? undefined : await apiBytes(path.substring(4), { signal: controller.signal });
      if (!isCurrent()) return;
      downloadURL = blob ? URL.createObjectURL(blob) : path;
      const anchor = document.createElement("a");
      anchor.href = downloadURL;
      anchor.download = name;
      anchor.click();
    } catch {
      if (isCurrent()) setFailed(true);
    } finally {
      if (downloadURL && downloadURL !== path)
        setTimeout(() => URL.revokeObjectURL(downloadURL), 1000);
      if (isCurrent()) setBusy(false);
    }
  };
  if (type === "file")
    return (
      <button
        className="attachment-card"
        disabled={busy}
        onClick={() => void download()}
      >
        <FileText size={24} />
        <span>{failed ? t("contentUnavailable") : name}</span>
        <Download size={17} aria-label={t("download")} />
      </button>
    );
  if (url && !failed)
    return type === "image" ? (
      <img
        className="message-image"
        src={url}
        alt={name}
        onError={() => setFailed(true)}
      />
    ) : (
      <MessagePlayback url={url} path={path} kind={type === "audio" ? "audio" : "video"} name={name} version={version} digest={digest} identity={playback ?? {}}
        onUnavailable={() => { if (ownedURL.current) URL.revokeObjectURL(ownedURL.current); ownedURL.current = ""; setURL(""); setFailed(true); }} />
    );
  return (
    <div className="private-media-placeholder" role="status">
      <FileText size={24} />
      <span>{t(busy ? "working" : "contentUnavailable")}</span>
      {failed && (
        <button
          className="secondary"
          onClick={() => setAttempt((value) => value + 1)}
        >
          <RefreshCw size={15} />
          {t("reload")}
        </button>
      )}
    </div>
  );
}
