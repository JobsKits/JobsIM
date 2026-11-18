// Created by Jobs on 2026年11月17日，星期二.
import fs from '@ohos.file.fs';
import common from '@ohos.app.ability.common';
import { DeletionLocalPlan } from './DeletionStore';

export class DeletionFiles {
  static clear(context: common.UIAbilityContext, plan: DeletionLocalPlan): void {
    for (const path of plan.files) {
      if (!path) { continue; }
      const cached = path.startsWith(context.cacheDir + '/') ? path.slice(context.cacheDir.length + 1) : '';
      const original = path.startsWith(context.filesDir + '/') ? path.slice(context.filesDir.length + 1) : '';
      if (!/^(attachment-|own-avatar-)[0-9a-f-]{36}\.[a-z0-9]+$/.test(cached) && !/^avatar-original-[0-9a-f-]{36}$/.test(original)) { throw new Error('DELETION_STORAGE_BINDING_CHANGED'); }
      if (fs.accessSync(path)) { fs.unlinkSync(path); }
    }
    const root = context.cacheDir + '/personal-exports';
    for (const namespace of plan.namespaces) {
      const directory = root + '/' + encodeURIComponent(namespace);
      if (!fs.accessSync(directory)) { continue; }
      for (const name of fs.listFileSync(directory)) {
        if (!/^[0-9a-f-]{36}\.(part|zip)$/.test(name)) { throw new Error('DELETION_STORAGE_BINDING_CHANGED'); }
        fs.unlinkSync(directory + '/' + name);
      }
      fs.rmdirSync(directory);
    }
    for (const directory of [context.cacheDir, context.filesDir, root]) {
      if (fs.accessSync(directory)) { const file = fs.openSync(directory, fs.OpenMode.READ_ONLY); try { fs.fsyncSync(file.fd); } finally { fs.closeSync(file); } }
    }
  }
}
