// Created by Jobs on 2026年11月17日，星期二.
import common from '@ohos.app.ability.common';
import fs from '@ohos.file.fs';
import statvfs from '@ohos.file.statvfs';
import { NativeStorageBudget } from './NativeStorageBudget';
import util from '@ohos.util';
import cryptoFramework from '@ohos.security.cryptoFramework';
import { MigrationCodec, MigrationIdentity, MigrationView, MigrationHistory, MigrationRules, MigrationFilePort } from './MigrationProtocol';

export class MigrationNative {
  static codec(): MigrationCodec {
    return {
      bytes: (text: string) => new util.TextEncoder().encodeInto(text),
      text: (bytes: Uint8Array) => util.TextDecoder.create('utf-8',{fatal:true}).decodeToString(bytes),
      encode: (bytes: Uint8Array, url: boolean) => { const encoded=new util.Base64Helper().encodeToStringSync(bytes);return url?encoded.replace(/\+/g,'-').replace(/\//g,'_').replace(/=+$/,''):encoded; },
      decode: (text: string, max: number, url: boolean) => {
        if(text.length>Math.ceil(max/3)*4 || !(url?/^[A-Za-z0-9_-]*$/:/^(?:[A-Za-z0-9+/]{4})*(?:[A-Za-z0-9+/]{2}==|[A-Za-z0-9+/]{3}=)?$/).test(text)){MigrationRules.fail();}
        const value=url?text.replace(/-/g,'+').replace(/_/g,'/')+'='.repeat((4-text.length%4)%4):text;const bytes=new util.Base64Helper().decodeSync(value);if(bytes.length>max){MigrationRules.fail();}return bytes;
      },
      hash: async (bytes: Uint8Array) => { const md=cryptoFramework.createMd('SHA256');await md.update({data:bytes});const result=await md.digest();return Array.from(result.data).map((byte: number)=>byte.toString(16).padStart(2,'0')).join(''); },
      decrypt: async (key: Uint8Array, iv: Uint8Array, aad: Uint8Array, encrypted: Uint8Array) => {
        if(key.length!==32 || iv.length!==12 || encrypted.length<16){MigrationRules.fail();}const generator=cryptoFramework.createSymKeyGenerator('AES256');const secret=await generator.convertKey({data:key});
        try { const cipher=cryptoFramework.createCipher('AES256|GCM|NoPadding');const params: cryptoFramework.GcmParamsSpec={algName:'GcmParamsSpec',iv:{data:iv},aad:{data:aad},authTag:{data:encrypted.slice(encrypted.length-16)}};await cipher.init(cryptoFramework.CryptoMode.DECRYPT_MODE,secret,params);const plain=await cipher.doFinal({data:encrypted.slice(0,encrypted.length-16)});return plain.data; }
        finally {secret.clearMem();key.fill(0);}
      }
    };
  }
}
export class MigrationFiles implements MigrationFilePort {
  static eraseOwned(context:common.UIAbilityContext,namespace:string,names:string[]):void{
    const parts=namespace.split('|');if(parts.length!==7||parts[0]!=='live'||parts[6]!=='personal'||!parts[1]){MigrationRules.fail('MIGRATION_UNSAFE_PATH');}for(const index of [2,3,5]){MigrationRules.id(parts[index]);}MigrationRules.positive(parts[4]);const root=context.filesDir+'/personal-migrations';const directory=root+'/'+encodeURIComponent(namespace);if(!fs.accessSync(root)||!fs.accessSync(directory)){return;}for(const path of [root,directory]){const info=fs.lstatSync(path);if(!info.isDirectory()||info.isSymbolicLink()){MigrationRules.fail('MIGRATION_UNSAFE_PATH');}}
    for(const name of names){if(!/^[0-9a-f-]{36}(?:-[0-9a-f-]{36})?\.(capsule|object|part)$/.test(name)){MigrationRules.fail('MIGRATION_UNSAFE_PATH');}const path=directory+'/'+name;if(fs.accessSync(path)){const info=fs.lstatSync(path);if(!info.isFile()||info.isSymbolicLink()){MigrationRules.fail('MIGRATION_UNSAFE_PATH');}fs.unlinkSync(path);}}
    const file=fs.openSync(directory,fs.OpenMode.READ_ONLY);try{fs.fsyncSync(file.fd);}finally{fs.closeSync(file);}
  }
  private context: common.UIAbilityContext;private current:()=>boolean;private identity:MigrationIdentity;private directory:string;
  constructor(context:common.UIAbilityContext,identity:MigrationIdentity,current:()=>boolean){this.context=context;this.current=current;this.identity=identity;const root=context.filesDir+'/personal-migrations';this.directory=root+'/'+encodeURIComponent(identity.namespace);this.ensure(root);this.ensure(this.directory);}
  private check():void{if(!this.current()){MigrationRules.fail('MIGRATION_CONTEXT_CHANGED');}}
  private ensure(path:string):void{if(!fs.accessSync(path)){fs.mkdirSync(path);this.sync(path.slice(0,path.lastIndexOf('/')));}const info=fs.lstatSync(path);if(!info.isDirectory()||info.isSymbolicLink()){MigrationRules.fail('MIGRATION_UNSAFE_PATH');}}
  private sync(path:string):void{const dir=fs.openSync(path,fs.OpenMode.READ_ONLY);try{fs.fsyncSync(dir.fd);}finally{fs.closeSync(dir);}}
  disk(required:number=0):void{this.check();if(!NativeStorageBudget.fits(statvfs.getFreeSizeSync(this.context.filesDir),required)){MigrationRules.fail('MIGRATION_STORAGE_LIMIT');}}
  private path(name:string):string{if(!/^[0-9a-f-]{36}(?:-[0-9a-f-]{36})?\.(capsule|object)$/.test(name)){MigrationRules.fail('MIGRATION_UNSAFE_PATH');}const path=this.directory+'/'+name;if(fs.accessSync(path)&&fs.lstatSync(path).isSymbolicLink()){MigrationRules.fail('MIGRATION_UNSAFE_PATH');}return path;}
  private write(name:string,bytes:Uint8Array):void{
    this.check();this.disk(bytes.length);const destination=this.path(name);const temp=this.directory+'/'+util.generateRandomUUID(true)+'.part';const file=fs.openSync(temp,fs.OpenMode.CREATE|fs.OpenMode.READ_WRITE|fs.OpenMode.TRUNC);let complete=false;
    try{let offset=0;while(offset<bytes.length){this.check();const written=fs.writeSync(file.fd,bytes.buffer.slice(bytes.byteOffset+offset,bytes.byteOffset+bytes.byteLength));if(written<=0){MigrationRules.fail('MIGRATION_FILE_INCOMPLETE');}offset+=written;}fs.fsyncSync(file.fd);this.check();complete=true;}finally{fs.closeSync(file);if(!complete&&fs.accessSync(temp)){fs.unlinkSync(temp);}}
    try{this.check();fs.renameSync(temp,destination);this.sync(this.directory);}catch(error){if(fs.accessSync(temp)){fs.unlinkSync(temp);}throw error as Error;}
  }
  async save(v:MigrationView,bytes:Uint8Array,codec:MigrationCodec):Promise<void>{this.disk(bytes.length);if(bytes.length!==v.size||await codec.hash(bytes)!==v.capsuleDigest){MigrationRules.fail();}this.check();this.write(v.id+'.capsule',bytes);}
  async load(v:MigrationView,codec:MigrationCodec):Promise<Uint8Array|undefined>{const path=this.path(v.id+'.capsule');if(!fs.accessSync(path)){return undefined;}const file=fs.openSync(path,fs.OpenMode.READ_ONLY);let bytes:Uint8Array;try{const size=fs.statSync(file.fd).size;if(size!==v.size||size>MigrationRules.limit+4096){MigrationRules.fail();}const data=new ArrayBuffer(size);if(fs.readSync(file.fd,data)!==size){MigrationRules.fail();}bytes=new Uint8Array(data);}finally{fs.closeSync(file);}if(await codec.hash(bytes)!==v.capsuleDigest){bytes.fill(0);MigrationRules.fail();}this.check();return bytes;}
  stage(v:MigrationView,history:MigrationHistory):Map<string,string>{this.disk();const files=new Map<string,string>();for(const object of history.objects){const name=MigrationRules.id(v.id)+'-'+MigrationRules.id(object.id)+'.object';this.write(name,object.bytes);files.set(object.id,name);}this.check();return files;}
  erase(names:string[]):void{for(const name of names){this.check();const path=this.path(name);if(fs.accessSync(path)){if(!fs.lstatSync(path).isFile()){MigrationRules.fail('MIGRATION_UNSAFE_PATH');}fs.unlinkSync(path);this.sync(this.directory);}}}
  discard(id:string,referenced:string[]):void{MigrationRules.id(id);const names=fs.listFileSync(this.directory).filter((name:string)=>(name===id+'.capsule'||name.startsWith(id+'-')&&name.endsWith('.object'))&&!referenced.includes(name));MigrationFiles.eraseOwned(this.context,this.identity.namespace,names);}
  object(name:string):ArrayBuffer{this.check();const path=this.path(name);const file=fs.openSync(path,fs.OpenMode.READ_ONLY);try{const size=fs.statSync(file.fd).size;if(size>64*1024*1024){MigrationRules.fail();}const bytes=new ArrayBuffer(size);if(size>0&&fs.readSync(file.fd,bytes)!==size){MigrationRules.fail();}this.check();return bytes;}finally{fs.closeSync(file);}}
}
