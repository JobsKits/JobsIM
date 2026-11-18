// Created by Jobs on 2026年11月17日，星期二.
import { SyncJSON, SyncRules } from './SyncProtocol';
import { ReceiptService } from './ReceiptService';
export interface ReceiptCandidate {id:string;version:string;}
export interface ReceiptCachePage {rows:ReceiptCandidate[];after:string;}
export interface ReceiptCoordinatorPorts {enabled:()=>boolean;factory:(current:()=>boolean)=>Promise<ReceiptService>;cached:(after:string,limit:number)=>Promise<ReceiptCachePage>;}
export class ReceiptCoordinator {
  private ports:ReceiptCoordinatorPorts;private pending:Map<string,ReceiptCandidate>=new Map();private cursor:string='';private revision:number=0;private running:boolean=false;private service?:ReceiptService;
  constructor(ports:ReceiptCoordinatorPorts){this.ports=ports;}
  stop():void{this.revision++;this.service?.clear();this.service=undefined;this.running=false;this.pending.clear();this.cursor='';}
  offer(raws:string[]):void{
    if(!this.ports.enabled()){return;}for(const raw of raws){try{const m=SyncJSON.parse(raw);if(m.get('deleted').value!==true&&m.get('forwarded').value!==true&&m.get('contentUnavailable').value!==true&&SyncRules.uuid(m.get('id').string())&&m.get('contentVersion').string()!=='0'){const row:ReceiptCandidate={id:m.get('id').string(),version:SyncRules.counter(m.get('contentVersion').string())};this.pending.set(row.id,row);}}catch(_){}}
    while(this.pending.size>512){const first=Array.from(this.pending.keys())[0];this.pending.delete(first);}if(this.running){return;}const stamp=this.revision;this.running=true;this.drain(stamp).finally(()=>{if(stamp===this.revision){this.running=false;}});
  }
  private async drain(stamp:number):Promise<void>{let service:ReceiptService|undefined;const current=():boolean=>stamp===this.revision&&this.ports.enabled();try{if(!current()){return;}service=await this.ports.factory(current);this.service=service;await service.reconcile();if(!current()){return;}const page=await this.ports.cached(this.cursor,8);if(!current()){return;}this.cursor=page.after;for(const row of page.rows){if(!this.pending.has(row.id)){this.pending.set(row.id,row);}}const items=Array.from(this.pending.values()).slice(0,8);for(const row of items){if(!current()){return;}try{await service.receiveIfTarget(row.id,row.version);}catch(_){if(!current()){return;}}this.pending.delete(row.id);}}catch(_){}finally{service?.clear();if(stamp===this.revision){this.service=undefined;}}}
}
