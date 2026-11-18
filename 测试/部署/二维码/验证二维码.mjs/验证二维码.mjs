// Created by Jobs on 2026年11月18日，星期三.
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import os from 'node:os';
import {fileURLToPath} from 'node:url';
import {spawnSync} from 'node:child_process';
const root=path.resolve(path.dirname(fileURLToPath(import.meta.url)),'../../../..');
const go=process.env.JOBS_IM_GO;
if(!go)throw Error('Set JOBS_IM_GO to an existing Go executable');
const work=fs.mkdtempSync(path.join(os.tmpdir(),'JobsIM-QR-'));
function exec(file,args,env={}){const r=spawnSync(file,args,{encoding:'utf8',timeout:60000,env:{...process.env,...env}});if(r.error||r.status!==0)throw Error(`${file}: ${r.status}\n${r.stderr}`);return r.stdout;}
const binary=path.join(work,process.platform==='win32'?'deployqr.exe':'deployqr');
exec(go,['-C',path.join(root,'部署/公共组件/地址二维码'),'build','-mod=readonly','-trimpath','-o',binary,'.'],{GOPROXY:'off',GOTOOLCHAIN:'local'});
const out=path.join(work,'中文 空格二维码');const urls=['https://fixture.trycloudflare.com/','https://127.0.0.1:19443/'];
exec(binary,['--output-dir',out,'--title','JobsIM访问入口','--entry','临时公网|'+urls[0],'--entry','仅本机|'+urls[1]]);
assert(fs.existsSync(path.join(out,'index.html')));
if(process.platform==='darwin'){
 const text=exec('/usr/bin/osascript',['-l','JavaScript','-e',`ObjC.import('AppKit');ObjC.import('CoreImage');function run(argv){var d=$.CIDetector.detectorOfTypeContextOptions($.CIDetectorTypeQRCode,$.CIContext.contextWithOptions($.NSDictionary.dictionary),$({CIDetectorAccuracy:'CIDetectorAccuracyHigh'}));var a=[];for(var n of ['01.png','02.png']){var image=$.CIImage.imageWithContentsOfURL($.NSURL.fileURLWithPath(argv[0]+'/'+n));var r=d.featuresInImage(image);if(Number(r.count)!==1)throw Error('QR cannot decode');a.push(ObjC.unwrap(r.objectAtIndex(0).messageString));}return JSON.stringify(a);}`,out]);
 assert.deepEqual(JSON.parse(text),urls);
}
for(const url of ['https://user:password@example.com/','https://example.com/?token=secret']){
 const r=spawnSync(binary,['--output-dir',path.join(work,'bad'),'--entry','拒绝|'+url],{encoding:'utf8'});assert.notEqual(r.status,0);
}
console.log(JSON.stringify({build:'passed',output:'HTML/SVG/PNG',pngDecoded:process.platform==='darwin'?2:0,credentialURLsRejected:2,outputDirectory:out}));
