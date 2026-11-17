'use strict';
const icons={shield:'<path d="M12 3 4.5 6v5.8c0 4.5 3.2 7.2 7.5 9.2 4.3-2 7.5-4.7 7.5-9.2V6Z"/><path d="m8.5 11.7 2.3 2.3 4.8-4.8"/>',power:'<path d="M12 3v8"/><path d="M7 5.8a8 8 0 1 0 10 0"/>',globe:'<circle cx="12" cy="12" r="9"/><ellipse cx="12" cy="12" rx="4" ry="9"/><path d="M3 12h18M4.5 7.5h15M4.5 16.5h15"/>',pulse:'<path d="M3 12h4l3-8 4 16 3-8h4"/>',settings:'<path d="m9 3-.5 3-2.5 1-2.5-1.4L1 10l2.2 2-1 3 4 2 1.8-1.8 3 1.3.5 3.5h5l.5-3.5 2.6-1 2.6 1.5 2-4-2.3-2 .8-2.8-3.9-2.1L18 7l-3-1-.5-3Z" transform="translate(1 0) scale(.9)"/><circle cx="12" cy="11.5" r="3"/>',desktop:'<rect x="3" y="4" width="18" height="13" rx="2"/><path d="M8 21h8m-4-4v4"/>',lock:'<rect x="5" y="10" width="14" height="11" rx="2"/><path d="M8 10V7a4 4 0 0 1 8 0v3m-4 4v3"/>','arrow-right':'<path d="M4 12h16m-6-6 6 6-6 6"/>','arrow-up-right':'<path d="M6 18 18 6M6 6h12v12"/>','arrow-down':'<path d="M12 4v16m-5-5 5 5 5-5"/>','arrow-up':'<path d="M12 20V4m-5 5 5-5 5 5"/>',activity:'<path d="M4 19v-4m5 4V9m5 10V5m5 14V2"/>',info:'<circle cx="12" cy="12" r="9"/><path d="M12 11v6m0-10v.1"/>',plus:'<path d="M12 5v14M5 12h14"/>',search:'<circle cx="10.5" cy="10.5" r="6.5"/><path d="m16 16 5 5"/>',refresh:'<path d="M20 7v5h-5M4 17v-5h5"/><path d="M5.2 8A7.5 7.5 0 0 1 18 6l2 3M4 15l2 3a7.5 7.5 0 0 0 12.8-2"/>',play:'<rect x="3" y="5" width="18" height="14" rx="4"/><path d="m10 9 5 3-5 3Z"/>',sliders:'<path d="M4 7h7m5 0h4M4 17h3m5 0h8"/><circle cx="13.5" cy="7" r="2.5"/><circle cx="9.5" cy="17" r="2.5"/>',route:'<circle cx="6" cy="5" r="2"/><circle cx="18" cy="19" r="2"/><path d="M6 7v7a3 3 0 0 0 3 3h6m3 0V7a3 3 0 0 0-3-3h-5"/>',check:'<path d="m5 12 4 4L19 6"/>',close:'<path d="m6 6 12 12M6 18 18 6"/>',folder:'<path d="M3 7V5a2 2 0 0 1 2-2h5l3 3h6a2 2 0 0 1 2 2v11H3Z"/>',trash:'<path d="M3 6h18M9 6V3h6v3M5 6l1 15h12l1-15M10 10v7m4-7v7"/>'};
const $=s=>document.querySelector(s);
const svg=name=>`<svg viewBox="0 0 24 24" aria-hidden="true">${icons[name]||icons.globe}</svg>`;
function decorate(root=document){root.querySelectorAll('[data-icon]').forEach(e=>{e.innerHTML=svg(e.dataset.icon)})}decorate();
let state=null,currentPage='home',settingsDirty=false,captchaID='',toastTimer,activeMode=null,lastSelected=null;
const rememberedProfiles={};
const profileMode=p=>p?.kind==='vk'?'turn':'vpn';
const modeProfiles=()=>state?.profiles.filter(p=>profileMode(p)===activeMode)||[];
let trafficSamples=Array(30).fill(0),previousTraffic=null;
const api=()=>window.go?.main?.Application;
function toast(message){const t=$('#toast');t.textContent=String(message?.message||message||'Не удалось выполнить действие').replace(/^Error:\s*/, '');t.classList.add('visible');clearTimeout(toastTimer);toastTimer=setTimeout(()=>t.classList.remove('visible'),5500)}
async function call(method,...args){if(!api()){toast('Запустите Blink, чтобы управлять соединением');return false}try{await api()[method](...args);return true}catch(e){toast(e);if(method==='Connect'){const error=$('#connection-error');error.textContent=String(e?.message||e).replace(/^Error:\s*/, '');error.hidden=false}return false}}
function page(name){currentPage=name;document.querySelectorAll('.page').forEach(p=>p.classList.toggle('active',p.id===`page-${name}`));document.querySelectorAll('[data-page]').forEach(p=>p.classList.toggle('active',p.dataset.page===name));$('#breadcrumb').textContent={home:'Подключение',profiles:'Профили',diagnostics:'Диагностика',settings:'Настройки'}[name];window.scrollTo(0,0)}
document.querySelectorAll('[data-page]').forEach(e=>e.addEventListener('click',()=>page(e.dataset.page)));document.querySelectorAll('[data-go]').forEach(e=>e.addEventListener('click',()=>page(e.dataset.go)));$('.brand').addEventListener('click',e=>{e.preventDefault();page('home')});
function openImport(){if(state?.busy||state?.phase!=='disconnected'){if(state)toast('Отключитесь перед импортом');return}$('#import-dialog').showModal();$('#import-input').focus()}
['#add-profile','#empty-add','#picker-add'].forEach(id=>$(id).addEventListener('click',openImport));$('#close-import').onclick=()=>$('#import-dialog').close();$('#import-dialog').addEventListener('close',()=>{$('#import-input').value=''})
$('#import-form').addEventListener('submit',async e=>{e.preventDefault();const source=$('#import-input').value.trim();if(!source)return;$('#import-input').value='';$('#import-dialog').close();await call('Import',source)});$('#import-file').onclick=async()=>{$('#import-dialog').close();await call('ImportFile')};$('#refresh-button').onclick=()=>call('Refresh');$('#profile-search').addEventListener('input',renderProfiles);
async function toggle(){if(!state)return;$('#connection-error').hidden=true;const active=['connected','connecting','captcha'].includes(state.phase);if(active){await call('Disconnect')}else if(!modeProfiles().length){page('profiles');openImport()}else{await call('Connect')}}$('#power-button').onclick=toggle;$('#connect-button').onclick=toggle;
const kinds={vk:'TURN',xray:'Xray','xray-json':'Xray JSON',wireguard:'WireGuard',amneziawg:'AmneziaWG',tuic:'TUIC v5',hysteria1:'Hysteria v1'};
const countryNames=[];
for(const locale of ['ru','en']){
 const names=new Intl.DisplayNames([locale],{type:'region'});
 for(const code of flagCountries)countryNames.push([names.of(code).toLocaleLowerCase('ru'),code]);
}
countryNames.sort((a,b)=>b[0].length-a[0].length);
function flagCode(name){
 const text=String(name),match=text.match(/[\u{1F1E6}-\u{1F1FF}]{2}/u);
 const emoji=match?[...match[0]].map(c=>String.fromCharCode(c.codePointAt(0)-0x1f1e6+65)).join(''):'';
 if(flagCountries.includes(emoji))return emoji;
 const normalized=text.toLocaleLowerCase('ru');
 for(const [country,code] of countryNames)if(normalized===country||normalized.startsWith(country+' ')||normalized.startsWith(country+' ·')||normalized.startsWith(country+'-'))return code;
 const short=text.match(/^(?:\[)?([A-Z]{2})(?:\]|[\s·_-]|$)/);return short&&flagCountries.includes(short[1])?short[1]:'';
}
function displayName(name){return String(name).replace(/[\u{1F1E6}-\u{1F1FF}]{2}/gu,'').trim()}
function setAvatar(element,p){
 element.replaceChildren();const code=p&&flagCode(p.name);
 if(code){const img=document.createElement('img');img.className='country-flag';img.alt=code;img.src=`flags/${code.toLowerCase()}.svg`;element.append(img)}
 else element.innerHTML=svg(p?.kind==='vk'?'shield':'globe');
}
async function switchMode(mode){
 if(!state||state.busy||state.phase!=='disconnected')return;
 activeMode=mode;
 const profiles=modeProfiles(),selected=profiles.find(p=>p.selected);
 if(!selected&&profiles.length){
  const target=profiles.find(p=>p.index===rememberedProfiles[mode])||profiles[0];
  if(!await call('Select',target.index)){activeMode=profileMode(state.profiles.find(p=>p.selected))}
 }
 render(state);
}
document.querySelectorAll('[data-mode]').forEach(button=>button.onclick=()=>switchMode(button.dataset.mode));
function profileChoice(p){
 const choice=document.createElement('button');choice.className='profile-choice';choice.disabled=state.busy||state.phase!=='disconnected';choice.setAttribute('aria-pressed',String(p.selected));
 const avatar=document.createElement('span');avatar.className='profile-avatar';setAvatar(avatar,p);
 const copy=document.createElement('span');copy.className='profile-copy';const name=document.createElement('strong');name.textContent=displayName(p.name);
 const kind=document.createElement('small');kind.textContent=kinds[p.kind]||'VPN';copy.append(name,kind);
 const check=document.createElement('span');check.className='profile-check';if(p.selected)check.innerHTML=svg('check');
 choice.append(avatar,copy,check);choice.onclick=()=>call('Select',p.index);return choice;
}
function renderPicker(){
 const locked=state.busy||state.phase!=='disconnected';
 document.querySelectorAll('[data-mode]').forEach(button=>{button.setAttribute('aria-pressed',String(button.dataset.mode===activeMode));button.disabled=locked});
 document.querySelectorAll('[data-count]').forEach(count=>count.textContent=state.profiles.filter(p=>profileMode(p)===count.dataset.count).length);
 const list=$('#home-profile-list');list.replaceChildren();
 for(const p of modeProfiles()){const row=document.createElement('div');row.className=`home-profile-row${p.selected?' selected':''}`;row.append(profileChoice(p));list.append(row)}
 if(!modeProfiles().length){const empty=document.createElement('p');empty.className='picker-empty';empty.textContent=activeMode==='turn'?'Нет TURN-профилей. Добавьте подписку с поддержкой TURN.':'Нет обычных VPN-профилей. Добавьте конфигурацию или подписку с VPN-профилями.';list.append(empty)}
 $('#picker-add').disabled=locked;
}
function formatBytes(n){n=Number(n)||0;const units=['Б','КиБ','МиБ','ГиБ'];let i=0;while(n>=1024&&i<3){n/=1024;i++}return `${n.toLocaleString('ru-RU',{maximumFractionDigits:i?1:0})} <small>${units[i]}</small>`}
function renderProfiles(){
 if(!state)return;const list=$('#profile-list');list.replaceChildren();const filter=$('#profile-search').value.trim().toLocaleLowerCase('ru');
 const profiles=modeProfiles(),matches=profiles.filter(p=>p.name.toLocaleLowerCase('ru').includes(filter));$('#empty-profiles').hidden=state.profiles.length>0;
 for(const p of matches){const row=document.createElement('div');row.className=`profile-row${p.selected?' selected':''}`;
 const del=document.createElement('button');del.className='icon-button';del.innerHTML=svg('trash');del.setAttribute('aria-label',`Удалить ${p.name}`);del.disabled=state.busy||state.phase!=='disconnected';del.onclick=async()=>{if(confirm(`Удалить профиль «${p.name}»?`))await call('Delete',p.index)};
 row.append(profileChoice(p),del);list.append(row)}
 if(state.profiles.length&&!matches.length){const empty=document.createElement('p');empty.className='field-help';empty.textContent=profiles.length?'По этому запросу ничего не найдено.':activeMode==='turn'?'В этой подписке нет TURN-профилей.':'В этой подписке нет обычных VPN-профилей. Добавьте конфигурацию или подписку с VPN-профилями.';list.append(empty)}
}
function setSettings(s){const locked=s.busy||s.phase!=='disconnected';$('#settings-form').querySelectorAll('input,textarea,select,button').forEach(e=>{if(!e.closest('#vk-fields'))e.disabled=locked});$('#vk-fields').disabled=locked||$('#server-defaults').checked;if(settingsDirty)return;const v=s.vk;$('#server-defaults').checked=v.serverDefaults;for(const key of ['transport','preset','dns','relay','connections','mtu','pace','window','interval'])$(`#vk-${key}`).value=v[key];$('#vk-congestion').checked=v.congestion;$('#routing-mode').value=s.routing.mode||'inherit';$('#routing-include').value=s.routing.include||'';$('#routing-exclude').value=s.routing.exclude||'';$('#vk-fields').disabled=v.serverDefaults||s.phase!=='disconnected';$('#settings-form').querySelectorAll('input,textarea,select,button').forEach(e=>{if(!e.closest('#vk-fields'))e.disabled=s.busy||s.phase!=='disconnected'})}
$('#settings-form').addEventListener('input',()=>{settingsDirty=true;$('#vk-fields').disabled=$('#server-defaults').checked});$('#settings-form').addEventListener('submit',async e=>{e.preventDefault();const vk={serverDefaults:$('#server-defaults').checked,congestion:$('#vk-congestion').checked};for(const k of ['transport','preset','dns','relay'])vk[k]=$(`#vk-${k}`).value.trim();for(const k of ['connections','mtu','pace','window','interval'])vk[k]=Number($(`#vk-${k}`).value);const routing={mode:$('#routing-mode').value,include:$('#routing-include').value,exclude:$('#routing-exclude').value};if(await call('SaveSettings',vk,routing)){settingsDirty=false;toast('Настройки сохранены');const fresh=await api().Snapshot();render(fresh)}});$('#recover-button').onclick=()=>call('Recover');
document.querySelectorAll('[data-probe]').forEach(e=>e.onclick=()=>call('Probe',e.dataset.probe));
function renderDiagnostic(d){if(!d)return;$('#diagnostic-title').textContent=d.ip||({google:'Google доступен',youtube:'YouTube доступен'}[d.kind]||'HTTPS доступен');$('#diagnostic-description').textContent=`Проверено ${new Date(d.checkedAt*1000).toLocaleTimeString('ru-RU')} · Маршруты текущего профиля`;const metrics=$('#diagnostic-metrics');metrics.replaceChildren();for(const [key,label] of [['dnsMs','DNS'],['tcpMs','TCP'],['tlsMs','TLS'],['responseMs','Ответ'],['totalMs','Всего']]){if(d[key]===undefined)continue;const span=document.createElement('span');span.textContent=label;const strong=document.createElement('strong');strong.textContent=`${d[key]} мс`;span.append(strong);metrics.append(span)}}
function renderTraffic(s){
 const total=(Number(s.stats?.rx)||0)+(Number(s.stats?.tx)||0);
 if(s.phase!=='connected'){trafficSamples=Array(30).fill(0);previousTraffic=null}else{const delta=previousTraffic===null?0:Math.max(0,total-previousTraffic);trafficSamples.push(delta);trafficSamples.shift();previousTraffic=total}
 const maximum=Math.max(1,...trafficSamples),d=trafficSamples.map((n,i)=>`${i?'L':'M'}${(i*360/29).toFixed(1)} ${(43-n/maximum*35).toFixed(1)}`).join(' ');
 $('.traffic-spark path').setAttribute('d',d);
}
function render(s){state=s;const selectedProfile=s.profiles.find(p=>p.selected);const selectionKey=selectedProfile?`${selectedProfile.index}:${selectedProfile.kind}:${selectedProfile.name}`:null;if(activeMode===null||selectedProfile&&lastSelected!==selectionKey){activeMode=profileMode(selectedProfile)}lastSelected=selectionKey;if(selectedProfile)rememberedProfiles[profileMode(selectedProfile)]=selectedProfile.index;const connected=s.phase==='connected',busy=['connecting','disconnecting'].includes(s.phase),active=connected||s.phase==='captcha'||s.phase==='connecting';$('#version').textContent=`BLINK ${s.version}`;$('#about-version').textContent=`Blink ${s.version}`;$('#architecture').textContent=s.arch==='386'?'32-bit · x86':'64-bit · x64';$('#profile-count').textContent=s.profiles.length;const top=$('.top-status');top.classList.toggle('connected',connected);$('#top-status').textContent=connected?'Соединение активно':busy?'Подключение…':s.phase==='captcha'?'Ожидает проверки':'Не подключено';const orbit=$('#power-button');orbit.classList.toggle('connected',connected);orbit.classList.toggle('busy',busy);orbit.disabled=s.phase==='disconnecting'||s.busy&&!active;orbit.setAttribute('aria-label',active?'Отключить':'Подключить');const pill=$('#connection-pill');pill.classList.toggle('active',connected);pill.replaceChildren();pill.append(document.createElement('span'),document.createTextNode(connected?'Соединение активно':busy?'Соединяем…':s.phase==='captcha'?'Проверка':'Ожидает подключения'));$('#connection-title').textContent=connected?'Можно просто быть онлайн':busy?'Уже почти на месте':s.phase==='captcha'?'Один небольшой шаг':'Ваш момент тишины';$('#connection-subtitle').textContent=connected?'Хорошее соединение — когда его не замечаешь.':busy?'Готовим ваш маршрут. Это займёт немного времени.':s.phase==='captcha'?'Нужна ручная проверка.':'Подключитесь, чтобы открыть больше.';$('#connect-button span:last-child').textContent=active?'Отключиться':modeProfiles().length?'Подключиться':'Добавить профиль';$('#connect-button').disabled=s.phase==='disconnecting'||s.busy&&!active;$('#message').textContent=s.message;const p=modeProfiles().find(p=>p.selected);$('#selected-name').textContent=p?displayName(p.name):(activeMode==='turn'?'Выберите TURN-профиль':'Выберите VPN-профиль');$('#selected-kind').textContent=p?`${kinds[p.kind]||p.kind}${s.expiresAt?' · до '+new Date(s.expiresAt*1000).toLocaleDateString('ru-RU'):''}`:'Всё нужное для первого подключения.';setAvatar($('#server-avatar'),p);$('#rx').innerHTML=formatBytes(s.stats?.rx);$('#tx').innerHTML=formatBytes(s.stats?.tx);$('#refresh-button').disabled=!s.hasSubscription||s.busy||active;$('#recover-button').disabled=s.busy||active;$('#add-profile').disabled=s.busy||active;document.querySelectorAll('[data-probe]').forEach(e=>e.disabled=!connected||s.busy);renderTraffic(s);renderPicker();renderProfiles();setSettings(s);renderDiagnostic(s.diagnostic);renderCaptcha(s.captcha)}
// Modern CAPTCHA is mounted by the pinned official VK ID Web SDK below.
let captchaWidget=null;
function closeCaptcha(){captchaWidget?.destroy?.();captchaWidget=null;$('#captcha-container').replaceChildren();if($('#captcha-dialog').open)$('#captcha-dialog').close();captchaID=''}
function renderCaptcha(c){if(!c){closeCaptcha();return}if(c.id===captchaID)return;closeCaptcha();captchaID=c.id;$('#captcha-dialog').showModal();$('#captcha-legacy').hidden=!!c.url;if(c.url){mountModernCaptcha(c)}else if(c.image){const img=document.createElement('img');img.alt='Проверочный код';img.referrerPolicy='no-referrer';img.src=c.image;$('#captcha-container').append(img);$('#captcha-help').textContent='Введите символы с изображения. Ответ действует только для текущей проверки.'}}
async function mountModernCaptcha(c){$('#captcha-help').textContent='Загружаем ручную проверку…';if(!window.BlinkCaptcha){$('#captcha-help').textContent='Не удалось загрузить SDK проверки. Попробуйте заново или отмените подключение.';return}try{captchaWidget=window.BlinkCaptcha.mount(c,$('#captcha-container'),async token=>{if(captchaID!==c.id)return;await call('SubmitCaptcha',c.id,'token',token)},message=>{$('#captcha-help').textContent=message});$('#captcha-help').textContent='Пройдите проверку вручную. Blink продолжит тот же запрос.'}catch(e){$('#captcha-help').textContent='Не удалось открыть проверку. Проверьте доступ к обычной сети.'}}
$('#cancel-captcha').onclick=()=>call('Disconnect');$('#captcha-dialog').addEventListener('cancel',e=>{e.preventDefault();call('Disconnect')});$('#captcha-retry').onclick=()=>{const c=state?.captcha;if(!c)return;captchaID='';renderCaptcha(c)};$('#captcha-legacy').addEventListener('submit',async e=>{e.preventDefault();const answer=$('#captcha-answer').value.trim();$('#captcha-answer').value='';if(captchaID)await call('SubmitCaptcha',captchaID,'key',answer)});
async function boot(){if(!api()){setTimeout(boot,80);return}window.runtime?.EventsOn('blink:state',render);try{render(await api().Snapshot())}catch(e){toast(e)}}boot();
