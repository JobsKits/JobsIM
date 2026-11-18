// Created by Jobs on 2026年11月15日，星期日.
import common from '@ohos.app.ability.common';
import util from '@ohos.util';
import fileUri from '@ohos.file.fileuri';
import notificationManager from '@ohos.notificationManager';
import notification from '@ohos.notification';
import { asset } from '@kit.AssetStoreKit';
import { filePreview } from '@kit.PreviewKit';
import { BusinessError } from '@kit.BasicServicesKit';
import fs from '@ohos.file.fs';
import cryptoFramework from '@ohos.security.cryptoFramework';
import { SyncCodec } from './SyncProtocol';

export class SecureSession {
  private aliasQuery(alias: string): asset.AssetMap {
    if (alias.length === 0) { throw new Error('SESSION_ALIAS_REQUIRED'); }
    const query: asset.AssetMap = new Map();
    query.set(asset.Tag.ALIAS, new util.TextEncoder().encodeInto(alias));
    return query;
  }
  async save(alias: string, token: string): Promise<void> {
    if (token.length === 0) { throw new Error('SESSION_TOKEN_REQUIRED'); }
    const attributes = this.aliasQuery(alias);
    const bytes = new util.TextEncoder().encodeInto(token);
    if (bytes.byteLength >= 1024) { bytes.fill(0); throw new Error('SESSION_ASSET_SIZE_LIMIT'); }
    attributes.set(asset.Tag.SECRET, bytes);
    attributes.set(asset.Tag.ACCESSIBILITY, asset.Accessibility.DEVICE_UNLOCKED);
    attributes.set(asset.Tag.SYNC_TYPE, asset.SyncType.NEVER);
    // Submit the complete pair in one OVERWRITE call; device crash behavior still needs validation.
    attributes.set(asset.Tag.CONFLICT_RESOLUTION, asset.ConflictResolution.OVERWRITE);
    try { await asset.add(attributes); }
    finally { bytes.fill(0); }
  }
  async load(alias: string): Promise<string> {
    const query = this.aliasQuery(alias);
    query.set(asset.Tag.RETURN_TYPE, asset.ReturnType.ALL);
    query.set(asset.Tag.RETURN_LIMIT, 1);
    let rows: asset.AssetMap[];
    try { rows = await asset.query(query); }
    catch (error) {
      if ((error as BusinessError).code === 24000002) { return ''; }
      throw new Error('AssetStore error ' + String((error as BusinessError).code));
    }
    if (rows.length === 0) { throw new Error('SESSION_ASSET_RESULT_MISSING'); }
    const value = rows[0].get(asset.Tag.SECRET);
    if (!(value instanceof Uint8Array)) { throw new Error('SESSION_ASSET_CORRUPTED'); }
    try { return util.TextDecoder.create('utf-8', { fatal: true }).decodeToString(value); }
    finally { value.fill(0); }
  }
  async remove(alias: string): Promise<void> {
    try { await asset.remove(this.aliasQuery(alias)); }
    catch (error) {
      if ((error as BusinessError).code !== 24000002) { throw new Error('AssetStore error ' + String((error as BusinessError).code)); }
    }
  }
}

export class NativeServices {
  static syncCodec(): SyncCodec {
    return {
      bytes: (text: string) => new util.TextEncoder().encodeInto(text),
      text: (bytes: Uint8Array) => util.TextDecoder.create('utf-8', { fatal: true }).decodeToString(bytes),
      decode: (text: string) => new util.Base64Helper().decodeSync(text.replace(/-/g, '+').replace(/_/g, '/') + '='.repeat((4 - text.length % 4) % 4)),
      encode: (bytes: Uint8Array) => new util.Base64Helper().encodeToStringSync(bytes).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, ''),
      sha256: async (bytes: Uint8Array) => {
        const md = cryptoFramework.createMd('SHA256'); await md.update({ data: bytes });
        const result = await md.digest(); return Array.from(result.data).map((value: number) => value.toString(16).padStart(2, '0')).join('');
      }
    };
  }
  static async clearSyncNotification(): Promise<void> { await notificationManager.cancel(46001); }
  static removeSyncCache(context: common.UIAbilityContext, path: string): void {
    if (!path) { return; }
    if (!path.startsWith(context.cacheDir + '/attachment-') || path.split('/').some((part: string) => part === '..' || part === '.')) { throw new Error('SYNC_CACHE_PATH_UNPROVEN'); }
    if (fs.accessSync(path)) { fs.unlinkSync(path); }
  }
  private static languages: string[] = ['zh-Hans', 'zh-Hant', 'bo', 'en', 'ja', 'fr', 'es', 'de', 'ru', 'ko'];
  private static genericMessages: string[] = ['有新消息', '有新訊息', 'འཕྲིན་ཡིག་གསར་པ་ཡོད།', 'New message', '新しいメッセージがあります', 'Nouveau message', 'Nuevo mensaje', 'Neue Nachricht', 'Новое сообщение', '새 메시지가 있습니다'];
  static notificationText(language: string): string {
    const index = NativeServices.languages.indexOf(language);
    return NativeServices.genericMessages[index < 0 ? 3 : index];
  }
  static async requestNotifications(context: common.UIAbilityContext): Promise<void> {
    await notificationManager.requestEnableNotification(context);
  }
  static async notify(_context: common.UIAbilityContext, title: string): Promise<void> {
    if (!(await notificationManager.isNotificationEnabled())) { return; }
    // Never publish caller-provided message text or a sender identity, even accidentally.
    const generic = NativeServices.genericMessages.includes(title) ? title : NativeServices.genericMessages[3];
    const request: notificationManager.NotificationRequest = {
      id: 46001,
      slotType: notification.SlotType.SOCIAL_COMMUNICATION,
      content: {
        notificationContentType: notificationManager.ContentType.NOTIFICATION_CONTENT_BASIC_TEXT,
        normal: { title: 'JobsIM', text: generic }
      }
    };
    await notificationManager.publish(request);
  }
  static async openFile(context: common.UIAbilityContext, path: string, mime: string, title: string): Promise<void> {
    const ownPath = path.startsWith(context.filesDir + '/') || path.startsWith(context.cacheDir + '/');
    if (!ownPath || path.split('/').some((part: string) => part === '..' || part === '.')) {
      throw new Error('SANDBOX_FILE_REQUIRED');
    }
    const uri = fileUri.getUriFromPath(path);
    if (!(await filePreview.canPreview(context, uri))) { throw new Error('FILE_PREVIEW_UNAVAILABLE'); }
    // The system preview is external UI and is opened only by an explicit user action.
    await filePreview.openPreview(context, { uri: uri, mimeType: mime, title: title });
  }
}
