// Created by Jobs on 2026年11月18日，星期三.
import { useEffect, useRef, useState } from "react";
import { apiBoundedBytes } from "./api";
import { authSnapshotCurrent, type AuthSnapshot } from "./AuthSession";
import { reportMediaPath, reportMediaLimit, reportPreviewMime, type AdminReportDetail } from "./AdminReportProtocol";
import "./AdminReport.css";

type Translation = (key: string) => string;
function EvidenceMedia({ path, reportId, objectId, kind, name, contentType, size, sha256, owner, active, onActivate, t, onFailure }: {
  path: string; reportId: string; objectId: string; kind: "evidence" | "message"; name: string; contentType: string; size?: number; sha256?: string; owner: AuthSnapshot; active: boolean; onActivate: (release: () => void) => void; t: Translation; onFailure: () => void;
}) {
  const [url, setURL] = useState(""), [busy, setBusy] = useState(false), [failed, setFailed] = useState(false);
  const request = useRef<AbortController | null>(null), ownedURL = useRef(""), video = useRef<HTMLVideoElement | null>(null);
  const retire = () => {
    request.current?.abort(); request.current = null;
    if (video.current) { video.current.pause(); video.current.removeAttribute("src"); video.current.load(); }
    if (ownedURL.current) URL.revokeObjectURL(ownedURL.current);
    ownedURL.current = ""; setURL(""); setBusy(false);
  };
  useEffect(() => {
    const changed = () => { if (!authSnapshotCurrent(owner)) retire(); };
    const hidden = () => { if (document.hidden) retire(); else changed(); };
    window.addEventListener("jobsim-auth-required", changed);
    window.addEventListener("storage", changed);
    document.addEventListener("visibilitychange", hidden);
    return () => {
      request.current?.abort(); request.current = null;
      if (ownedURL.current) URL.revokeObjectURL(ownedURL.current);
      ownedURL.current = "";
      window.removeEventListener("jobsim-auth-required", changed);
      window.removeEventListener("storage", changed);
      document.removeEventListener("visibilitychange", hidden);
    };
  }, [path, owner.revision, owner.auth?.familyId]);
  useEffect(() => { if (!active) retire(); }, [active]);
  useEffect(() => {
    const player = video.current;
    return () => { if (player) { player.pause(); player.removeAttribute("src"); player.load(); } };
  }, [url]);
  const open = async () => {
    onActivate(retire); retire(); setFailed(false);
    const limit = reportMediaLimit(contentType);
    if (!limit) { setFailed(true); return; }
    if (!authSnapshotCurrent(owner)) { onFailure(); return; }
    const controller = new AbortController(); request.current = controller; setBusy(true);
    try {
      const bytes = await apiBoundedBytes(reportMediaPath(path, reportId, objectId, kind), limit,
        { signal: controller.signal, cache: "no-store", redirect: "error" });
      if (controller.signal.aborted || request.current !== controller || !authSnapshotCurrent(owner)) return;
      if (size !== undefined && bytes.byteLength !== size) throw Error("REPORT_MEDIA_SIZE");
      if (sha256) {
        const digest = [...new Uint8Array(await crypto.subtle.digest("SHA-256", bytes))].map(v => v.toString(16).padStart(2, "0")).join("");
        if (digest.toLowerCase() !== sha256.toLowerCase()) throw Error("REPORT_MEDIA_DIGEST");
      }
      if (controller.signal.aborted || request.current !== controller || !authSnapshotCurrent(owner)) return;
      const type = reportPreviewMime(bytes, contentType);
      ownedURL.current = URL.createObjectURL(new Blob([bytes], { type })); setURL(ownedURL.current);
    } catch {
      if (!controller.signal.aborted && request.current === controller) { retire(); setFailed(true); onFailure(); }
    } finally {
      if (request.current === controller) setBusy(false);
    }
  };
  const limit = reportMediaLimit(contentType);
  const exceeded = size !== undefined && size > limit;
  return <div className="admin-evidence-media">
    {active && url && authSnapshotCurrent(owner) && (contentType.startsWith("image")
      ? <img src={url} alt={name} onError={() => { retire(); setFailed(true); }} />
      : <video ref={video} src={url} controls preload="metadata" aria-label={name} controlsList="nodownload" onError={() => { retire(); setFailed(true); }} />)}
    {failed && <p role="status">{t("contentUnavailable")}</p>}
    {!limit ? <p>{t("notSupported")}</p> : exceeded ? <p>{t("reportPreviewLimit")}</p> : <button type="button" className="secondary" disabled={busy} onClick={() => void open()}>{t(busy ? "working" : "viewEvidence")}</button>}
    {active && url && <button type="button" className="secondary" onClick={retire}>{t("close")}</button>}
  </div>;
}

export default function AdminReportDetails({ detail, owner, t, canWrite, onResolve, onClose, onInvalid }: {
  detail: AdminReportDetail; owner: AuthSnapshot; t: Translation; canWrite: boolean; onResolve: () => void; onClose: () => void; onInvalid: () => void;
}) {
  const [activePreview, setActivePreview] = useState<string | null>(null);
  const previewRelease = useRef<(() => void) | null>(null);
  const activatePreview = (key: string, release: () => void) => {
    previewRelease.current?.();
    previewRelease.current = release;
    setActivePreview(key);
  };
  if (!authSnapshotCurrent(owner)) return null;
  return <section className="admin-report-detail" aria-label={t("reportDetail")}>
    <header><h3>{t("reportDetail")}</h3><button className="secondary" type="button" onClick={onClose}>{t("close")}</button></header>
    <dl><dt>{t("reporterId")}</dt><dd>{detail.report.reporterId}</dd><dt>{t("targetUserId")}</dt><dd>{detail.report.targetUserId || "—"}</dd></dl>
    <p>{detail.report.reason}</p><p className="admin-report-hint">{t("reportScopeNote")}</p>
    <h4>{t("reportEvidence")}</h4>
    {!detail.evidence.length && <p>{t("noReportEvidence")}</p>}
    {detail.evidence.map(item => <article className="admin-evidence-card" key={item.id}>
      <strong>{item.name}</strong><small>{item.contentType} · {item.size.toLocaleString()} B</small>
      <small>SHA256 {item.sha256}</small>
      {item.available ? <EvidenceMedia path={item.url} reportId={detail.report.id} objectId={item.id} kind="evidence" name={item.name} contentType={item.contentType} size={item.size} sha256={item.sha256} owner={owner} active={activePreview === "evidence:" + item.id} onActivate={release => activatePreview("evidence:" + item.id, release)} t={t} onFailure={onInvalid} /> : <p>{t("contentUnavailable")}</p>}
    </article>)}
    <h4>{t("selectedReportMessages")}</h4>
    {!detail.selectedMessages.length && <p>{t("noReportEvidence")}</p>}
    {detail.selectedMessages.map(item => <article className="admin-evidence-card" key={item.id}>
      {item.available && <small>{item.senderId} · {item.createdAt}</small>}
      {item.available ? <><p>{item.text}</p>{item.attachmentURL && ["image", "video"].includes(item.type) && <EvidenceMedia path={item.attachmentURL} reportId={detail.report.id} objectId={item.id} kind="message" name={item.id} contentType={item.type} owner={owner} active={activePreview === "message:" + item.id} onActivate={release => activatePreview("message:" + item.id, release)} t={t} onFailure={onInvalid} />}</> : <p>{t("contentUnavailable")}</p>}
    </article>)}
    <button className="primary" type="button" disabled={!canWrite} onClick={onResolve}>{t("manage")}</button>
  </section>;
}
