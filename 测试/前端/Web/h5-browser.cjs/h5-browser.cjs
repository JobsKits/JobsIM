// Created by Jobs on 2026年11月18日，星期三.
const fs=require('node:fs'),path=require('node:path'),os=require('node:os'),assert=require('node:assert/strict');
const {pathToFileURL}=require('node:url');
const {chromium}=require(process.env.JOBSIM_PLAYWRIGHT_MODULE||'playwright');
const root=path.resolve(__dirname,'../../../..'),web=path.join(root,'代码/前端/Web');
const out=process.env.JOBSIM_H5_OUTPUT||path.join(root,'项目相关报告/测试/前端/Web/20261118-H5适配',String(Date.now()));
fs.mkdirSync(out,{recursive:true});
const results=[],errors=[];
async function check(name,fn){try{await fn();results.push({name,status:'passed'});}catch(e){results.push({name,status:'failed',error:e.message});}}
(async()=>{
 const {createServer}=await import(pathToFileURL(path.join(web,'node_modules/vite/dist/node/index.js')).href);
 const vite=await createServer({root:web,configFile:false,cacheDir:fs.mkdtempSync(path.join(os.tmpdir(),'JobsIM-H5-Vite-')),server:{host:'127.0.0.1',port:0}});
 await vite.listen();const url='http://127.0.0.1:'+vite.httpServer.address().port;
 const browser=await chromium.launch({headless:true,executablePath:process.env.BROWSER_EXECUTABLE||'/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',args:['--use-fake-ui-for-media-stream','--use-fake-device-for-media-stream']});
 try{
  const context=await browser.newContext({viewport:{width:390,height:844},isMobile:true,hasTouch:true,deviceScaleFactor:2});
  await context.addInitScript(()=>{const vv=new EventTarget();Object.assign(vv,{height:innerHeight,width:innerWidth,scale:1,offsetTop:0,offsetLeft:0});Object.defineProperty(window,'visualViewport',{configurable:true,value:vv});window.__h5Viewport=(height,top=0,scale=1)=>{Object.assign(vv,{height,width:innerWidth,offsetTop:top,scale});vv.dispatchEvent(new Event('resize'));};});
  const page=await context.newPage();page.setDefaultTimeout(5000);page.on('pageerror',e=>errors.push(e.message));await page.route('**/api/**',r=>r.abort());
  await page.goto(url);await page.locator('.conversation-row').first().waitFor();await page.evaluate(()=>window.__h5Viewport(innerHeight));await page.waitForTimeout(50);
  await check('mobile conversation touch navigation',async()=>{await page.locator('.conversation-row').first().tap();await page.locator('.composer textarea').waitFor({state:'visible'});});
  await check('touch send remains usable',async()=>{await page.locator('.composer textarea').fill('H5 isolated demo');await page.locator('.send-button').tap();await page.locator('.thread-body').getByText('H5 isolated demo',{exact:true}).waitFor();});
  await check('keyboard visible viewport keeps composer inside',async()=>{await page.locator('.composer textarea').focus();await page.evaluate(()=>window.__h5Viewport(300));await page.waitForTimeout(80);const box=await page.locator('.composer').boundingBox();assert(box&&box.y>=0&&box.y+box.height<=301,JSON.stringify(box));const history=await page.locator('.thread-body').boundingBox();assert(history&&history.height>=32,'keyboard must retain readable message area');});
  await page.evaluate(()=>window.__h5Viewport(innerHeight));await page.waitForTimeout(30);
  await check('keyboard viewport offset keeps app visible',async()=>{await page.locator('.composer textarea').focus();await page.evaluate(()=>window.__h5Viewport(340,80));await page.waitForTimeout(80);const box=await page.locator('.app').boundingBox();assert(box&&box.y>=79&&box.y+box.height<=421,JSON.stringify(box));});
  await page.evaluate(()=>window.__h5Viewport(innerHeight));await page.waitForTimeout(30);
  await check('pinch zoom retains document layout',async()=>{const before=await page.locator('.app').boundingBox();await page.evaluate(()=>window.__h5Viewport(420,0,2));await page.waitForTimeout(50);const after=await page.locator('.app').boundingBox();assert.equal(after.height,before.height);assert(!await page.locator('meta[name=viewport]').getAttribute('content').then(x=>/user-scalable=no|maximum-scale=1/.test(x)));});
  await check('pinched visual viewport excludes an offscreen message still inside the layout container',async()=>{
   const result=await page.evaluate(async()=>{const {isRectInVisibleViewport}=await import('/Src/useVisibleViewport.ts');window.__h5Viewport(230,0,2);const container=new DOMRect(0,0,390,844),message=new DOMRect(20,300,100,44);return {legacy:message.bottom<=Math.min(innerHeight,container.bottom),current:isRectInVisibleViewport(message,container)};});assert(result.legacy&&!result.current);
  });
  await check('visible read bounds also exclude shifted viewport top and horizontal clipping',async()=>{
   assert(await page.evaluate(async()=>{const {isRectInVisibleViewport}=await import('/Src/useVisibleViewport.ts');window.__h5Viewport(230,80,2);visualViewport.offsetLeft=30;visualViewport.width=300;const box=new DOMRect(0,0,390,844);const result=!isRectInVisibleViewport(new DOMRect(50,40,100,30),box)&&!isRectInVisibleViewport(new DOMRect(5,100,100,30),box)&&isRectInVisibleViewport(new DOMRect(50,100,100,30),box);visualViewport.offsetLeft=0;visualViewport.width=innerWidth;return result;}));
  });
  await check('coupled layout and visual viewport shrink at 230px preserves reading and sending',async()=>{
   await page.setViewportSize({width:390,height:230});await page.locator('.composer textarea').focus();await page.evaluate(()=>window.__h5Viewport(innerHeight));await page.waitForTimeout(80);
   assert.equal(await page.evaluate(()=>innerHeight),230);assert.equal(await page.locator('html').getAttribute('data-keyboard'),'open');
   const history=await page.locator('.thread-body').boundingBox(),send=await page.locator('.send-button').boundingBox();assert(history&&history.height>=32,'230px reading area');assert(send&&send.y>=0&&send.y+send.height<=231,'230px send control');
   assert(await page.locator('.composer-bottom > span').isVisible(),'local demo indicator');await page.screenshot({path:path.join(out,'keyboard-390x230.png')});
  });
  await check('230px scrolling exposes a complete unobscured message text line',async()=>{
   const result=await page.evaluate(async()=>{
    const box=document.querySelector('.thread-body'),text=[...box.querySelectorAll('.bubble p')].find(p=>p.textContent==='H5 isolated demo');
    const range=document.createRange();range.selectNodeContents(text);let line=range.getClientRects()[0],bounds=box.getBoundingClientRect();
    box.scrollTop+=line.top+line.height/2-(bounds.top+bounds.height/2);await new Promise(resolve=>requestAnimationFrame(resolve));
    line=range.getClientRects()[0];bounds=box.getBoundingClientRect();const hit=document.elementFromPoint(line.left+line.width/2,line.top+line.height/2);
    return {line:{top:line.top,bottom:line.bottom},bounds:{top:bounds.top,bottom:bounds.bottom},unobscured:hit===text||text.contains(hit),hit:hit?.className};
   });assert(result.line.top>=result.bounds.top&&result.line.bottom<=result.bounds.bottom&&result.unobscured,JSON.stringify(result));
   await page.screenshot({path:path.join(out,'keyboard-390x230-message-line.png')});
  });
  await page.setViewportSize({width:390,height:844});await page.evaluate(()=>window.__h5Viewport(innerHeight));
  await check('touch attachment opens the browser file chooser',async()=>{const [chooser]=await Promise.all([page.waitForEvent('filechooser'),page.locator('.composer-tools .icon-button').first().tap()]);await chooser.setFiles([]);});
  await check('touch recording with synthetic microphone retains preview and cancellation',async()=>{
   await page.locator('.audio-recorder > button').first().tap();await page.locator('.audio-recorder button').filter({hasText:'停止并试听'}).waitFor();await page.waitForTimeout(1200);
   await page.locator('.audio-recorder button').filter({hasText:'停止并试听'}).tap();await page.locator('.audio-recording-preview audio').waitFor();
   await page.setViewportSize({width:390,height:300});await page.evaluate(()=>window.__h5Viewport(innerHeight));await page.waitForTimeout(60);assert((await page.locator('.thread-body').boundingBox()).height>=32);
   await page.locator('.audio-recording-preview button').filter({hasText:'取消录音'}).scrollIntoViewIfNeeded();await page.locator('.audio-recording-preview button').filter({hasText:'取消录音'}).tap();await page.locator('.audio-recording-preview').waitFor({state:'hidden'});
  });
  await page.evaluate(()=>window.__h5Viewport(innerHeight));
  for(const [width,height]of [[320,568],[390,844],[430,932],[844,390]]){
   await page.setViewportSize({width,height});await page.evaluate(()=>window.__h5Viewport(innerHeight));await page.waitForTimeout(60);
   await check(`layout ${width}x${height}`,async()=>{assert(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth));assert(!await page.locator('.sidebar').isVisible(),'phone conversation must retain single-pane navigation');const b=await page.locator('.send-button').boundingBox();assert(b&&b.x>=0&&b.x+b.width<=width+1&&b.y+b.height<=height+1,JSON.stringify(b));const history=await page.locator('.thread-body').boundingBox();assert(history&&history.height>=48,'conversation history must remain visible');});
   await page.screenshot({path:path.join(out,`chat-${width}x${height}.png`)});
  }
  await check('CtrlK follows the visible sidebar on coarse-pointer landscape',async()=>{await page.keyboard.press('Control+k');await page.locator('.modal').waitFor();await page.keyboard.press('Escape');});
  await check('touch return to conversation list',async()=>{await page.locator('.thread-header button[aria-label="返回"]').tap();await page.locator('.conversation-row').first().waitFor({state:'visible'});});
  await check('mobile authentication modal fits visible viewport',async()=>{await page.locator('.me-avatar').tap();await page.locator('.modal').waitFor();await page.locator('.modal input').first().focus();await page.evaluate(()=>window.__h5Viewport(320));await page.waitForTimeout(60);const b=await page.locator('.modal').boundingBox();assert(b&&b.y>=0&&b.y+b.height<=321,JSON.stringify(b));});
  await page.evaluate(()=>window.__h5Viewport(innerHeight));await page.keyboard.press('Escape');
  if(await page.locator('.modal').count())await page.locator('.modal .modal-close, .modal-overlay > .modal button[aria-label]').first().click();
  await page.locator('.rail-bottom button[aria-label="设置"]').tap();await page.locator('.settings-content').waitFor();
  await page.setViewportSize({width:320,height:568});await page.evaluate(()=>window.__h5Viewport(innerHeight));
  const selects=page.locator('.settings-section select');
  for(const font of ['small','normal','large']){await selects.nth(1).selectOption(font);
  for(const locale of ['zh-CN','zh-TW','bo','en','ja','fr','es','de','ru','ko']){
   await selects.nth(0).selectOption(locale);
   await check('mobile '+font+' font '+locale,async()=>{assert(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth));});
  }}
  for(const key of ['light','dark','system']){await page.locator('.segmented button').nth(['light','dark','system'].indexOf(key)).tap();await check('mobile theme '+key,async()=>{assert.equal(await page.locator('html').getAttribute('data-theme'),key==='system'?'light':key);});}
  await page.screenshot({path:path.join(out,'settings-320-large.png')});
  await check('no unhandled page errors',()=>assert.deepEqual(errors,[]));await context.close();
 }finally{await browser.close();await vite.close();}
 const failed=results.filter(r=>r.status==='failed');fs.writeFileSync(path.join(out,'summary.json'),JSON.stringify({status:failed.length?'failed':'passed',results,errors,scope:'Chrome mobile/touch, synthetic microphone and explicit visualViewport fixture; not physical iOS/Android keyboard, real microphone or real HTTPS'},null,2));console.log(JSON.stringify({out,passed:results.length-failed.length,failed}));if(failed.length)process.exitCode=1;
})().catch(e=>{fs.writeFileSync(path.join(out,'harness-error.txt'),e.stack);console.error(e);process.exitCode=1});
