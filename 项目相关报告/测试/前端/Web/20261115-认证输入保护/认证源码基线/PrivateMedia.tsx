// Created by Jobs on 2026年11月15日，星期日.
import { useEffect, useState } from "react";
import { Download, FileText, RefreshCw } from "lucide-react";
import { apiBytes } from "./api";

export default function PrivateMedia({
  path,
  type,
  name,
  version,
  t,
}: {
  path: string;
  type: string;
  name: string;
  version: string;
  t: (key: string) => string;
}) {
  const [url, setURL] = useState("");
  const [failed, setFailed] = useState(false);
  const [busy, setBusy] = useState(false);
  const [attempt, setAttempt] = useState(0);
  useEffect(() => {
    const controller = new AbortController();
    let active = true,
      objectURL = "";
    setURL("");
    setFailed(false);
    if (type === "file") return () => controller.abort();
    if (path.startsWith("blob:")) {
      setURL(path);
      return () => controller.abort();
    }
    setBusy(true);
    void apiBytes(path.substring(4), { signal: controller.signal })
      .then((blob) => {
        if (!active) return;
        objectURL = URL.createObjectURL(blob);
        setURL(objectURL);
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
    };
  }, [path, type, version, attempt]);
  const download = async () => {
    if (busy) return;
    setBusy(true);
    setFailed(false);
    let downloadURL = "";
    try {
      downloadURL = path.startsWith("blob:")
        ? path
        : URL.createObjectURL(await apiBytes(path.substring(4)));
      const anchor = document.createElement("a");
      anchor.href = downloadURL;
      anchor.download = name;
      anchor.click();
    } catch {
      setFailed(true);
    } finally {
      if (downloadURL && downloadURL !== path)
        setTimeout(() => URL.revokeObjectURL(downloadURL), 1000);
      setBusy(false);
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
    ) : type === "audio" ? (
      <audio
        controls
        preload="metadata"
        src={url}
        aria-label={name}
        onError={() => setFailed(true)}
      />
    ) : (
      <video
        controls
        preload="metadata"
        src={url}
        aria-label={name}
        onError={() => setFailed(true)}
      />
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
