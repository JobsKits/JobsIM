#!/usr/bin/env python3
"""Owned isolated backend runner; no default instance, downloads or cache cleaning."""
import json,os,pathlib,signal,subprocess,sys,time
report=pathlib.Path(__file__).resolve().parent
meta=json.loads((report/'runtime.json').read_text())
root=next(p for p in report.parents if (p/'代码/后端/Go/go.mod').exists())
label,pattern=sys.argv[1:3]
assert label.replace('-','').isalnum()
env=os.environ.copy()
for key in list(env):
 if key.startswith('PG'):env.pop(key)
env.update(json.loads(pathlib.Path(meta['environmentFile']).read_text()))
scratch=pathlib.Path(meta['privateRealm'])/'Scratch';scratch.mkdir(mode=0o700,exist_ok=True);env['TMPDIR']=str(scratch)
go=str(pathlib.Path.home()/'Library/Application Support/JobsIM/Toolchains/go1.27.4/go/bin/go')
args=[go,'-C',str(root/'代码/后端/Go'),'test','-overlay',str(report/'overlay.json'),'-p','1','-parallel','1','-count','1','-timeout','12m','-json','-run',pattern,'.']
available=lambda:int(subprocess.check_output(['/bin/df','-k',str(root)],text=True).splitlines()[-1].split()[3])
start_free=available();assert start_free>=5*1024*1024,'5GiB pre-run gate'
started=time.time();minimum=start_free
with (report/(label+'.stdout.jsonl')).open('w') as out,(report/(label+'.stderr.log')).open('w') as err:
 child=subprocess.Popen(args,env=env,cwd=str(root),stdout=out,stderr=err,stdin=subprocess.DEVNULL,start_new_session=True)
 (report/'active-child.json').write_text(json.dumps({'label':label,'pid':child.pid,'started':started,'command':args},ensure_ascii=False,indent=2)+'\n')
 print(json.dumps({'started':label,'pid':child.pid}),flush=True)
 while child.poll() is None:
  minimum=min(minimum,available())
  if minimum<4*1024*1024:
   os.killpg(child.pid,signal.SIGTERM);child.wait(timeout=15);break
  time.sleep(2)
 code=child.wait()
top={'pass':[],'fail':[],'skip':[]};package=[]
for line in (report/(label+'.stdout.jsonl')).read_text().splitlines():
 try:value=json.loads(line)
 except ValueError:continue
 action=value.get('Action');name=value.get('Test')
 if action in top and name and '/' not in name:top[action].append(name)
 if action in ('pass','fail','skip') and not name:package.append(value)
result={'label':label,'exit':code,'elapsedSeconds':round(time.time()-started,3),'diskBeforeKiB':start_free,'diskAfterKiB':available(),'diskMinimumKiB':minimum,'topLevel':top,'packageResults':package,'stdout':label+'.stdout.jsonl','stderr':label+'.stderr.log','command':args}
(report/(label+'.result.json')).write_text(json.dumps(result,ensure_ascii=False,indent=2)+'\n')
print(json.dumps(result,ensure_ascii=False),flush=True)
sys.exit(code)
