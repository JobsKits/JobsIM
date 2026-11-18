export class DataExportFileError extends Error {
  constructor(readonly code: "DATA_EXPORT_FILE_INVALID" | "DATA_EXPORT_FILE_LIMIT") { super(code); }
}
const invalid = (): never => { throw new DataExportFileError("DATA_EXPORT_FILE_INVALID"); };
const limited = (): never => { throw new DataExportFileError("DATA_EXPORT_FILE_LIMIT"); };
export const exportZIPLimit = 1024 * 1024 * 1024;
export const exportDirectoryLimit = 32 * 1024 * 1024;

export function exportFileName(header: string | null): string {
  if (!header || !/^attachment(?:\s*;|$)/i.test(header)) return invalid();
  const values = new Map<string, string>();
  const parameter = /;\s*(filename\*|filename)\s*=\s*("(?:[^"\\]|\\.)*"|[^;]*)/gi;
  for (const match of header.matchAll(parameter)) {
    const name = match[1].toLowerCase();
    if (values.has(name)) return invalid();
    let value = match[2].trim();
    if (value.startsWith('"')) value = value.slice(1, -1).replace(/\\(.)/g, "$1");
    values.set(name, value);
  }
  let name = values.get("filename*") ?? values.get("filename");
  if (values.has("filename*")) {
    if (!name || !/^UTF-8''/i.test(name)) return invalid();
    try { name = decodeURIComponent(name.slice(7)); } catch { return invalid(); }
  }
  if (!name || name.length > 180 || name !== name.trim() || /[\p{Cc}\p{Cf}/\\<>:"|?*]/u.test(name) || name === "." || name === ".." || !name.toLowerCase().endsWith(".zip")) return invalid();
  return name;
}

// A server may end an interrupted ZIP with an otherwise valid HTTP200. Verify
// its complete directory before handing a Blob to the browser download flow.
export async function validateExportZIP(blob: Blob): Promise<void> {
  if (blob.size > exportZIPLimit) return limited();
  if (blob.size < 22) return invalid();
  const tailStart = Math.max(0, blob.size - 65557), tail = new DataView(await blob.slice(tailStart).arrayBuffer());
  let eocd = -1;
  for (let i = tail.byteLength - 22; i >= 0; i--) {
    if (tail.getUint32(i, true) === 0x06054b50 && i + 22 + tail.getUint16(i + 20, true) === tail.byteLength) { eocd = i; break; }
  }
  if (eocd < 0 || tail.getUint16(eocd + 4, true) !== 0 || tail.getUint16(eocd + 6, true) !== 0) return invalid();
  let count = tail.getUint16(eocd + 10, true), size = tail.getUint32(eocd + 12, true), offset = tail.getUint32(eocd + 16, true);
  const eocdOffset = tailStart + eocd;
  let directoryEnd = eocdOffset;
  if (tail.getUint16(eocd + 8, true) !== count) return invalid();
  if (count === 65535 || size === 0xffffffff || offset === 0xffffffff) {
    if (eocd < 20 || tail.getUint32(eocd - 20, true) !== 0x07064b50 || tail.getUint32(eocd - 16, true) !== 0 || tail.getUint32(eocd - 4, true) !== 1) return invalid();
    const recordOffset = tail.getBigUint64(eocd - 12, true);
    if (recordOffset > BigInt(blob.size - 56)) return invalid();
    const record = new DataView(await blob.slice(Number(recordOffset), Number(recordOffset) + 56).arrayBuffer());
    if (record.getUint32(0, true) !== 0x06064b50 || record.getUint32(16, true) !== 0 || record.getUint32(20, true) !== 0 || record.getBigUint64(4, true) < 44n || recordOffset + 12n + record.getBigUint64(4, true) !== BigInt(eocdOffset - 20)) return invalid();
    const n = record.getBigUint64(32, true), s = record.getBigUint64(40, true), o = record.getBigUint64(48, true);
    if (record.getBigUint64(24, true) !== n) return invalid();
    if (s > BigInt(exportDirectoryLimit) || n > BigInt(Math.floor(exportDirectoryLimit / 46))) return limited();
    if (o > BigInt(exportZIPLimit)) return invalid();
    count = Number(n); size = Number(s); offset = Number(o); directoryEnd = Number(recordOffset);
  }
  if (size > exportDirectoryLimit) return limited();
  if (count < 1 || offset + size > directoryEnd || size < count * 46) return invalid();
  const directory = new DataView(await blob.slice(offset, offset + size).arrayBuffer());
  let cursor = 0;
  for (let i = 0; i < count; i++) {
    if (cursor + 46 > size || directory.getUint32(cursor, true) !== 0x02014b50) return invalid();
    const compressed = directory.getUint32(cursor + 20, true), local = directory.getUint32(cursor + 42, true);
    const name = directory.getUint16(cursor + 28, true), extra = directory.getUint16(cursor + 30, true), comment = directory.getUint16(cursor + 32, true);
    if (!name || local + compressed + 30 > offset || directory.getUint16(cursor + 34, true) !== 0) return invalid();
    cursor += 46 + name + extra + comment;
  }
  if (cursor !== size) return invalid();
}
