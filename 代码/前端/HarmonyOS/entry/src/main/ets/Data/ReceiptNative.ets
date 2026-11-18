// Created by Jobs on 2026年11月17日，星期二.
import common from '@ohos.app.ability.common';
import fs from '@ohos.file.fs';
import statvfs from '@ohos.file.statvfs';
import { NativeStorageBudget } from './NativeStorageBudget';
import util from '@ohos.util';
import cryptoFramework from '@ohos.security.cryptoFramework';
import { MigrationIdentity, MigrationRules, MigrationCodec } from './MigrationProtocol';
import { SyncJSON } from './SyncProtocol';
export class ReceiptFiles {
  private static buildKey:string='';private static buildHash:string='';
  private context:common.UIAbilityContext;private identity:MigrationIdentity;private current:()=>boolean;private directory:string;
  constructor(context:common.UIAbilityContext,identity:MigrationIdentity,current:()=>boolean){this.context=context;this.identity=identity;this.current=current;const root=context.filesDir+'/receipt-originals';this.ensure(root);this.directory=root+'/'+encodeURIComponent(identity.namespace);this.ensure(this.directory);}
  private check():void{if(!this.current()){MigrationRules.fail('RECEIPT_CONTEXT_CHANGED');}}
  private ensure(path:string):void{if(!fs.accessSync(path)){fs.mkdirSync(path);this.sync(path.slice(0,path.lastIndexOf('/')));}const info=fs.lstatSync(path);if(!info.isDirectory()||info.isSymbolicLink()){MigrationRules.fail('RECEIPT_UNSAFE_PATH');}}
  private sync(path:string):void{const file=fs.openSync(path,fs.OpenMode.READ_ONLY);try{fs.fsyncSync(file.fd);}finally{fs.closeSync(file);}}
  disk(required:number=0):void{this.check();if(!NativeStorageBudget.fits(statvfs.getFreeSizeSync(this.context.filesDir),required)){MigrationRules.fail('RECEIPT_STORAGE_LIMIT');}}
  private path(name:string):string{if(!/^[0-9a-f-]{36}\.bin$/.test(name)){MigrationRules.fail('RECEIPT_UNSAFE_PATH');}const path=this.directory+'/'+name;if(fs.accessSync(path)){const info=fs.lstatSync(path);if(!info.isFile()||info.isSymbolicLink()){MigrationRules.fail('RECEIPT_UNSAFE_PATH');}}return path;}
  async digest(path:string):Promise<string>{const file=fs.openSync(path,fs.OpenMode.READ_ONLY);try{const info=fs.statSync(file.fd);if(!info.isFile()){MigrationRules.fail();}const md=cryptoFramework.createMd('SHA256');let offset=0;while(offset<info.size){this.check();const count=Math.min(65536,info.size-offset);const data=new ArrayBuffer(count);fs.lseek(file.fd,offset,fs.WhenceType.SEEK_SET);if(fs.readSync(file.fd,data)!==count){MigrationRules.fail();}await md.update({data:new Uint8Array(data)});offset+=count;}const result=await md.digest();this.check();return Array.from(result.data).map((value:number)=>value.toString(16).padStart(2,'0')).join('');}finally{fs.closeSync(file);}}
  async buildDigest():Promise<string>{
    this.check();const root=this.context.bundleCodeDir;const info=fs.lstatSync(root);if(!info.isDirectory()||info.isSymbolicLink()){MigrationRules.fail('RECEIPT_BUILD_UNREADABLE');}const names=fs.listFileSync(root).filter((name:string)=>/^[A-Za-z0-9_.-]+\.hap$/.test(name));if(names.length!==1){MigrationRules.fail('RECEIPT_BUILD_UNREADABLE');}const path=root+'/'+names[0];const target=fs.lstatSync(path);if(!target.isFile()||target.isSymbolicLink()){MigrationRules.fail('RECEIPT_BUILD_UNREADABLE');}const key=path+'|'+String(target.size)+'|'+String(target.mtime);if(ReceiptFiles.buildKey===key&&ReceiptFiles.buildHash){this.check();return ReceiptFiles.buildHash;}const hash=await this.digest(path);ReceiptFiles.buildKey=key;ReceiptFiles.buildHash=hash;return hash;
  }
  async save(id:string,ref:string,bytes:Uint8Array,codec:MigrationCodec):Promise<string>{
    this.disk(bytes.length);const object=SyncJSON.parse(ref);if(bytes.length!==Number(object.get('sizeBytes').string())||bytes.length>64*1024*1024||await codec.hash(bytes)!==object.get('sha256').string()){MigrationRules.fail('RECEIPT_OBJECT_MISMATCH');}const name=MigrationRules.id(id)+'.bin';const destination=this.path(name);const temporary=this.directory+'/'+util.generateRandomUUID(true)+'.part';const file=fs.openSync(temporary,fs.OpenMode.CREATE|fs.OpenMode.READ_WRITE|fs.OpenMode.TRUNC);let completed=false;
    try{let offset=0;while(offset<bytes.length){this.check();const count=fs.writeSync(file.fd,bytes.buffer.slice(bytes.byteOffset+offset,bytes.byteOffset+bytes.length));if(count<=0){MigrationRules.fail();}offset+=count;}fs.fsyncSync(file.fd);this.check();completed=true;}finally{fs.closeSync(file);if(!completed&&fs.accessSync(temporary)){fs.unlinkSync(temporary);}}
    try{this.check();fs.renameSync(temporary,destination);this.sync(this.directory);}catch(error){if(fs.accessSync(temporary)){fs.unlinkSync(temporary);}throw error as Error;}
    const descriptor:Record<string,Object>={};descriptor['id']=object.get('objectId').string();descriptor['path']=name;descriptor['sha256']=object.get('sha256').string();descriptor['sizeBytes']=object.get('sizeBytes').string();descriptor['contentType']=object.get('contentType').string();const raw=JSON.stringify(descriptor);await this.verify(raw);return raw;
  }
  async verify(raw:string):Promise<void>{const d=SyncJSON.parse(raw);const path=this.path(d.get('path').string());if(String(fs.lstatSync(path).size)!==d.get('sizeBytes').string()||await this.digest(path)!==d.get('sha256').string()){MigrationRules.fail('RECEIPT_OBJECT_MISMATCH');}this.check();}
  async read(raw:string):Promise<Uint8Array>{await this.verify(raw);const d=SyncJSON.parse(raw);const path=this.path(d.get('path').string());const file=fs.openSync(path,fs.OpenMode.READ_ONLY);try{const size=fs.statSync(file.fd).size;if(size>64*1024*1024){MigrationRules.fail();}const data=new ArrayBuffer(size);if(size>0&&fs.readSync(file.fd,data)!==size){MigrationRules.fail();}this.check();return new Uint8Array(data);}finally{fs.closeSync(file);}}
  erase(names:string[]):void{ReceiptFiles.eraseOwned(this.context,this.identity.namespace,names);}
  static eraseOwned(context:common.UIAbilityContext,namespace:string,names:string[]):void{
    const parts=namespace.split('|');if(parts.length!==7||parts[0]!=='live'||parts[6]!=='personal'||!parts[1]){MigrationRules.fail('RECEIPT_UNSAFE_PATH');}for(const index of [2,3,5]){MigrationRules.id(parts[index]);}MigrationRules.positive(parts[4]);const root=context.filesDir+'/receipt-originals';const dir=root+'/'+encodeURIComponent(namespace);if(!fs.accessSync(root)||!fs.accessSync(dir)){return;}for(const path of [root,dir]){const info=fs.lstatSync(path);if(!info.isDirectory()||info.isSymbolicLink()){MigrationRules.fail('RECEIPT_UNSAFE_PATH');}}
    for(const name of names){if(!/^[0-9a-f-]{36}\.(bin|part)$/.test(name)){MigrationRules.fail('RECEIPT_UNSAFE_PATH');}const path=dir+'/'+name;if(fs.accessSync(path)){const info=fs.lstatSync(path);if(!info.isFile()||info.isSymbolicLink()){MigrationRules.fail('RECEIPT_UNSAFE_PATH');}fs.unlinkSync(path);}}
    const file=fs.openSync(dir,fs.OpenMode.READ_ONLY);try{fs.fsyncSync(file.fd);}finally{fs.closeSync(file);}
  }
}
