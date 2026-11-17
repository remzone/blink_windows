// Real Windows/WebView2 smoke test. Never connects a VPN or changes routes.
const {chromium}=require('playwright');
const {spawn}=require('child_process');const fs=require('fs'),path=require('path'),net=require('net'),assert=require('assert');
const arch=process.argv[2]||'amd64',root=path.resolve(__dirname,'..'),dir=path.join(root,'work',`windows-smoke-${arch}`);fs.mkdirSync(dir,{recursive:true});
(async()=>{
 const socket=net.createServer();await new Promise(r=>socket.listen(0,'127.0.0.1',r));const port=socket.address().port;await new Promise(r=>socket.close(r));
 const child=spawn(path.join(root,'dist',arch,'Blink.exe'),[],{env:{...process.env,APPDATA:dir,WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS:`--remote-debugging-port=${port} --remote-debugging-address=127.0.0.1`},windowsHide:false});
 let launchError;child.on('error',e=>{launchError=e});let browser;
 try{
  for(let i=0;i<90;i++){if(launchError)throw launchError;if(child.exitCode!==null)throw new Error(`Blink exited: ${child.exitCode}`);try{browser=await chromium.connectOverCDP(`http://127.0.0.1:${port}`,{timeout:800});break}catch{}await new Promise(r=>setTimeout(r,500))}
  assert(browser,'WebView2 debugging endpoint did not appear');const context=browser.contexts()[0];let page;
  for(let i=0;i<60;i++){page=context.pages().find(p=>p.url().includes('wails'));if(page)break;await new Promise(r=>setTimeout(r,300))}assert(page,'Application document did not load');
  await page.waitForFunction(()=>!!window.go?.main?.Application,{timeout:30000});const snapshot=await page.evaluate(()=>window.go.main.Application.Snapshot());assert.equal(snapshot.arch,arch);assert.equal(snapshot.phase,'disconnected');assert.deepEqual(snapshot.profiles,[]);assert(!('subscriptionURL' in snapshot));
  await page.locator('[data-page=profiles]').click();await page.locator('#empty-add').click();await page.locator('#import-input').fill('vless://00000000-0000-4000-8000-000000000001@vpn.example.org:443?type=tcp&security=tls&sni=vpn.example.org#Windows%20smoke%20fixture');await page.locator('#import-form button[type=submit]').click();await page.locator('.profile-row').waitFor();
  assert.equal(await page.locator('.profile-copy strong').innerText(),'Windows smoke fixture');
  const stateFile=path.join(dir,'Blink','profiles.dpapi');assert(fs.existsSync(stateFile));assert(!fs.readFileSync(stateFile).includes(Buffer.from('00000000-0000-4000-8000-000000000001')));
  await page.locator('[data-page=settings]').click();await page.locator('#server-defaults').uncheck();await page.locator('#vk-transport').selectOption('tcp');await page.locator('#settings-form button[type=submit]').click();await page.locator('#toast').filter({hasText:'Настройки сохранены'}).waitFor();
  assert.equal((await page.evaluate(()=>window.go.main.Application.Snapshot())).vk.transport,'tcp');
  await page.locator('[data-page=home]').click();await page.screenshot({path:path.join(dir,'windows-ui.png')});
  console.log(`Windows ${arch}: real WebView2, Go bindings, profile import, DPAPI write, settings save, and screenshot passed. No VPN started.`);
 }finally{if(browser)await browser.close();if(child.pid)try{require('child_process').execFileSync('taskkill',['/PID',String(child.pid),'/T','/F'],{stdio:'ignore'})}catch{}}
})().catch(e=>{console.error(e);process.exitCode=1});
