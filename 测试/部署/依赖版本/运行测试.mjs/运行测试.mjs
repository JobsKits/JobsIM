// Created by Jobs on 2026年11月18日，星期三.
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { spawnSync } from 'node:child_process';
const here=path.resolve(path.dirname(fileURLToPath(import.meta.url)),'..');
const root=path.resolve(here,'../../..');
const moduleRoot=path.join(root,'部署/公共组件/依赖版本');
const go=process.env.JOBS_IM_GO;
if(!go||!fs.existsSync(go))throw Error('Set JOBS_IM_GO to an existing healthy Go executable');
const work=fs.mkdtempSync(path.join(os.tmpdir(),'JobsIM-DeployVersions-'));
const files=fs.readdirSync(here,{withFileTypes:true}).filter(x=>x.isFile()&&x.name.endsWith('_test.go')).map(x=>x.name).sort();
const overlay=path.join(work,'overlay.json');
fs.writeFileSync(overlay,JSON.stringify({Replace:Object.fromEntries(files.map(n=>[path.join(moduleRoot,n),path.join(here,n)]))}));
const r=spawnSync(go,['test','-overlay',overlay,'-count=1','-timeout=120s','-v','./...'],{cwd:moduleRoot,env:{...process.env,GOTOOLCHAIN:'local'},stdio:'inherit'});
if(r.error)throw r.error;
process.exitCode=r.status??2;
