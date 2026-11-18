// Created by Jobs on 2026年11月17日，星期二.
import fs from '@ohos.file.fs';
import common from '@ohos.app.ability.common';
import { DeletionLocalPlan } from './DeletionStore';
import { MigrationFiles } from './MigrationNative';
import { ReceiptFiles } from './ReceiptNative';

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
      const receiptRoot=context.filesDir+'/receipt-originals';const receipt=receiptRoot+'/'+encodeURIComponent(namespace);if(fs.accessSync(receipt)){ReceiptFiles.eraseOwned(context,namespace,fs.listFileSync(receipt));fs.rmdirSync(receipt);const handle=fs.openSync(receiptRoot,fs.OpenMode.READ_ONLY);try{fs.fsyncSync(handle.fd);}finally{fs.closeSync(handle);}}
      const migrationRoot=context.filesDir+'/personal-migrations';const migration=migrationRoot+'/'+encodeURIComponent(namespace);
      if(fs.accessSync(migration)){for(const path of [migrationRoot,migration]){const info=fs.lstatSync(path);if(!info.isDirectory()||info.isSymbolicLink()){throw new Error('DELETION_STORAGE_BINDING_CHANGED');}}MigrationFiles.eraseOwned(context,namespace,fs.listFileSync(migration));fs.rmdirSync(migration);const directory=fs.openSync(migrationRoot,fs.OpenMode.READ_ONLY);try{fs.fsyncSync(directory.fd);}finally{fs.closeSync(directory);}}

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
