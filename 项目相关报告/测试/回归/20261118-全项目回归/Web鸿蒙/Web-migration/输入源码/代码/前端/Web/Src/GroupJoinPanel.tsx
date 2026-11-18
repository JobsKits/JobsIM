// Created by Jobs on 2026年11月17日，星期二.
import { useEffect, useRef, useState, type FormEvent } from "react";
import { Users, RefreshCw } from "lucide-react";
import { GroupGovernanceActions, type GroupGovernanceContext } from "./GroupGovernanceActions";
import { groupLabel } from "./GroupGovernanceLabels";
import type { GroupOperation } from "./GroupGovernanceStore";

export default function GroupJoinPanel({context,locale,t,onChanged,onLogin}:{context?:GroupGovernanceContext;locale:string;t:(key:string)=>string;onChanged:()=>Promise<void>;onLogin:()=>void}){
  const label=(key:string)=>groupLabel(locale,key),[rows,setRows]=useState<GroupOperation[]>([]),[busy,setBusy]=useState(false),[failure,setFailure]=useState(false),[shown,setShown]=useState(20);
  const live=useRef(context),mounted=useRef(false),generation=useRef(0),working=useRef(false),form=useRef<HTMLFormElement>(null);live.current=context;
  const capture=()=>{const original=live.current,g=generation.current;return original?new GroupGovernanceActions({...original,isCurrent:()=>mounted.current&&g===generation.current&&original.isCurrent()}):undefined};
  const load=async(source:GroupGovernanceActions)=>{const values=await source.rows();if(source.context.isCurrent())setRows(values)};
  const run=async(action:(source:GroupGovernanceActions)=>Promise<void>)=>{
    const source=capture(),g=generation.current;if(!source||working.current)return;working.current=true;setBusy(true);setFailure(false);
    try{await action(source)}catch{if(source.context.isCurrent())setFailure(true)}finally{if(source.context.isCurrent()){await load(source).catch(()=>setFailure(true));setBusy(false)}if(g===generation.current)working.current=false}
  };
  useEffect(()=>{
    mounted.current=true;generation.current++;working.current=false;setRows([]);setBusy(false);setFailure(false);setShown(20);form.current?.reset();const source=capture();if(source)void load(source).catch(()=>{if(source.context.isCurrent())setFailure(true)});
    return()=>{mounted.current=false;generation.current++;form.current?.reset()};
  },[context?.identity.familyId,context?.identity.namespace]);
  const submit=(event:FormEvent<HTMLFormElement>)=>{
    event.preventDefault();const data=new FormData(event.currentTarget),code=String(data.get("code")??""),otp=String(data.get("otp")??""),consent=data.get("consent")==="on",value=((event.nativeEvent as SubmitEvent).submitter as HTMLButtonElement|null)?.value??"join";
    event.currentTarget.reset();if(!consent){setFailure(true);return}
    void run(async source=>{if(value==="join")await source.join(code);else await source.resume(value,true,code,otp);if(source.context.isCurrent())await onChanged()});
  };
  const name=(row:GroupOperation)=>row.kind==="join"?label("joinByCode"):row.kind==="invite"?label("newInvite"):row.kind==="revoke"?t("remove"):row.kind==="decide"?label("applications"):JSON.parse(row.originalBody).action==="transfer"?t("transferOwner"):JSON.parse(row.originalBody).action==="recover"?label("recover"):label("title");
  return <section className="group-join-panel" aria-label={label("joinByCode")}>
    <div className="self-state-heading"><h3>{label("joinByCode")}</h3><button className="icon-button" type="button" aria-label={t("reload")} disabled={busy||!context} onClick={()=>void run(load)}><RefreshCw size={17}/></button></div>
    {!context&&<div className="empty"><Users size={28}/><h4>{t("sessionExpired")}</h4><button type="button" className="secondary" onClick={onLogin}>{t("signIn")}</button></div>}
    <form ref={form} className="password-change-form" onSubmit={submit} noValidate>
      <label>{label("joinByCode")}<textarea name="code" autoComplete="off" spellCheck={false} maxLength={4096} rows={4} disabled={busy||!context}/></label>
      <p className="self-state-hint">{label("shareHint")}</p>
      <label className="password-change-consent"><input name="consent" type="checkbox" disabled={busy||!context}/>{label("joinConfirm")}</label>
      <button className="primary" type="submit" value="join" disabled={busy||!context}>{busy?t("working"):label("joinByCode")}</button>
      <h4>{label("originals")}</h4>
      {!rows.length?<div className="empty"><Users size={26}/><h4>{label("loadHint")}</h4><button type="button" className="secondary" disabled={busy||!context} onClick={()=>void run(load)}>{t("reload")}</button></div>:rows.slice(0,shown).map(row=><article className="self-state-panel" key={row.requestId}><div className="self-state-heading"><h4>{name(row)}</h4><span role="status">{label(row.state)}</span></div><p className="self-state-hint">{row.conversationId} · {row.requestId}</p>{row.kind==="join"&&row.result&&<p>{t(row.result.state==="pending"?"pendingReview":row.result.state==="approved"?"accepted":"rejected")}</p>}<div className="self-state-buttons"><button className="secondary" type="button" disabled={busy} onClick={()=>{form.current?.reset();void run(async source=>{await source.resume(row.requestId,false)})}}>{label("check")}</button>{row.state!=="applied"&&row.state!=="blocked"&&<button className="secondary" type="submit" value={row.requestId} disabled={busy}>{label("retry")}</button>}</div></article>)}
      {rows.length>shown&&<button className="secondary" type="button" onClick={()=>setShown(shown+20)}>{t("loadMore")}</button>}
      {rows.some(row=>row.kind==="governance"&&JSON.parse(row.originalBody).action==="recover"&&row.state!=="applied")&&<label>{t("otp")}<input name="otp" inputMode="numeric" autoComplete="one-time-code" maxLength={6}/></label>}
    </form>
    {failure&&<p role="alert">{t("unknownResult")} {label("permissionChanged")}</p>}
  </section>;
}
