// Created by Jobs on 2026年11月18日，星期三.
import type { UserReport } from "./ReportPanel";

export type ReportEvidence = { id: string; name: string; contentType: string; size: number; sha256: string; url: string; available: boolean };
export type ReportMessage = { id: string; conversationId: string; senderId: string; type: string; text: string; contentVersion: string; createdAt: string; attachmentURL?: string; available: boolean };
export type AdminReportDetail = { report: UserReport & { targetUserId?: string }; evidence: ReportEvidence[]; selectedMessages: ReportMessage[]; includesWholeConversation: false };
export const reportUUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
export function reportMediaLimit(contentType: string): number {
  const kind = contentType.split(";", 1)[0].toLowerCase();
  if (kind === "image" || /^image\/(jpeg|png|webp|gif)$/.test(kind)) return 20 * 1024 * 1024;
  if (kind === "video" || /^video\/(mp4|quicktime|webm)$/.test(kind)) return 64 * 1024 * 1024;
  return 0;
}

export function reportListPath(reporterId: string, targetUserId: string): string {
  const query = new URLSearchParams();
  for (const [key, raw] of [["reporterId", reporterId], ["targetUserId", targetUserId]]) {
    const value = raw.trim();
    if (value && !reportUUID.test(value)) throw Error("REPORT_FILTER_INVALID");
    if (value) query.set(key, value);
  }
  const suffix = query.toString();
  return "/admin/reports" + (suffix ? "?" + suffix : "");
}

// Only the exact report-bound media routes may receive the current authentication context.
export function reportMediaPath(raw: string, reportId: string, objectId: string, kind: "evidence" | "message"): string {
  if (!reportUUID.test(reportId) || !reportUUID.test(objectId) || !raw.startsWith("/api/")) throw Error("REPORT_MEDIA_INVALID");
  const url = new URL(raw, "https://jobsim.invalid");
  if (url.origin !== "https://jobsim.invalid" || raw.split("?", 1)[0] !== url.pathname || url.hash || url.username || url.password) throw Error("REPORT_MEDIA_INVALID");
  if (kind === "evidence") {
    if (url.pathname !== "/api/reports/evidence/" + objectId || [...url.searchParams].length !== 1 || url.searchParams.get("reportId") !== reportId) throw Error("REPORT_MEDIA_INVALID");
  } else if (url.pathname !== "/api/admin/reports/" + reportId + "/messages/" + objectId + "/attachment" || url.search) throw Error("REPORT_MEDIA_INVALID");
  return url.pathname.slice(4) + url.search;
}

export function checkedReportDetail(value: AdminReportDetail, reportId: string): AdminReportDetail {
  const report = value?.report;
  if (!report || report.id !== reportId || !reportUUID.test(report.reporterId) || !reportUUID.test(report.targetId)
      || !["user", "message", "conversation"].includes(report.targetType) || typeof report.reason !== "string"
      || typeof report.state !== "string" || typeof report.version !== "string" || !/^(0|[1-9][0-9]*)$/.test(report.version)
      || value.includesWholeConversation !== false || !Array.isArray(value.evidence) || !Array.isArray(value.selectedMessages)) throw Error("REPORT_DETAIL_INVALID");
  const evidenceIds = new Set<string>(), messageIds = new Set<string>();
  const evidence = value.evidence.map(item => {
    if (!reportUUID.test(item.id) || evidenceIds.has(item.id) || typeof item.available !== "boolean") throw Error("REPORT_DETAIL_INVALID");
    evidenceIds.add(item.id);
    if (!item.available) return { id: item.id, name: typeof item.name === "string" ? item.name : "", contentType: typeof item.contentType === "string" ? item.contentType : "", size: Number.isSafeInteger(item.size) && item.size >= 0 ? item.size : 0, sha256: typeof item.sha256 === "string" ? item.sha256 : "", url: "", available: false };
    if (!Number.isSafeInteger(item.size) || item.size < 0 || !/^[a-f0-9]{64}$/i.test(item.sha256) || typeof item.name !== "string" || typeof item.contentType !== "string") throw Error("REPORT_DETAIL_INVALID");
    reportMediaPath(item.url, reportId, item.id, "evidence");
    return item;
  });
  const selectedMessages = value.selectedMessages.map(item => {
    if (!reportUUID.test(item.id) || messageIds.has(item.id) || typeof item.available !== "boolean") throw Error("REPORT_DETAIL_INVALID");
    messageIds.add(item.id);
    if (!item.available) return { id: item.id, conversationId: "", senderId: "", type: "", text: "", contentVersion: "0", createdAt: "", available: false };
    if (!reportUUID.test(item.conversationId) || !reportUUID.test(item.senderId) || typeof item.text !== "string" || typeof item.type !== "string" || typeof item.createdAt !== "string" || typeof item.contentVersion !== "string" || !/^(0|[1-9][0-9]*)$/.test(item.contentVersion)) throw Error("REPORT_DETAIL_INVALID");
    if (item.attachmentURL) reportMediaPath(item.attachmentURL, reportId, item.id, "message");
    return item;
  });
  return { report, evidence, selectedMessages, includesWholeConversation: false };
}

export function reportPreviewMime(bytes: Uint8Array, declared: string): string {
  const prefix = (values: number[]) => values.every((v, i) => bytes[i] === v);
  let type = "";
  if (prefix([0x89,0x50,0x4e,0x47,0x0d,0x0a,0x1a,0x0a])) type = "image/png";
  else if (prefix([0xff,0xd8,0xff])) type = "image/jpeg";
  else if (["GIF87a", "GIF89a"].includes(String.fromCharCode(...bytes.slice(0,6)))) type = "image/gif";
  else if (String.fromCharCode(...bytes.slice(0,4)) === "RIFF" && String.fromCharCode(...bytes.slice(8,12)) === "WEBP") type = "image/webp";
  else if (prefix([0x1a,0x45,0xdf,0xa3])) type = "video/webm";
  else if (bytes.length >= 12 && String.fromCharCode(...bytes.slice(4,8)) === "ftyp") type = String.fromCharCode(...bytes.slice(8,12)) === "qt  " ? "video/quicktime" : "video/mp4";
  const hint = declared.split(";", 1)[0].toLowerCase();
  if (!type || (hint.includes("/") ? hint !== type : !type.startsWith(hint + "/"))) throw Error("REPORT_MEDIA_TYPE");
  return type;
}
