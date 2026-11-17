// Run with NODE_PATH pointing to an installed Playwright package; no browser downloads.
const {chromium}=require('playwright');
const http=require('http'),fs=require('fs'),path=require('path'),assert=require('assert');
const root=path.resolve(__dirname,'../cmd/blink/ui'),out=path.resolve(__dirname,'../work/ui');
fs.mkdirSync(out,{recursive:true});
const mime={'.html':'text/html; charset=utf-8','.js':'text/javascript','.css':'text/css','.svg':'image/svg+xml'};
const server=http.createServer((req,res)=>{const p=path.resolve(root,'.'+new URL(req.url,'http://localhost').pathname);const f=p===root?path.join(root,'index.html'):p;if(!f.startsWith(root+path.sep)){res.writeHead(403).end();return}fs.readFile(f,(e,b)=>{if(e){res.writeHead(404).end();return}res.setHeader('Content-Type',mime[path.extname(f)]||'text/plain');res.end(b)})});
(async()=>{
 await new Promise(r=>server.listen(0,'127.0.0.1',r));
 const browser=await chromium.launch({executablePath:process.env.BLINK_CHROMIUM||chromium.executablePath(),headless:true,args:['--no-sandbox']});
 try{
 const page=await browser.newPage({viewport:{width:1160,height:820},reducedMotion:'reduce'});const errors=[];page.on('pageerror',e=>errors.push(e.message));
 await page.addInitScript(()=>{
  window.calls=[];window.stateFixture={version:'0.2.2',arch:'386',profiles:[{index:0,name:'🇳🇱 Нидерланды · Amsterdam',kind:'xray',selected:true,routing:false},{index:1,name:'🇩🇪 Германия · Frankfurt',kind:'wireguard',selected:false,routing:true},{index:2,name:'🇫🇮 Финляндия · Helsinki',kind:'turn',selected:false,routing:true}],selected:0,busy:false,phase:'disconnected',message:'Всё начинается с одного подключения.',hasSubscription:true,revision:3,turn:{serverDefaults:true,transport:'auto',preset:'compatibility',dns:'1.1.1.1',relay:'',connections:1,mtu:1200,pace:0,window:64,interval:20,congestion:true},routing:{mode:'inherit',include:'',exclude:''}};
  let listener=()=>{};window.runtime={EventsOn:(name,fn)=>{listener=fn}};window.updateFixture=patch=>{Object.assign(window.stateFixture,patch);listener(structuredClone(window.stateFixture))};
  window.go={main:{Application:{Snapshot:async()=>structuredClone(window.stateFixture),Connect:async()=>{window.calls.push('Connect');window.updateFixture({phase:'connected',stats:{rx:106954752,tx:12373196}})},Disconnect:async()=>{window.calls.push('Disconnect');window.updateFixture({phase:'disconnected',captcha:null})},Import:async source=>{window.calls.push(['Import',source]);if(source==='invalid')throw 'Неподдерживаемый профиль'},ImportFile:async()=>{window.calls.push('ImportFile')},Refresh:async()=>{},Select:async i=>{window.calls.push(['Select',i]);window.stateFixture.profiles.forEach(p=>p.selected=p.index===i);window.updateFixture({selected:i})},Delete:async()=>{},Probe:async kind=>{window.calls.push(['Probe',kind]);window.updateFixture({diagnostic:{kind,ip:'203.0.113.10',dnsMs:12,tcpMs:23,tlsMs:45,responseMs:68,totalMs:148,checkedAt:1794934800}})},SaveSettings:async(turn,routing)=>{window.calls.push(['SaveSettings',turn,routing]);window.updateFixture({turn,routing})},Recover:async()=>{window.calls.push('Recover')},SubmitCaptcha:async(...args)=>{window.calls.push(['SubmitCaptcha',...args])}}}};
 });
 await page.goto(`http://127.0.0.1:${server.address().port}/`);await page.locator('#selected-name').filter({hasText:'Amsterdam'}).waitFor();
 assert((await page.locator('#architecture').innerText()).includes('32-bit'));
 await page.screenshot({path:path.join(out,'home-disconnected.png'),fullPage:true});
 // A type switch chooses an actual profile and renders offline SVG country flags.
 await page.locator('#page-home [data-mode=turn]').click();
 await page.locator('#selected-name').filter({hasText:'Helsinki'}).waitFor();
 assert.equal(await page.locator('#home-profile-list .profile-choice').count(),1);
 assert.equal(await page.locator('#selected-kind').innerText(),'TURN');
 await page.evaluate(()=>{window.stateFixture.profiles.push({index:3,name:'🇰🇿 Kazakhstan',kind:'turn',selected:false});window.updateFixture({})});
 await page.locator('#home-profile-list .profile-choice').filter({hasText:'Kazakhstan'}).click();
 await page.locator('#selected-name').filter({hasText:'Kazakhstan'}).waitFor();
 assert.equal(await page.locator('#server-avatar img').getAttribute('src'),'flags/kz.svg');
 await page.evaluate(()=>{window.stateFixture.profiles.find(p=>p.index===3).name='Kazakhstan';window.updateFixture({})});
 assert.equal(await page.locator('#server-avatar img').getAttribute('src'),'flags/kz.svg');
 await page.waitForFunction(()=>document.querySelector('#server-avatar img')?.naturalWidth>0);
 assert(!/VK|ВК/.test(await page.locator('#page-home').innerText()));
 await page.screenshot({path:path.join(out,'home-turn.png'),fullPage:true});
 await page.locator('#page-home [data-mode=vpn]').click();
 await page.locator('#selected-name').filter({hasText:'Amsterdam'}).waitFor();
 await page.locator('#page-home [data-mode=turn]').click();
 await page.locator('#selected-name').filter({hasText:'Kazakhstan'}).waitFor();
 await page.locator('#page-home [data-mode=vpn]').click();

 await page.evaluate(()=>{window.originalConnect=window.go.main.Application.Connect;window.go.main.Application.Connect=async()=>{throw new Error('Не удалось определить сетевой адаптер')}});
 await page.locator('#connect-button').click();await page.locator('#connection-error').filter({hasText:'Не удалось определить сетевой адаптер'}).waitFor();
 assert(!(await page.locator('#toast').innerText()).startsWith('Error:'));
 await page.evaluate(()=>{window.go.main.Application.Connect=window.originalConnect});
 await page.locator('#connect-button').click();await page.locator('#top-status').filter({hasText:'Соединение активно'}).waitFor();
 assert(await page.locator('#page-home [data-mode=turn]').isDisabled());
 assert(await page.locator('#connection-error').isHidden());assert((await page.locator('#rx').innerText()).includes('102'));await page.screenshot({path:path.join(out,'home-connected.png'),fullPage:true});
 await page.locator('[data-page=diagnostics]').click();await page.locator('[data-probe=ip]').click();await page.locator('#diagnostic-title').filter({hasText:'203.0.113.10'}).waitFor();await page.screenshot({path:path.join(out,'diagnostics.png'),fullPage:true});
 await page.locator('[data-page=home]').click();await page.locator('#connect-button').click();await page.locator('[data-page=profiles]').click();await page.locator('#profile-search').fill('Германия');assert.equal(await page.locator('.profile-row').count(),1);await page.locator('#profile-list .profile-choice').click();assert((await page.evaluate(()=>window.calls)).some(c=>Array.isArray(c)&&c[0]==='Select'&&c[1]===1));await page.locator('#profile-search').fill('');await page.screenshot({path:path.join(out,'profiles.png'),fullPage:true});
 await page.locator('#add-profile').click();await page.locator('#import-input').fill('invalid');await page.locator('#import-form button[type=submit]').click();await page.locator('#toast').filter({hasText:'Неподдерживаемый профиль'}).waitFor();assert.equal(await page.locator('#import-input').inputValue(),'');
 await page.locator('[data-page=settings]').click();await page.locator('#server-defaults').uncheck();await page.locator('#turn-transport').selectOption('tcp');await page.locator('#settings-form button[type=submit]').click();assert.equal(await page.evaluate(()=>window.stateFixture.turn.transport),'tcp');assert(!/VK|ВК/.test(await page.locator('#page-settings').innerText()));await page.screenshot({path:path.join(out,'settings.png'),fullPage:true});
 // Long imported subscriptions open a clean, searchable picker immediately.
 await page.evaluate(()=>{window.savedProfiles=structuredClone(window.stateFixture.profiles);window.updateFixture({profiles:Array.from({length:120},(_,i)=>({index:i,name:['🇰🇿 Казахстан · Алматы','🇩🇪 Германия · Frankfurt','🇺🇸 США · New York'][i%3]+' '+String(i+1).padStart(3,'0'),kind:i%2?'wireguard':'turn',selected:i===0}))})});
 await page.locator('[data-page=profiles]').click();await page.locator('#profile-search').fill('старый запрос');await page.locator('#add-profile').click();await page.locator('#import-input').fill('https://example.invalid/subscriptions/demo');await page.locator('#import-form button[type=submit]').click();await page.locator('#profile-picker').waitFor();
 assert.equal(await page.locator('#profile-search').inputValue(),'');assert.equal(await page.locator('#picker-search').inputValue(),'');assert.equal(await page.locator('#picker-list .server-option').count(),120);
 assert(await page.locator('#picker-list').evaluate(e=>e.scrollHeight>e.clientHeight),'large list must scroll inside picker');
 await page.screenshot({path:path.join(out,'server-picker.png'),fullPage:false});
 await page.locator('#picker-search').fill('United States 120');assert.equal(await page.locator('#picker-list .server-option').count(),1);
 await page.screenshot({path:path.join(out,'server-picker-search.png'),fullPage:false});
 await page.locator('#picker-search').press('Enter');await page.waitForFunction(()=>window.stateFixture.profiles.find(p=>p.index===119).selected);assert.equal(await page.locator('#profile-picker').evaluate(e=>e.open),false);
 await page.locator('#open-picker').click();await page.locator('[data-picker-filter=turn]').click();assert.equal(await page.locator('#picker-list .server-option').count(),60);
 await page.locator('#picker-search').fill('not-found');await page.locator('#picker-list button').filter({hasText:'Показать все серверы'}).click();assert.equal(await page.locator('#picker-list .server-option').count(),120);
 await page.locator('#close-picker').click();await page.locator('[data-page=profiles]').click();await page.locator('#profile-search').fill('казахстан');assert.equal(await page.locator('#profile-list .profile-row').count(),40);await page.locator('#clear-profile-search').click();assert.equal(await page.locator('#profile-list .profile-row').count(),120);
 await page.locator('[data-library-filter=turn]').click();assert.equal(await page.locator('#profile-list .profile-row').count(),60);await page.locator('[data-library-filter=all]').click();
 await page.screenshot({path:path.join(out,'profiles-many.png'),fullPage:false});
 // A subscription pasted in Search is moved to Import without any network request.
 const importsBefore=await page.evaluate(()=>window.calls.filter(c=>Array.isArray(c)&&c[0]==='Import').length);
 await page.locator('#profile-search').evaluate(e=>{const data=new DataTransfer();data.setData('text/plain','https://example.invalid/subscriptions/demo');e.dispatchEvent(new ClipboardEvent('paste',{bubbles:true,cancelable:true,clipboardData:data}))});
 assert(await page.locator('#import-dialog').evaluate(e=>e.open));assert.equal(await page.locator('#profile-search').inputValue(),'');assert.equal(await page.locator('#import-input').inputValue(),'https://example.invalid/subscriptions/demo');assert.equal(await page.evaluate(()=>window.calls.filter(c=>Array.isArray(c)&&c[0]==='Import').length),importsBefore);await page.locator('#close-import').click();
 await page.evaluate(()=>window.updateFixture({profiles:window.savedProfiles}));
 await page.locator('[data-page=home]').click();
 await page.locator('#home-search').fill('Amsterdam');assert.equal(await page.locator('#home-profile-list .home-profile-row').count(),1);await page.locator('#home-search').fill('missing');assert((await page.locator('#home-profile-list').innerText()).includes('Ничего не найдено'));await page.locator('#home-search').fill('');
 // Provider identifiers in imported names and errors are never product labels.
 await page.evaluate(()=>{window.stateFixture.profiles.find(p=>p.selected).name='VK TURN Нидерланды';window.updateFixture({message:'VK временный транспорт'})});assert(!/VK|ВК/i.test(await page.locator('#page-home').innerText()));
 await page.locator('[data-page=home]').click();
 await page.evaluate(()=>window.updateFixture({profiles:window.stateFixture.profiles.filter(p=>p.kind==='turn')}));
 await page.locator('#page-home [data-mode=vpn]').click();
 assert((await page.locator('#home-profile-list').innerText()).includes('Нет обычных VPN-профилей'));
 assert.equal(await page.locator('#connect-button span:last-child').innerText(),'Добавить профиль');
 await page.locator('#connect-button').click();assert(await page.locator('#import-dialog').evaluate(e=>e.open));await page.locator('#close-import').click();
 await page.evaluate(()=>window.updateFixture({profiles:[{index:0,name:'<img src=x onerror="window.injected=true">',kind:'xray',selected:true}]}));assert.equal(await page.evaluate(()=>window.injected),undefined);assert.equal(await page.locator('#selected-name img').count(),0);
 await page.setViewportSize({width:800,height:720});await page.locator('[data-page=home]').click();assert(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),'horizontal overflow at 800px');await page.screenshot({path:path.join(out,'compact.png'),fullPage:true});
 await page.emulateMedia({reducedMotion:'reduce'});assert.equal(await page.locator('.ring-progress').evaluate(e=>getComputedStyle(e).animationName),'none');
 // External CAPTCHA resources are mocked; no real VK network or credentials.
 await page.route('https://**.vk.com/**',route=>route.fulfill({contentType:'text/html',body:'<!doctype html><title>Mock challenge</title>'}));
 await page.evaluate(()=>window.updateFixture({phase:'captcha',captcha:{id:'test-challenge',url:'https://id.vk.com/captcha?session=fixture',expires_at:Math.floor(Date.now()/1000)+120}}));await page.locator('#captcha-container iframe').waitFor();
 await page.evaluate(()=>window.dispatchEvent(new MessageEvent('message',{origin:'https://id.vk.com',source:window,data:{type:'vk-sak-sdk',handler:'VKCaptchaGetResult',params:{token:'forged'}}})));await page.waitForTimeout(150);
 assert(!(await page.evaluate(()=>window.calls)).some(c=>Array.isArray(c)&&c[0]==='SubmitCaptcha'),'forged CAPTCHA accepted');
 const frame=page.frames().find(f=>f.url().startsWith('https://id.vk.com/'));assert(frame,'mock challenge frame missing');
 await frame.evaluate(()=>parent.postMessage({type:'vk-sak-sdk',handler:'VKCaptchaGetResult',params:{token:'fixture-token'}},'*'));
 await page.waitForFunction(()=>window.calls.some(c=>Array.isArray(c)&&c[0]==='SubmitCaptcha'&&c[3]==='fixture-token'));
 assert.equal((await page.evaluate(()=>window.calls)).filter(c=>Array.isArray(c)&&c[0]==='SubmitCaptcha').length,1);
 await page.locator('#cancel-captcha').click();assert.equal(await page.locator('#captcha-dialog').evaluate(e=>e.open),false);
 assert.deepEqual(errors,[]);console.log('UI passed: 120-server import picker, country/protocol search, scroll and keyboard selection, paste-to-import without fetching, reset filters, home search, VPN/TURN selection and remembered choice, empty mode, Kazakhstan SVG with/without emoji, neutral labels, persistent connection error, locked connected mode, navigation, connect/disconnect, diagnostics, profile search/select, import error, settings, XSS, compact layout, reduced motion, CAPTCHA origin/source guard and cancellation.');
 await page.close();
 }finally{await browser.close();server.close()}
})().catch(e=>{console.error(e);server.close();process.exitCode=1});
