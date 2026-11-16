#!/usr/bin/env python3
"""Formal personal ZIP lifecycle in one explicitly owned independent HTTPS realm."""

import argparse
import base64
import concurrent.futures
from datetime import datetime, timezone
import hashlib
import fcntl
import http.cookiejar
import importlib.util
import io
import json
from pathlib import Path
import secrets
import shutil
import subprocess
import signal
import socket
import ssl
import struct
import time
import urllib.request
import urllib.error
import urllib.parse
import uuid
import zipfile
import zlib


HERE = Path(__file__).resolve().parent
SPEC = importlib.util.spec_from_file_location("jobsim_personal_realm", HERE / "realm.py")
REALM = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(REALM)
SPEC = importlib.util.spec_from_file_location("jobsim_personal_forward_helpers", HERE.parent / "forward_refs.py/forward_refs.py")
HELPER = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(HELPER)


class PersonalExportSuite(HELPER.ForwardSuite):
    def __init__(self, environment, seed):
        if not environment.get("freshFormalExportRealm") or environment["serverURL"] != "https://127.0.0.1:58443":
            raise ValueError("Explicit fresh formal-export realm required")
        super().__init__(environment)
        self.seed = Path(seed)
        self.epoch = environment["restoreEpoch"]
        self.fixture_path = Path(environment["runDir"]) / "Secrets/personal-export-fixture.json"
        self.fixture = json.loads(self.fixture_path.read_text()) if self.fixture_path.exists() else {}
        self.passwords = self.fixture.get("passwords", {})
        self.transcript = []

    def save(self):
        self.fixture["passwords"] = self.passwords
        REALM.private_json(self.fixture_path, self.fixture)

    def refresh_expired_owned_actors(self):
        for label in ['owner','peer','outsider']:
            actor=self.fixture.get(label)
            if not actor:continue
            request=urllib.request.Request(self.base+'/api/me',headers={'Authorization':'Bearer '+actor['token']})
            try:
                with urllib.request.urlopen(request,context=self.trust,timeout=15) as response:response.read();self.checks+=1
                continue
            except urllib.error.HTTPError as error:
                body=json.load(error);self.checks+=1
                if error.code!=401 or body.get('error',{}).get('code')!='ACCESS_EXPIRED':raise AssertionError('owned actor was revoked, not an expiring access credential')
            original={'requestId':str(uuid.uuid4()),'familyId':actor['familyId'],'refreshGeneration':actor['refreshGeneration'],'refreshToken':actor['refreshToken']}
            self.fixture.setdefault('explicitExpiredRefreshes',[]).append({'label':label,'body':original});self.save()
            result=self.call('POST','/api/auth/refresh',original)
            self.check(result['familyId']==actor['familyId'] and int(result['refreshGeneration'])==int(actor['refreshGeneration'])+1,'real elapsed ACCESS_EXPIRED rotates its one current refresh generation without reauthentication')
            self.fixture[label]=result;self.save()

    def check(self, condition, description):
        super().check(condition, description)
        self.transcript.append({"assertion": description, "passed": True})

    def sql(self, query, journal=False):
        return REALM.sql(self.environment, journal, query)

    def prepare(self):
        if self.fixture.get("prepared"):
            return
        if "owner" not in self.fixture:
            owner, peer, outsider = [self.register(label) for label in ["exowner", "expeer", "exother"]]
            self.fixture.update(owner=owner, peer=peer, outsider=outsider)
            self.save()
        owner,peer=self.fixture["owner"],self.fixture["peer"]
        if "conversation" not in self.fixture:
            self.friend(owner, peer)
            self.fixture["conversation"] = self.call("POST", "/api/conversations", {"kind": "group", "title": "Formal personal export", "memberIds": [peer["user"]["id"]]}, owner)["id"]
            self.save()
        conversation=self.fixture["conversation"]
        self.call("PATCH", "/api/me", {"displayName": "Own exported profile"}, owner)
        if "source" not in self.fixture:
            self.fixture["source"] = self.send(peer, conversation, self.payload(peer, conversation, "ORDINARY_CURRENT_EXPORT_TEXT"))
            self.save()
        source=self.fixture["source"]
        if "forwarded" not in self.fixture:
            forwarded, _ = self.forward(owner, conversation, source)
            self.fixture["forwarded"] = forwarded
            self.save()
        if "attachment" not in self.fixture:
            data = b"JobsIM formal exact authenticated file bytes\n" * 97
            attachment = self.upload(peer, "../visible-original-name.txt", data)
            file_message = self.send(peer, conversation, self.payload(peer, conversation, "ordinary file", type="file", attachmentId=attachment["id"]))
            self.fixture.update(attachment=attachment, fileMessage=file_message, attachmentDigest=hashlib.sha256(data).hexdigest())
            self.save()
        preferences = self.call("GET", "/api/me/preferences", session=owner)
        if preferences["fontPreset"]!="large":
            self.call("PATCH", "/api/me/preferences", {"requestId": str(uuid.uuid4()), "revisionEpoch": self.epoch, "expectedVersion": preferences["version"], "fontPreset": "large"}, owner)
        view = self.call("GET", "/api/conversations/" + conversation + "/view", session=owner)
        if not view["manualUnread"]:
            self.call("PATCH", "/api/conversations/" + conversation + "/view", {"requestId": str(uuid.uuid4()), "revisionEpoch": self.epoch, "expectedVersion": view["version"], "changes": {"manualUnread": True,"expectedManualUnreadGeneration":view["manualUnreadGeneration"]}}, owner)
        metadata = self.conversation(owner, conversation)
        self.call("PUT", "/api/conversations/" + conversation + "/read", {"requestId": str(uuid.uuid4()), "revisionEpoch": self.epoch, "membershipGeneration": metadata["membershipGeneration"], "observedMessageIds": [source["id"]]}, owner)
        draft = self.call("GET", "/api/conversations/" + conversation + "/draft", session=owner)
        message = self.message(owner, source["id"])
        self.call("PUT", "/api/conversations/" + conversation + "/draft", {"requestId": str(uuid.uuid4()), "revisionEpoch": self.epoch, "expectedVersion": draft["version"], "draftAdmission": draft["draftAdmission"], "ownedText": "INDEPENDENT_OWNED_EXPORT_DRAFT", "reply": {"messageId": source["id"], "sourceVersion": message["contentVersion"], "replyAdmission": message["replyAdmission"]}}, owner)
        announcement = self.announce_body(owner, conversation, message)
        announcement["roleGeneration"] = metadata["roleGeneration"]
        self.call("PUT", "/api/conversations/" + conversation + "/announcement", announcement, owner)
        self.fixture["prepared"]=True
        self.save()

    def original(self):
        return {"requestId": str(uuid.uuid4()), "restoreEpoch": self.epoch}

    def create(self, actor, body, expect=202):
        return self.call("POST", "/api/me/data-exports", body, actor, expect)

    def result(self, actor, body, expect=200):
        return self.call("POST", "/api/me/data-export-results", body, actor, expect)

    def status(self, actor, job):
        return self.call("GET", "/api/me/data-exports/" + job, session=actor)

    def ready(self, actor, job):
        for _ in range(100):
            current = self.status(actor, job)
            state = current["job"]["state"]
            if state == "ready":
                return current
            if state not in ["requested", "building"]:
                raise AssertionError("Formal export terminal state " + state + "/" + current["job"].get("failureCode", "unknown"))
            time.sleep(0.15)
        raise AssertionError("Formal export did not finish within bounded wait")

    def download(self, actor, job):
        ticket = self.call("POST", "/api/me/data-exports/" + job + "/download-ticket", {}, actor)
        data, headers = self.binary(actor, "/api/me/data-exports/" + job + "/download", headers={"X-JobsIM-Download-Token": ticket["downloadToken"]})
        self.check(headers["Content-Type"] == "application/zip", "controlled gateway delivers usable ZIP, never a plaintext decryption key")
        return zipfile.ZipFile(io.BytesIO(data)), data

    def lost_create(self, actor, body):
        endpoint = urllib.parse.urlsplit(self.base)
        data = json.dumps(body).encode()
        with socket.create_connection((endpoint.hostname, endpoint.port), timeout=15) as connection:
            with self.trust.wrap_socket(connection, server_hostname=endpoint.hostname) as secure:
                header = (f"POST /api/me/data-exports HTTP/1.1\r\nHost: {endpoint.netloc}\r\nAuthorization: Bearer {actor['token']}\r\nContent-Type: application/json\r\nContent-Length: {len(data)}\r\nConnection: close\r\n\r\n").encode()
                secure.sendall(header + data)
                for _ in range(100):
                    count = self.sql("SELECT count(*) FROM governance.export_jobs WHERE user_id='" + actor["user"]["id"] + "' AND request_id='" + body["requestId"] + "'")
                    if count == "1":
                        return
                    time.sleep(0.02)
                raise AssertionError("Original export creation not observed; response remained unread")

    def named_task(self,label,actor):
        if label+"Body" not in self.fixture:
            self.fixture[label+"Body"]=self.original()
            self.save()
        body=self.fixture[label+"Body"]
        if label+"Job" not in self.fixture:
            result=self.create(actor,body)
            self.fixture[label+"Job"]=result['job']['id']
            self.save()
        return self.ready(actor,self.fixture[label+"Job"])

    def avatar(self,actor):
        def chunk(kind,data):return struct.pack('>I',len(data))+kind+data+struct.pack('>I',zlib.crc32(kind+data)&0xffffffff)
        image=b'\x89PNG\r\n\x1a\n'+chunk(b'IHDR',struct.pack('>IIBBBBB',16,16,8,2,0,0,0))+chunk(b'IDAT',zlib.compress((b'\0'+bytes([40,110,75])*16)*16))+chunk(b'IEND',b'')
        user=self.call('GET','/api/me',session=actor)
        boundary='JobsIMFormalAvatar'+secrets.token_hex(8)
        fields={'requestId':str(uuid.uuid4()),'expectedVersion':user['profileVersion']}
        raw=b''
        for key,value in fields.items():raw+=b'--'+boundary.encode()+b'\r\nContent-Disposition: form-data; name="'+key.encode()+b'"\r\n\r\n'+value.encode()+b'\r\n'
        raw+=b'--'+boundary.encode()+b'\r\nContent-Disposition: form-data; name="file"; filename="avatar.png"\r\nContent-Type: image/png\r\n\r\n'+image+b'\r\n--'+boundary.encode()+b'--\r\n'
        request=urllib.request.Request(self.base+'/api/me/avatar',data=raw,method='POST',headers={'Authorization':'Bearer '+actor['token'],'Content-Type':'multipart/form-data; boundary='+boundary})
        with urllib.request.urlopen(request,context=self.trust,timeout=25) as response:result=json.load(response);self.checks+=1
        self.check(result['status']=='applied','real static avatar source published before formal export')

    def source_drain(self):
        owner,peer=self.fixture['owner'],self.fixture['peer']
        if not self.fixture.get('sourceSetup'):
            if not self.call('GET','/api/me',session=owner).get('avatarURL'):self.avatar(owner)
            self.check(self.status(owner,self.fixture['basicJob'])['job']['cleanupState']=='cleared','profile source mutation returns only after its old personal export is cleared')
            if 'sourceGap' not in self.fixture:
                self.fixture['sourceGap']=self.send(peer,self.fixture['conversation'],self.payload(peer,self.fixture['conversation'],'UNOBSERVED_GAP_FOR_SPARSE_PROGRESS'));self.save()
            if 'sourceObserved' not in self.fixture:
                self.fixture['sourceObserved']=self.send(peer,self.fixture['conversation'],self.payload(peer,self.fixture['conversation'],'OBSERVED_AFTER_REAL_GAP'));self.save()
            observed=self.fixture['sourceObserved']
            membership=self.conversation(owner,self.fixture['conversation'])['membershipGeneration']
            self.call('PUT','/api/conversations/'+self.fixture['conversation']+'/read',{'requestId':str(uuid.uuid4()),'revisionEpoch':self.epoch,'membershipGeneration':membership,'observedMessageIds':[observed['id']]},owner)
            self.fixture['sourceSetup']=True;self.save()
        task=self.named_task('source',owner);job=task['job']['id']
        archive,_=self.download(owner,job)
        names=archive.namelist()
        self.check(sum(name.startswith('Profile/') and name.endswith('.jpg') for name in names)==2,'own real crop and thumbnail assets exported through typed profile holders')
        self.check(sum('/read_observation/' in name for name in names)>0,'a real non-contiguous own observation is included rather than invented from a read point')
        current=self.call('GET','/api/me/preferences',session=owner)
        patch=self.fixture.setdefault('sourcePreferenceBody',{'requestId':str(uuid.uuid4()),'revisionEpoch':self.epoch,'expectedVersion':current['version'],'fontPreset':'small'})
        self.save()
        lock_path=Path(self.environment['stateDir'])/'Exports/Locks'/(task['job']['artifactId']+'.lock')
        jar=http.cookiejar.CookieJar();opener=urllib.request.build_opener(urllib.request.HTTPSHandler(context=self.trust),urllib.request.HTTPCookieProcessor(jar))
        login=self.fixture.setdefault('bufferedLoginBody',{'requestId':str(uuid.uuid4()),'username':owner['user']['username'],'password':self.passwords[owner['user']['id']],'deviceName':'formal-cookie-drain'})
        self.save()
        def browser_login():
            request=urllib.request.Request(self.base+'/api/auth/login',data=json.dumps(login).encode(),headers={'Content-Type':'application/json','Origin':self.base,'X-JobsIM-Client':'web'})
            try:
                with opener.open(request,timeout=15) as response:status,raw,headers=response.status,response.read(),response.headers
            except urllib.error.HTTPError as error:status,raw,headers=error.code,error.read(),error.headers
            self.checks+=1
            return status,json.loads(raw),headers
        with lock_path.open('rb') as held:
            fcntl.flock(held.fileno(),fcntl.LOCK_SH)
            try:
                first=self.call('PATCH','/api/me/preferences',patch,owner,503)
                self.check(first['error']['code']=='EXPORT_CLEANUP_PENDING','committed source mutation does not claim success while an export reader has not drained')
                self.check(self.sql("SELECT state FROM governance.export_jobs WHERE id='"+job+"'")=='invalidated','download authorization revoked durably before artifact drain')
                self.call('PATCH','/api/me/preferences',patch,owner,503)
                self.check(self.sql("SELECT count(*) FROM directory.source_preference_operations WHERE user_id='"+owner['user']['id']+"' AND request_id='"+patch['requestId']+"'")=='1','same committed source nonce resumes cleanup without another source write')
                status,body,headers=browser_login()
                self.check(status==503 and headers.get('Set-Cookie') is None and not list(jar),'initial authentication success cookie and body are not partially flushed before export cleanup')
            finally:fcntl.flock(held.fileno(),fcntl.LOCK_UN)
        repeated=self.call('PATCH','/api/me/preferences',patch,owner)
        self.check(repeated['status']=='applied' and self.status(owner,job)['job']['cleanupState']=='cleared','original successful nonce finishes the pending handoff after the exact holder is released')
        status,body,headers=browser_login()
        self.check(status==200 and headers.get('Set-Cookie') and body.get('familyId') and 'token' not in body,'same unknown initial authentication intent delivers its one original BFF family after cleanup')
        family=body['familyId']
        def browser_call(method,path,body=None,csrf=None):
            headers={'Content-Type':'application/json','X-JobsIM-Auth-Family':family,'Origin':self.base}
            if csrf:headers['X-CSRF-Token']=csrf
            request=urllib.request.Request(self.base+path,method=method,data=None if body is None else json.dumps(body).encode(),headers=headers)
            with opener.open(request,timeout=15) as response:result=json.load(response);self.checks+=1;return result
        csrf=browser_call('GET','/api/auth/csrf')['csrfToken'];browser_call('POST','/api/auth/confirm',{},csrf);browser_call('POST','/api/auth/logout',{},csrf)
        self.call('POST','/api/me/data-exports/'+job+'/download-ticket',{},owner,409)

    def streams(self):
        owner,peer=self.fixture['owner'],self.fixture['peer']
        if not self.fixture.get('mediaSamples'):
            work=Path(self.environment['runDir'])/'Run/MediaSamples';work.mkdir(exist_ok=True)
            ffmpeg=shutil.which('ffmpeg')
            if not ffmpeg:raise AssertionError('existing ffmpeg required; no tool download attempted')
            fixtures=[('image.png',['-f','lavfi','-i','color=c=green:s=16x16:d=0.1','-frames:v','1'],'image'),('animated.gif',['-f','lavfi','-i','testsrc=size=16x16:duration=0.3:rate=10'],'image'),('voice.m4a',['-f','lavfi','-i','sine=frequency=440:duration=0.2','-c:a','aac'],'audio'),('clip.mp4',['-f','lavfi','-i','color=c=green:s=32x32:d=0.2','-c:v','libx264','-pix_fmt','yuv420p'],'video')]
            samples=[]
            for name,arguments,kind in fixtures:
                subprocess.run([ffmpeg,'-hide_banner','-loglevel','error','-y',*arguments,str(work/name)],check=True,capture_output=True,timeout=30)
                data=(work/name).read_bytes();asset=self.upload(peer,name,data)
                message=self.send(peer,self.fixture['conversation'],self.payload(peer,self.fixture['conversation'],name,type=kind,attachmentId=asset['id']))
                samples.append({'asset':asset,'message':message,'digest':hashlib.sha256(data).hexdigest(),'kind':kind})
                self.fixture['mediaSamplesPartial']=samples;self.save()
            self.fixture['mediaSamples']=samples;self.save()
        if 'largeMedia' not in self.fixture:
            data=secrets.token_bytes(16<<20)
            asset=self.upload(peer,'controlled-stream.bin',data)
            message=self.send(peer,self.fixture['conversation'],self.payload(peer,self.fixture['conversation'],'ordinary complete stream asset',type='file',attachmentId=asset['id']))
            self.fixture.update(largeMedia=asset,largeMessage=message,largeDigest=hashlib.sha256(data).hexdigest());self.save()
        source_task=self.named_task('streamSource',owner)
        cancel_task=self.named_task('streamCancel',peer)
        archive,_=self.download(owner,source_task['job']['id'])
        for sample in self.fixture['mediaSamples']:
            content=archive.read('Media/'+sample['asset']['id'])
            message=json.loads(archive.read('Messages/'+self.fixture['conversation']+'/'+sample['message']['id']+'.json'))
            self.check(hashlib.sha256(content).hexdigest()==sample['digest'] and message['type']==sample['kind'],'real PNG/GIF/AAC-M4A/MP4 export retains original type and exact attachment bytes')
        def open_stream(actor,task):
            ticket=self.call('POST','/api/me/data-exports/'+task['job']['id']+'/download-ticket',{},actor)
            request=urllib.request.Request(self.base+'/api/me/data-exports/'+task['job']['id']+'/download',headers={'Authorization':'Bearer '+actor['token'],'X-JobsIM-Download-Token':ticket['downloadToken']})
            response=urllib.request.urlopen(request,context=self.trust,timeout=40)
            self.checks+=1
            self.check(response.read(1024).startswith(b'PK'),'real stream starts with authenticated plaintext ZIP only after complete encrypted validation')
            return response
        response=open_stream(owner,source_task)
        try:
            with concurrent.futures.ThreadPoolExecutor(max_workers=1) as pool:
                future=pool.submit(self.call,'PATCH','/api/me',{'displayName':'After real source stream drain'},owner)
                time.sleep(.2)
                self.check(not future.done() and self.sql("SELECT state FROM governance.export_jobs WHERE id='"+source_task['job']['id']+"'")=='ready','real download SH holds the source EX mutation outside its commit')
                response.close()
                future.result(timeout=15)
        finally:response.close()
        self.check(self.status(owner,source_task['job']['id'])['job']['cleanupState']=='cleared','source EX mutation completes only after real TLS client disconnect and artifact drain')
        response=open_stream(peer,cancel_task)
        cancellation=self.fixture.setdefault('streamCancelOperationBody',{'requestId':str(uuid.uuid4())});self.save()
        try:
            with concurrent.futures.ThreadPoolExecutor(max_workers=1) as pool:
                future=pool.submit(self.call,'POST','/api/me/data-exports/'+cancel_task['job']['id']+'/cancel',cancellation,peer)
                for _ in range(100):
                    if self.sql("SELECT state FROM governance.export_jobs WHERE id='"+cancel_task['job']['id']+"'")=='cancelled':break
                    time.sleep(.02)
                else:raise AssertionError('cancel did not persist delivery denial')
                response.close()
                result=future.result(timeout=15)
                self.check(result['job']['cleanupState']=='cleared','cancel during a real authenticated download revokes, drains and clears its immutable prefix')
        finally:response.close()
        self.call('POST','/api/me/data-exports/'+cancel_task['job']['id']+'/download-ticket',{},peer,409)
        self.check(self.sql("SELECT key_ciphertext IS NULL FROM governance.export_jobs WHERE id='"+cancel_task['job']['id']+"'")=='t','cancelled task has no sealed delivery key')

    def open_zip_stream(self,actor,task):
        ticket=self.call('POST','/api/me/data-exports/'+task['job']['id']+'/download-ticket',{},actor)
        request=urllib.request.Request(self.base+'/api/me/data-exports/'+task['job']['id']+'/download',headers={'Authorization':'Bearer '+actor['token'],'X-JobsIM-Download-Token':ticket['downloadToken']})
        response=urllib.request.urlopen(request,context=self.trust,timeout=40)
        self.checks+=1
        self.check(response.read(1024).startswith(b'PK'),'real authenticated complete ZIP begins only after the complete encrypted verification pass')
        return response

    def stream_cancel(self):
        actor=self.fixture['peer'];job=self.fixture['streamCancelJob'];task=self.ready(actor,job)
        cancellation=self.fixture.setdefault('streamCancelOperationBody',{'requestId':str(uuid.uuid4())});self.save()
        response=self.open_zip_stream(actor,task)
        try:
            with concurrent.futures.ThreadPoolExecutor(max_workers=1) as pool:
                future=pool.submit(self.call,'POST','/api/me/data-exports/'+job+'/cancel',cancellation,actor)
                try:
                    for _ in range(150):
                        if future.done():future.result()
                        if self.sql("SELECT state FROM governance.export_jobs WHERE id='"+job+"'")=='cancelled':break
                        time.sleep(.02)
                    else:raise AssertionError('real cancellation did not commit delivery denial')
                finally:response.close()
                result=future.result(timeout=15)
                self.check(result['job']['cleanupState']=='cleared','real in-progress download cancellation revokes and drains before acknowledging cleanup')
        finally:response.close()
        self.call('POST','/api/me/data-exports/'+job+'/download-ticket',{},actor,409)
        self.check(self.sql("SELECT key_ciphertext IS NULL FROM governance.export_jobs WHERE id='"+job+"'")=='t','real cancelled stream retains no sealed delivery key')

    def source_stream(self):
        actor=self.fixture['peer'];task=self.named_task('sourceStreamRepeat',actor);job=task['job']['id']
        response=self.open_zip_stream(actor,task)
        try:
            with concurrent.futures.ThreadPoolExecutor(max_workers=1) as pool:
                future=pool.submit(self.call,'PATCH','/api/me',{'displayName':'After strict real source stream'},actor)
                try:
                    time.sleep(.2)
                    self.check(not future.done() and self.sql("SELECT state FROM governance.export_jobs WHERE id='"+job+"'")=='ready','active controlled TLS download holds source EX mutation before source commit')
                finally:response.close()
                future.result(timeout=15)
        finally:response.close()
        self.check(self.status(actor,job)['job']['cleanupState']=='cleared','source mutation flushes success only after real TLS disconnect and complete source/artifact handoff')

    def source_finalize(self):
        owner=self.fixture['owner'];job=self.fixture['sourceJob']
        patch=self.fixture['sourcePreferenceBody']
        self.call('PATCH','/api/me/preferences',patch,owner)
        self.check(self.status(owner,job)['job']['cleanupState']=='cleared','original committed source nonce waits for the complete governance handoff, including concurrent cancellation facts')
        self.check(self.sql("SELECT count(*) FROM directory.source_preference_operations WHERE user_id='"+owner['user']['id']+"' AND request_id='"+patch['requestId']+"'")=='1','source continuation did not rewrite its committed CAS fact')
        jar=http.cookiejar.CookieJar();opener=urllib.request.build_opener(urllib.request.HTTPSHandler(context=self.trust),urllib.request.HTTPCookieProcessor(jar))
        login=self.fixture['bufferedLoginBody']
        def authenticate(path,body):
            request=urllib.request.Request(self.base+path,data=json.dumps(body).encode(),headers={'Content-Type':'application/json','Origin':self.base,'X-JobsIM-Client':'web'})
            try:
                with opener.open(request,timeout=20) as response:status,raw,headers=response.status,response.read(),response.headers
            except urllib.error.HTTPError as error:status,raw,headers=error.code,error.read(),error.headers
            self.checks+=1;return status,json.loads(raw),headers
        status,body,headers=authenticate('/api/auth/login',login)
        if status==401 and body.get('error',{}).get('code')=='DELIVERY_EXPIRED':
            original=self.fixture.setdefault('bufferedRecoveryBody',dict(login,requestId=str(uuid.uuid4()),previousRequestId=login['requestId'],previousRequestIds=[login['requestId']]))
            self.save();status,body,headers=authenticate('/api/auth/reauthenticate',original)
            self.check(self.sql("SELECT bool_and(s.revoked_at IS NOT NULL) FROM auth.local_auth_requests a JOIN auth.sessions s ON s.id=a.session_id WHERE a.request_id='"+login['requestId']+"'")=='t','expired unknown initial delivery retired its exact previous family through new verified authentication')
        self.check(status==200 and headers.get('Set-Cookie') and body.get('familyId') and not any(key in body for key in ['token','refreshToken']),'controlled BFF recovery publishes only its own cookie after the prior cleanup barrier')
        family=body['familyId']
        def call(method,path,body=None,csrf=None):
            headers={'Content-Type':'application/json','X-JobsIM-Auth-Family':family,'Origin':self.base}
            if csrf:headers['X-CSRF-Token']=csrf
            request=urllib.request.Request(self.base+path,method=method,data=None if body is None else json.dumps(body).encode(),headers=headers)
            with opener.open(request,timeout=15) as response:value=json.load(response);self.checks+=1;return value
        csrf=call('GET','/api/auth/csrf')['csrfToken'];call('POST','/api/auth/confirm',{},csrf);call('POST','/api/auth/logout',{},csrf)

    def ledger(self):
        actor=self.fixture['outsider']
        if 'ledgerConversation' not in self.fixture:
            self.fixture['ledgerConversation']=self.call('POST','/api/conversations',{'kind':'self','memberIds':[]},actor)['id'];self.save()
        label='ledger'
        if self.fixture.get('ledgerJob') and self.status(actor,self.fixture['ledgerJob'])['job']['state']=='failed':
            prior=self.result(actor,self.fixture['ledgerBody'])
            self.check(prior['job']['state']=='failed','original failed task remains terminal; retry requires a new explicit task intent')
            label='ledgerRetry'
        task=self.named_task(label,actor);job=task['job']['id']
        time.sleep(2.1)
        self.check(self.status(actor,job)['job']['state']=='ready','cleanup of earlier terminal tasks never sweeps a newer current task for the same owner')
        current=self.call('GET','/api/me/preferences',session=actor)
        patch=self.fixture.setdefault('ledgerPreferenceBody',{'requestId':str(uuid.uuid4()),'revisionEpoch':self.epoch,'expectedVersion':current['version'],'fontPreset':'small'});self.save()
        unique=uuid.uuid4().hex[:12];name='formal_ack_gate_'+unique;key=secrets.randbelow(1<<30)+1
        self.sql("CREATE FUNCTION governance."+name+"() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$BEGIN IF NEW.job_id='"+job+"'::uuid AND NEW.state='cleared' THEN PERFORM pg_advisory_xact_lock("+str(key)+"); END IF; RETURN NEW; END$$; CREATE TRIGGER "+name+" BEFORE UPDATE ON governance.export_cancellations FOR EACH ROW EXECUTE FUNCTION governance."+name+"();")
        parsed=urllib.parse.urlparse(self.environment['businessURL']);env=REALM.clean_environment();application='formal-ack-'+unique
        env.update({'PGHOST':parsed.hostname,'PGPORT':str(parsed.port),'PGUSER':parsed.username,'PGDATABASE':parsed.path[1:],'PGPASSFILE':self.environment['pgPassFile'],'PGAPPNAME':application,'PGOPTIONS':'-c statement_timeout=45000'})
        holder=subprocess.Popen([str(Path(self.environment['pgBin'])/'psql'),'-XqAt','-v','ON_ERROR_STOP=1'],stdin=subprocess.PIPE,stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL,env=env,text=True)
        holder.stdin.write('BEGIN; SELECT pg_advisory_xact_lock('+str(key)+'); SELECT pg_sleep(40); ROLLBACK;\n');holder.stdin.flush()
        try:
            for _ in range(100):
                count=self.sql("SELECT count(*) FROM pg_stat_activity a JOIN pg_locks l ON l.pid=a.pid WHERE a.application_name='"+application+"' AND l.locktype='advisory' AND l.granted")
                if count=='1':break
                time.sleep(.02)
            else:raise AssertionError('owned final-ack scheduling gate not held')
            self.call('PATCH','/api/me/preferences',patch,actor,503)
            cleared=self.sql("SELECT count(*) FROM directory.personal_export_holders WHERE job_id='"+job+"' AND state<>'cleared'")
            pending=self.sql("SELECT count(*) FROM directory.personal_export_cleanups WHERE job_id='"+job+"' AND state='pending'")
            self.check(cleared=='0' and pending=='1','source cleanup responsibility survives after holder byte-clear while governance final acknowledgment is blocked')
            self.call('PATCH','/api/me/preferences',patch,actor,503)
            self.check(self.sql("SELECT count(*) FROM directory.source_preference_operations WHERE user_id='"+actor['user']['id']+"' AND request_id='"+patch['requestId']+"'")=='1','already committed original nonce does not bypass the persistent cleanup ledger')
        finally:
            self.sql("SELECT pg_cancel_backend(pid) FROM pg_stat_activity WHERE application_name='"+application+"' AND usename='"+parsed.username+"' AND datname='"+parsed.path[1:]+"'")
            holder.wait(timeout=8)
            self.sql('DROP TRIGGER '+name+' ON governance.export_cancellations; DROP FUNCTION governance.'+name+'();')
        self.call('PATCH','/api/me/preferences',patch,actor)
        self.check(self.status(actor,job)['job']['cleanupState']=='cleared' and self.sql("SELECT state FROM directory.personal_export_cleanups WHERE job_id='"+job+"'")=='cleared','original source intent succeeds only after the exact durable final-ack gate is released')

    def basic(self):
        self.prepare()
        actor, outsider = self.fixture["owner"], self.fixture["outsider"]
        body = self.fixture.setdefault("basicBody",self.original())
        self.save()
        if "basicJob" not in self.fixture:
            before = self.sql("SELECT count(*) FROM governance.export_jobs")
            self.result(actor, body, 404)
            self.check(self.sql("SELECT count(*) FROM governance.export_jobs") == before, "unknown full-body lookup creates neither job nor key")
            self.lost_create(actor, body)
        original = self.result(actor, body)
        job = original["job"]["id"]
        self.fixture["basicJob"] = job
        self.save()
        self.check(self.create(actor, body, 200)["job"]["id"] == job, "unread create response reuses the same nonce, task and sealed key")
        self.result(actor, dict(body, restoreEpoch="2"), 409)
        self.call("GET", "/api/me/data-exports/" + job, session=outsider, expect=404)
        ready = self.ready(actor, job)
        self.check(ready["format"] == "zip" and "decryptionKey" not in json.dumps(ready), "task metadata never delivers a ZIP key")
        self.check(self.sql("SELECT key_ciphertext IS NOT NULL FROM governance.export_jobs WHERE id='" + job + "'") == "t", "ready artifact keeps only an instance-sealed server key until cleanup")
        refresh = {"requestId": str(uuid.uuid4()), "familyId": actor["familyId"], "refreshGeneration": actor["refreshGeneration"], "refreshToken": actor["refreshToken"]}
        self.fixture["refreshOriginal"] = refresh
        self.save()
        actor = self.call("POST", "/api/auth/refresh", refresh)
        self.fixture["owner"] = actor
        self.save()
        self.check(self.status(actor, job)["job"]["state"] == "ready", "ordinary access/refresh rotation does not invalidate stable public device metadata")
        self.call("PATCH", "/api/conversations/" + self.fixture["conversation"], {"title": "outsider cannot change this"}, outsider, 403)
        self.check(self.status(actor, job)["job"]["state"] == "ready", "unauthorized metadata mutation does not revoke another owner's package")
        archive, raw = self.download(actor, job)
        self.check(archive.testzip() is None, "all downloaded ZIP entries pass their complete CRC")
        names = archive.namelist()
        self.check(all(not name.startswith("/") and ".." not in Path(name).parts for name in names), "original file name never becomes an unsafe ZIP path")
        values = {name: json.loads(archive.read(name)) for name in names if name.endswith(".json")}
        self.check(any("profile" in name and value.get("displayName") == "Own exported profile" for name, value in values.items()), "own current profile exported")
        self.check(any("preferences" in name and value.get("fontPreset") == "large" for name, value in values.items()), "own source font preference exported")
        self.check(any("conversation_view" in name and value.get("manualUnread") for name, value in values.items()), "own independent manual unread view exported")
        observations=int(self.sql("SELECT count(*) FROM sync.read_observations WHERE user_id='"+actor['user']['id']+"'"))
        self.check(any("read_state" in name for name in values) and sum("read_observation" in name for name in values)==observations, "own read point and complete currently stored sparse observations exported; contiguous observations may be compacted")
        self.check(any("draft" in name and value.get("ownedText") == "INDEPENDENT_OWNED_EXPORT_DRAFT" and value.get("reply", {}).get("messageId") == self.fixture["source"]["id"] for name, value in values.items()), "independent applied draft text and pure reply reference exported")
        forwarded = values["Messages/" + self.fixture["conversation"] + "/" + self.fixture["forwarded"]["id"] + ".json"]
        self.check(forwarded.get("exportState") == "forward_placeholder" and not any(key in forwarded for key in ["text", "attachmentId", "attachmentURL", "forwardOfId", "forwardAdmission"]), "forward target becomes a bodyless placeholder without source grant")
        message = values["Messages/" + self.fixture["conversation"] + "/" + self.fixture["source"]["id"] + ".json"]
        self.check(message["text"] == "ORDINARY_CURRENT_EXPORT_TEXT", "currently authorized ordinary message body exported")
        media = archive.read("Media/" + self.fixture["attachment"]["id"])
        self.check(hashlib.sha256(media).hexdigest() == self.fixture["attachmentDigest"], "ordinary attachment download retains exact authenticated bytes")
        encoded = json.dumps(values)
        self.check(not any(key in encoded for key in ['"forwardAdmission"', '"replyAdmission"', '"draftAdmission"', '"sendAdmission"', '"refreshToken"', '"password_hash"', '"intent_ciphertext"']), "JSON and manifests contain no reusable proof, credential or original recovery store")
        self.check(self.sql("SELECT purpose FROM governance.export_jobs WHERE id='" + job + "'") == "personal_data", "formal export task is explicitly distinct from legacy experimental chat export")
        self.fixture["downloadSHA256"] = hashlib.sha256(raw).hexdigest()
        self.save()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--environment-file", type=Path, required=True)
    parser.add_argument("--group", choices=["basic","source_drain","source_finalize","ledger","streams","stream_cancel","source_stream"], required=True)
    parser.add_argument("--report", type=Path, required=True)
    arguments = parser.parse_args()
    environment = json.loads(arguments.environment_file.read_text())
    if shutil.disk_usage(environment["runDir"]).free < REALM.MINIMUM_FREE:
        raise SystemExit("4GiB disk guard")
    suite = PersonalExportSuite(environment, arguments.environment_file)
    failure = None
    started = time.monotonic()
    try:
        suite.refresh_expired_owned_actors()
        getattr(suite, arguments.group)()
    except Exception as error:
        failure = str(error)
    finally:
        suite.save()
    result = {"group": arguments.group, "status": "passed" if failure is None else "failed", "failure": failure,
              "httpChecks": suite.checks, "semanticAssertions": suite.assertions,
              "elapsedSeconds": round(time.monotonic() - started, 3), "binarySHA256": environment["binarySHA256"],
              "scriptSHA256": hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
              "checkedAt": datetime.now(timezone.utc).isoformat(), "assertions": suite.transcript}
    arguments.report.write_text(json.dumps(result, ensure_ascii=False, indent=2) + "\n")
    print(json.dumps({key: value for key, value in result.items() if key != "assertions"}, ensure_ascii=False))
    if failure:
        raise SystemExit(1)


if __name__ == "__main__":
    main()
