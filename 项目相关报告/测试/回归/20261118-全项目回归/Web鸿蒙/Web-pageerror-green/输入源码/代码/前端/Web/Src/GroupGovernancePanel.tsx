// Created by Jobs on 2026年11月17日，星期二.
import { useEffect, useRef, useState, type FormEvent } from "react";
import { RefreshCw, Users } from "lucide-react";
import { ApiError } from "./api";
import { SyncProtocolError } from "./SyncProtocol";
import { GroupGovernanceActions, type GroupGovernanceContext } from "./GroupGovernanceActions";
import { shareGroupInvite, type GroupGovernance, type GroupInvite, type GroupJoin } from "./GroupGovernanceProtocol";
import { groupLabel } from "./GroupGovernanceLabels";
import type { GroupOperation } from "./GroupGovernanceStore";
import type { User } from "./types";

export default function GroupGovernancePanel({ context, conversationId, members, locale, t, onChanged, onMessage }: { context?: GroupGovernanceContext; conversationId: string; members: User[]; locale: string; t: (key: string) => string; onChanged: () => Promise<void>; onMessage: (message: string) => void }) {
  const label = (key: string) => groupLabel(locale,key);
  const [snapshot,setSnapshot]=useState<GroupGovernance|null>(null),[invites,setInvites]=useState<GroupInvite[]>([]),[applications,setApplications]=useState<GroupJoin[]>([]),[inviteCursor,setInviteCursor]=useState(""),[applicationCursor,setApplicationCursor]=useState("");
  const [rows,setRows]=useState<GroupOperation[]>([]),[target,setTarget]=useState<User|null>(null),[consent,setConsent]=useState(false),[busy,setBusy]=useState(false),[failure,setFailure]=useState(""),[shown,setShown]=useState(20);
  const mounted=useRef(false),generation=useRef(0),working=useRef(false),live=useRef(context),recoverForm=useRef<HTMLFormElement>(null),retryForm=useRef<HTMLFormElement>(null);live.current=context;
  const capture=()=>{const own=live.current,g= generation.current;return own?new GroupGovernanceActions({...own,isCurrent:()=>mounted.current&&g===generation.current&&own.isCurrent()}):undefined};
  const originals=async(source:GroupGovernanceActions)=>{const all=await source.rows();if(source.context.isCurrent())setRows(all.filter(row=>row.conversationId===conversationId&&row.kind!=="join"))};
  const load=async(source:GroupGovernanceActions)=>{
    const value=await source.governance(conversationId);if(!source.context.isCurrent())return;
    setSnapshot(value);setTarget(null);setConsent(false);setInvites([]);setApplications([]);setInviteCursor("");setApplicationCursor("");recoverForm.current?.reset();retryForm.current?.reset();
    if(value.state==="active"&&["owner","admin"].includes(value.role)){
      const results=await Promise.allSettled([source.invites(conversationId),source.applications(conversationId)]);
      if(!source.context.isCurrent())return;
      if(results[0].status==="fulfilled"){setInvites(results[0].value.items);setInviteCursor(results[0].value.nextCursor)}else setFailure("failure");
      if(results[1].status==="fulfilled"){setApplications(results[1].value.items);setApplicationCursor(results[1].value.nextCursor)}else setFailure("failure");
    }
  };
  const error=(cause:unknown)=>cause instanceof ApiError&&["GROUP_PERMISSION_CHANGED","GROUP_TARGET_CHANGED","GROUP_READ_ONLY","GROUP_ADMIN_REQUIRED","OWNER_REQUIRED"].includes(cause.code)?"permissionChanged":cause instanceof SyncProtocolError&&["GROUP_INVITE_REQUIRED","GROUP_PROOF_REQUIRED","GROUP_PROTOCOL_INVALID"].includes(cause.code)?"invalid":"failure";
  const run=async(action:(source:GroupGovernanceActions)=>Promise<void>,changed=false)=>{
    const source=capture(),g=generation.current;if(!source||working.current)return;
    working.current=true;setBusy(true);setFailure("");
    try{await action(source);if(changed&&source.context.isCurrent()){await onChanged();if(source.context.isCurrent())await load(source)}}
    catch(cause){if(source.context.isCurrent())setFailure(error(cause))}
    finally{if(source.context.isCurrent()){await originals(source).catch(()=>setFailure("failure"));setBusy(false)}if(g===generation.current)working.current=false}
  };
  useEffect(()=>{
    mounted.current=true;generation.current++;working.current=false;setSnapshot(null);setTarget(null);setInvites([]);setApplications([]);setRows([]);setFailure("");setBusy(false);setConsent(false);setShown(20);recoverForm.current?.reset();retryForm.current?.reset();
    const source=capture();if(source){setBusy(true);void Promise.allSettled([load(source),originals(source)]).then(results=>{if(source.context.isCurrent()){setBusy(false);if(results.some(value=>value.status==="rejected"))setFailure("failure")}})};
    return()=>{mounted.current=false;generation.current++;recoverForm.current?.reset();retryForm.current?.reset()};
  },[context?.identity.namespace,context?.identity.familyId,conversationId]);
  const write=(action:(source:GroupGovernanceActions,original:GroupGovernance)=>Promise<unknown>)=>{
    if(!snapshot||!consent){setFailure("invalid");return}
    const original=structuredClone(snapshot);setConsent(false);void run(async source=>{await action(source,original)},true);
  };
  const policy=(event:FormEvent<HTMLFormElement>)=>{
    event.preventDefault();const data=new FormData(event.currentTarget),slowSeconds=Number(data.get("slowSeconds")),maxMembers=Number(data.get("maxMembers")),muteAll=data.get("muteAll")==="on";
    if(!Number.isInteger(slowSeconds)||slowSeconds<0||slowSeconds>3600||!Number.isInteger(maxMembers)||maxMembers<2||maxMembers>10000){setFailure("invalid");return}
    write((source,original)=>source.mutate(conversationId,original,{action:"settings",slowSeconds,maxMembers,muteAll}));
  };
  const invitation=(event:FormEvent<HTMLFormElement>)=>{
    event.preventDefault();const data=new FormData(event.currentTarget),hours=Number(data.get("hours")),maxUses=Number(data.get("maxUses")),approval=data.get("approval")==="on";
    if(!Number.isInteger(hours)||hours<1||hours>720||!Number.isInteger(maxUses)||maxUses<1||maxUses>10000){setFailure("invalid");return}
    const deadline=new Date(Date.now()+hours*3600000).toISOString();write((source,original)=>source.invite(conversationId,original,deadline,maxUses,approval));
  };
  const recover=(event:FormEvent<HTMLFormElement>)=>{event.preventDefault();const otp=String(new FormData(event.currentTarget).get("otp")??"");event.currentTarget.reset();if(!target||!/^\d{6}$/.test(otp)){setFailure("invalid");return}const chosen=structuredClone(target);write((source,original)=>source.recover(conversationId,original,chosen,otp))};
  const retry=(event:FormEvent<HTMLFormElement>)=>{event.preventDefault();const data=new FormData(event.currentTarget),id=((event.nativeEvent as SubmitEvent).submitter as HTMLButtonElement|null)?.value??"",otp=String(data.get("otp")??"");event.currentTarget.reset();if(!consent){setFailure("invalid");return}setConsent(false);void run(async source=>{await source.resume(id,true,undefined,otp)},true)};
  const copy=(invite:GroupInvite)=>void run(async source=>{
    // Reconcile this exact invitation again before copying a secret to the
    // user's clipboard; list state alone cannot reissue a revoked grant.
    let after="",current:GroupInvite|undefined;
    for(let page=0;page<200;page++){
      const result=await source.invites(conversationId,after);current=result.items.find(row=>row.id===invite.id);if(current||!result.nextCursor)break;after=result.nextCursor;
    }
    if(!current||current.revokedAt||Date.parse(current.expiresAt)<=Date.now()||current.uses>=current.maxUses||!source.context.isCurrent())throw Error("GROUP_INVITE_REQUIRED");
    const code=shareGroupInvite(source.context.identity.scopeId,conversationId,current);if(!source.context.isCurrent())return;await navigator.clipboard.writeText(code);if(source.context.isCurrent())onMessage(t("copied"));
  });
  const manage=snapshot?.state==="active"&&!snapshot.readOnly&&["owner","admin"].includes(snapshot.role),owner=manage&&snapshot?.role==="owner";
  const eligible=members.filter(user=>(!user.state||user.state==="active")&&(snapshot?.canRecover||user.id!==context?.identity.userId));
  return <section className="group-governance-panel" aria-label={label("title")}>
    <div className="self-state-heading"><h3>{label("title")}</h3><button type="button" className="icon-button" disabled={busy||!context} aria-label={t("reload")} onClick={()=>void run(async source=>{await onChanged();await load(source)})}><RefreshCw size={17}/></button></div>
    {!snapshot?<div className="empty"><Users size={26}/><h4>{t(busy?"working":context?"failure":"notSupported")}</h4><button className="secondary" disabled={busy||!context} onClick={()=>void run(load)}>{t("reload")}</button></div>:<>
      <p className="self-state-hint">{t(snapshot.role==="owner"?"owner":snapshot.role==="admin"?"admin":"members")} · #{snapshot.version}</p>
      {(manage||snapshot.canRecover)&&<label className="password-change-consent group-confirm"><input type="checkbox" checked={consent} onChange={event=>setConsent(event.target.checked)}/>{label("confirm")}</label>}
      <form className="password-change-form" key={conversationId+"|"+snapshot.version} onSubmit={policy} noValidate>
        <label className="password-change-consent"><input name="muteAll" type="checkbox" defaultChecked={snapshot.policy.muteAll} disabled={!owner||busy}/>{label("muteAll")}</label>
        <label>{label("slowSeconds")}<input name="slowSeconds" type="number" min="0" max="3600" defaultValue={snapshot.policy.slowSeconds} disabled={!owner||busy}/></label>
        <label>{label("maxMembers")}<input name="maxMembers" type="number" min="2" max="10000" defaultValue={snapshot.policy.maxMembers} disabled={!owner||busy}/></label>
        {owner&&<button className="secondary" disabled={busy||!consent}>{t("save")}</button>}
      </form>
      {(manage||snapshot.canRecover)&&<>
        <label className="group-target">{t("chooseMembers")}<select value={target?.id??""} disabled={busy} onChange={event=>{const chosen=eligible.find(user=>user.id===event.target.value);setTarget(chosen?structuredClone(chosen):null);setConsent(false)}}><option value="">{t("chooseMembers")}</option>{eligible.map(user=><option key={user.id} value={user.id}>{user.displayName||user.username}</option>)}</select></label>
        {target&&manage&&<div className="self-state-buttons">
          {target.role!=="owner"&&(owner||target.role!=="admin")&&<><button type="button" className="secondary" disabled={busy||!consent} onClick={()=>{const chosen=structuredClone(target);write((source,original)=>source.target(conversationId,original,"mute",chosen,{mutedUntil:new Date(Date.now()+3600000).toISOString()}))}}>{label("mute")} · {t("oneHour")}</button><button type="button" className="secondary" disabled={busy||!consent} onClick={()=>{const chosen=structuredClone(target);write((source,original)=>source.target(conversationId,original,"mute",chosen,{mutedUntil:""}))}}>{label("unmute")}</button></>}
          {owner&&<><button type="button" className="secondary" disabled={busy||!consent||target.role==="owner"} onClick={()=>{const chosen=structuredClone(target);write((source,original)=>source.target(conversationId,original,"role",chosen,{role:chosen.role==="admin"?"member":"admin"}))}}>{label(target.role==="admin"?"makeMember":"makeAdmin")}</button><button type="button" className="secondary" disabled={busy||!consent} onClick={()=>{const chosen=structuredClone(target);write((source,original)=>source.target(conversationId,original,"transfer",chosen))}}>{t("transferOwner")}</button></>}
        </div>}
        {snapshot.canRecover&&<form ref={recoverForm} className="password-change-form" onSubmit={recover} noValidate><label>{t("otp")}<input name="otp" inputMode="numeric" autoComplete="one-time-code" maxLength={6}/></label><button className="secondary" disabled={busy||!consent||!target}>{label("recover")}</button></form>}
      </>}
      {manage&&<>
        <h4>{label("invitations")}</h4>
        <form className="password-change-form" onSubmit={invitation} noValidate><label>{label("hours")}<input name="hours" type="number" min="1" max="720" defaultValue="24"/></label><label>{label("maxUses")}<input name="maxUses" type="number" min="1" max="10000" defaultValue="1"/></label><label className="password-change-consent"><input name="approval" type="checkbox" defaultChecked/>{label("approvalRequired")}</label><button className="secondary" disabled={busy||!consent}>{label("newInvite")}</button></form>
        {!invites.length?<div className="empty"><Users size={24}/><h4>{label("loadHint")}</h4><button className="secondary" disabled={busy} onClick={()=>void run(load)}>{t("reload")}</button></div>:invites.map(invite=><article className="self-state-panel" key={invite.id}><p className="self-state-hint">{invite.id}</p><p>{invite.uses}/{invite.maxUses} · {invite.expiresAt.replace("T"," ").replace("Z"," UTC")}</p><div className="self-state-buttons"><button type="button" className="secondary" disabled={busy||!!invite.revokedAt||Date.parse(invite.expiresAt)<=Date.now()||invite.uses>=invite.maxUses} onClick={()=>copy(invite)}>{label("copyInvite")}</button><button type="button" className="secondary" disabled={busy||!consent||!!invite.revokedAt} onClick={()=>write((source,original)=>source.revoke(conversationId,original,invite.id))}>{t("remove")}</button></div></article>)}
        {inviteCursor&&<button className="secondary" disabled={busy} onClick={()=>void run(async source=>{const result=await source.invites(conversationId,inviteCursor);if(source.context.isCurrent()){setInvites(old=>[...old,...result.items.filter(item=>!old.some(row=>row.id===item.id))]);setInviteCursor(result.nextCursor)}})}>{t("loadMore")}</button>}
        <h4>{label("applications")}</h4>
        {!applications.length?<div className="empty"><Users size={24}/><h4>{label("loadHint")}</h4><button className="secondary" disabled={busy} onClick={()=>void run(load)}>{t("reload")}</button></div>:applications.map(application=><article className="self-state-panel" key={application.id}><h4>{application.displayName||application.username||t(application.accountState==="tombstoned"?"tombstoned":"contentUnavailable")}</h4><p className="self-state-hint">{t(application.state==="pending"?"pendingReview":application.state==="approved"?"accepted":"rejected")} · {application.createdAt?.replace("T"," ").replace("Z"," UTC")}</p><div className="self-state-buttons">{application.state==="pending"&&<><button className="secondary" type="button" disabled={busy||!consent} onClick={()=>write((source,original)=>source.decide(conversationId,original,application.id,true))}>{t("accept")}</button><button className="secondary" type="button" disabled={busy||!consent} onClick={()=>write((source,original)=>source.decide(conversationId,original,application.id,false))}>{t("reject")}</button></>}</div></article>)}
        {applicationCursor&&<button className="secondary" disabled={busy} onClick={()=>void run(async source=>{const result=await source.applications(conversationId,applicationCursor);if(source.context.isCurrent()){setApplications(old=>[...old,...result.items.filter(item=>!old.some(row=>row.id===item.id))]);setApplicationCursor(result.nextCursor)}})}>{t("loadMore")}</button>}
      </>}
      {owner&&<button className="text-danger" type="button" disabled={busy||!consent} onClick={()=>write((source,original)=>source.mutate(conversationId,original,{action:"dissolve"}))}>{label("dissolve")}</button>}
    </>}
    <h4>{label("originals")}</h4>
    {!rows.length?<div className="empty"><Users size={24}/><h4>{label("loadHint")}</h4><button className="secondary" disabled={busy||!context} onClick={()=>void run(originals)}>{t("reload")}</button></div>:<form ref={retryForm} className="password-change-form" onSubmit={retry} noValidate>{rows.slice(0,shown).map(row=><article className="self-state-panel" key={row.requestId}><p className="self-state-hint">{row.requestId} · {label(row.state)}</p><div className="self-state-buttons"><button className="secondary" type="button" disabled={busy} onClick={()=>void run(async source=>{await source.resume(row.requestId,false)})}>{label("check")}</button>{row.state!=="applied"&&row.state!=="blocked"&&<button className="secondary" type="submit" value={row.requestId} disabled={busy||!consent}>{label("retry")}</button>}</div></article>)}{rows.length>shown&&<button className="secondary" type="button" onClick={()=>setShown(shown+20)}>{t("loadMore")}</button>}{rows.some(row=>JSON.parse(row.originalBody).action==="recover"&&row.state!=="applied")&&<label>{t("otp")}<input name="otp" inputMode="numeric" autoComplete="one-time-code" maxLength={6}/></label>}</form>}
    {failure&&<p role="alert">{failure==="failure"?t("failure"):label(failure)}</p>}
  </section>;
}
