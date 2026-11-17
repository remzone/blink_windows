'use strict';
const icons={shield:'<path d="M12 3 4.5 6v5.8c0 4.5 3.2 7.2 7.5 9.2 4.3-2 7.5-4.7 7.5-9.2V6Z"/><path d="m8.5 11.7 2.3 2.3 4.8-4.8"/>',power:'<path d="M12 3v8"/><path d="M7 5.8a8 8 0 1 0 10 0"/>',globe:'<circle cx="12" cy="12" r="9"/><ellipse cx="12" cy="12" rx="4" ry="9"/><path d="M3 12h18M4.5 7.5h15M4.5 16.5h15"/>',pulse:'<path d="M3 12h4l3-8 4 16 3-8h4"/>',settings:'<path d="m9 3-.5 3-2.5 1-2.5-1.4L1 10l2.2 2-1 3 4 2 1.8-1.8 3 1.3.5 3.5h5l.5-3.5 2.6-1 2.6 1.5 2-4-2.3-2 .8-2.8-3.9-2.1L18 7l-3-1-.5-3Z" transform="translate(1 0) scale(.9)"/><circle cx="12" cy="11.5" r="3"/>',desktop:'<rect x="3" y="4" width="18" height="13" rx="2"/><path d="M8 21h8m-4-4v4"/>',lock:'<rect x="5" y="10" width="14" height="11" rx="2"/><path d="M8 10V7a4 4 0 0 1 8 0v3m-4 4v3"/>','arrow-right':'<path d="M4 12h16m-6-6 6 6-6 6"/>','arrow-up-right':'<path d="M6 18 18 6M6 6h12v12"/>','arrow-down':'<path d="M12 4v16m-5-5 5 5 5-5"/>','arrow-up':'<path d="M12 20V4m-5 5 5-5 5 5"/>',activity:'<path d="M4 19v-4m5 4V9m5 10V5m5 14V2"/>',info:'<circle cx="12" cy="12" r="9"/><path d="M12 11v6m0-10v.1"/>',plus:'<path d="M12 5v14M5 12h14"/>',search:'<circle cx="10.5" cy="10.5" r="6.5"/><path d="m16 16 5 5"/>',refresh:'<path d="M20 7v5h-5M4 17v-5h5"/><path d="M5.2 8A7.5 7.5 0 0 1 18 6l2 3M4 15l2 3a7.5 7.5 0 0 0 12.8-2"/>',play:'<rect x="3" y="5" width="18" height="14" rx="4"/><path d="m10 9 5 3-5 3Z"/>',sliders:'<path d="M4 7h7m5 0h4M4 17h3m5 0h8"/><circle cx="13.5" cy="7" r="2.5"/><circle cx="9.5" cy="17" r="2.5"/>',route:'<circle cx="6" cy="5" r="2"/><circle cx="18" cy="19" r="2"/><path d="M6 7v7a3 3 0 0 0 3 3h6m3 0V7a3 3 0 0 0-3-3h-5"/>',check:'<path d="m5 12 4 4L19 6"/>',close:'<path d="m6 6 12 12M6 18 18 6"/>',folder:'<path d="M3 7V5a2 2 0 0 1 2-2h5l3 3h6a2 2 0 0 1 2 2v11H3Z"/>',trash:'<path d="M3 6h18M9 6V3h6v3M5 6l1 15h12l1-15M10 10v7m4-7v7"/>'};
const $=s=>document.querySelector(s);
const svg=name=>`<svg viewBox="0 0 24 24" aria-hidden="true">${icons[name]||icons.globe}</svg>`;
function decorate(root=document){root.querySelectorAll('[data-icon]').forEach(e=>{e.innerHTML=svg(e.dataset.icon)})}decorate();
let state=null,currentPage='home',settingsDirty=false,captchaID='',toastTimer,activeMode=null,lastSelected=null,libraryFilter='all',pickerFilter='all';
const rememberedProfiles={};
const profileMode=p=>p?.kind==='turn'?'turn':'vpn';
const modeProfiles=()=>state?.profiles.filter(p=>profileMode(p)===activeMode)||[];
let trafficSamples=Array(30).fill(0),previousTraffic=null;
const api=()=>window.go?.main?.Application;
function productText(value){return String(value??'').replace(/vk[ _-]*turn/gi,'TURN').replace(/vk/gi,'TURN').replace(/(^|[^\p{L}\p{N}])ВК(?=$|[^\p{L}\p{N}])/giu,'$1TURN')}
function toast(message){const t=$('#toast');t.textContent=productText(message?.message||message||'Не удалось выполнить действие').replace(/^Error:\s*/, '');t.classList.add('visible');clearTimeout(toastTimer);toastTimer=setTimeout(()=>t.classList.remove('visible'),5500)}
async function call(method,...args){if(!api()){toast('Запустите Blink, чтобы управлять соединением');return false}try{await api()[method](...args);return true}catch(e){toast(e);if(method==='Connect'){const error=$('#connection-error');error.textContent=productText(e?.message||e).replace(/^Error:\s*/, '');error.hidden=false}return false}}
function page(name){currentPage=name;document.querySelectorAll('.page').forEach(p=>p.classList.toggle('active',p.id===`page-${name}`));document.querySelectorAll('[data-page]').forEach(p=>p.classList.toggle('active',p.dataset.page===name));$('#breadcrumb').textContent={home:'Подключение',profiles:'Профили',diagnostics:'Диагностика',settings:'Настройки'}[name];window.scrollTo(0,0)}
document.querySelectorAll('[data-page]').forEach(e=>e.addEventListener('click',()=>page(e.dataset.page)));document.querySelectorAll('[data-go]').forEach(e=>e.addEventListener('click',()=>page(e.dataset.go)));$('.brand').addEventListener('click',e=>{e.preventDefault();page('home')});
function openImport(source=''){if(typeof source!=='string')source='';if(state?.busy||state?.phase!=='disconnected'){if(state)toast('Отключитесь перед импортом');return}$('#import-input').value=source;$('#import-dialog').showModal();$('#import-input').focus()}
['#add-profile','#empty-add','#picker-add'].forEach(id=>$(id).addEventListener('click',openImport));$('#close-import').onclick=()=>$('#import-dialog').close();$('#import-dialog').addEventListener('close',()=>{$('#import-input').value=''})
$('#import-form').addEventListener('submit',async e=>{e.preventDefault();const source=$('#import-input').value.trim();if(!source)return;$('#import-input').value='';$('#import-dialog').close();if(await call('Import',source))await showImportedProfiles()});$('#import-file').onclick=async()=>{$('#import-dialog').close();if(await call('ImportFile'))await showImportedProfiles()};$('#refresh-button').onclick=()=>call('Refresh');$('#profile-search').addEventListener('input',renderProfiles);
async function toggle(){if(!state)return;$('#connection-error').hidden=true;const active=['connected','connecting','captcha'].includes(state.phase);if(active){await call('Disconnect')}else if(!modeProfiles().length){page('profiles');openImport()}else{await call('Connect')}}$('#power-button').onclick=toggle;$('#connect-button').onclick=toggle;
const kinds={turn:'TURN',xray:'Xray','xray-json':'Xray JSON',wireguard:'WireGuard',amneziawg:'AmneziaWG',tuic:'TUIC v5',hysteria1:'Hysteria v1'};
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
function displayName(name){return productText(name).replace(/[\u{1F1E6}-\u{1F1FF}]{2}/gu,'').trim()}
function setAvatar(element,p){
 element.replaceChildren();const code=p&&flagCode(p.name);
 if(code){const img=document.createElement('img');img.className='country-flag';img.alt=code;img.src=`flags/${code.toLowerCase()}.svg`;element.append(img)}
 else element.innerHTML=svg(p?.kind==='turn'?'shield':'globe');
}
async function switchMode(mode){
 if(!state||state.busy||state.phase!=='disconnected')return;
 activeMode=mode;
 const profiles=modeProfiles(),selected=profiles.find(p=>p.selected);
 if(!selected&&profiles.length){
  const target=profiles.find(p=>p.index===(state.selections?.[mode]??rememberedProfiles[mode]))||profiles[0];
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
 choice.append(avatar,copy,check);choice.onclick=async()=>{if(await call('Select',p.index)){if($('#profile-picker').open){$('#profile-picker').close();page('home')}$('#home-search').value=''}};return choice;
}
function renderPicker(){
 const locked=state.busy||state.phase!=='disconnected';
 document.querySelectorAll('[data-mode]').forEach(button=>{button.setAttribute('aria-pressed',String(button.dataset.mode===activeMode));button.disabled=locked});
 document.querySelectorAll('[data-count]').forEach(count=>count.textContent=count.dataset.count==='all'?state.profiles.length:state.profiles.filter(p=>profileMode(p)===count.dataset.count).length);
 const list=$('#home-profile-list');list.replaceChildren();
 for(const p of searchProfiles(modeProfiles(),$('#home-search').value)){const row=document.createElement('div');row.className=`home-profile-row${p.selected?' selected':''}`;row.append(profileChoice(p));list.append(row)}
 if(!modeProfiles().length){const empty=document.createElement('p');empty.className='picker-empty';empty.textContent=activeMode==='turn'?'Нет TURN-профилей. Добавьте подписку с поддержкой TURN.':'Нет обычных VPN-профилей. Добавьте конфигурацию или подписку с VPN-профилями.';list.append(empty)}
 $('#picker-add').disabled=locked;$('#open-picker').disabled=locked||!state.profiles.length;$('#picker-total').textContent=state.profiles.length;$('#home-search').disabled=locked;
 if(modeProfiles().length&&!list.children.length){const empty=document.createElement('p');empty.className='picker-empty';empty.textContent='Ничего не найдено. Попробуйте другую страну или откройте все серверы.';list.append(empty)}
}
function formatBytes(n){n=Number(n)||0;const units=['Б','КиБ','МиБ','ГиБ'];let i=0;while(n>=1024&&i<3){n/=1024;i++}return `${n.toLocaleString('ru-RU',{maximumFractionDigits:i?1:0})} <small>${units[i]}</small>`}
function renderProfiles(){
 if(!state)return;const list=$('#profile-list');list.replaceChildren();const filter=$('#profile-search').value.trim().toLocaleLowerCase('ru');
 const profiles=filteredProfiles(libraryFilter),matches=searchProfiles(profiles,filter);
 $('#library-results').textContent=`Найдено ${matches.length} из ${profiles.length}`;
 document.querySelectorAll('[data-library-filter]').forEach(b=>b.setAttribute('aria-pressed',String(b.dataset.libraryFilter===libraryFilter)));$('#empty-profiles').hidden=state.profiles.length>0;
 for(const p of matches){const row=document.createElement('div');row.className=`profile-row${p.selected?' selected':''}`;
 const del=document.createElement('button');del.className='icon-button';del.innerHTML=svg('trash');del.setAttribute('aria-label',`Удалить ${displayName(p.name)}`);del.disabled=state.busy||state.phase!=='disconnected';del.onclick=async()=>{if(confirm(`Удалить профиль «${displayName(p.name)}»?`))await call('Delete',p.index)};
 row.append(profileChoice(p),del);list.append(row)}
 if(state.profiles.length&&!matches.length){const empty=document.createElement('p');empty.className='field-help';empty.textContent=profiles.length?'Ничего не найдено. Очистите поиск или выберите вкладку «Все».':libraryFilter==='turn'?'В этой подписке нет TURN-профилей.':'В этой подписке нет обычных VPN-профилей. Добавьте конфигурацию или подписку с VPN-профилями.';list.append(empty);if(filter){const reset=document.createElement('button');reset.className='text-button';reset.textContent='Сбросить поиск';reset.onclick=()=>{$('#profile-search').value='';renderProfiles();$('#profile-search').focus()};list.append(reset)}}
}
function normalizedSearch(value){return productText(value).toLocaleLowerCase('ru').normalize('NFD').replace(/[\u0300-\u036f]/g,'').replace(/ё/g,'е')}
function filteredProfiles(filter){return state.profiles.filter(p=>filter==='all'||profileMode(p)===filter)}
function searchProfiles(profiles,query){
 const words=normalizedSearch(query).trim().split(/\s+/).filter(Boolean);
 return profiles.filter(p=>{const code=flagCode(p.name),countries=countryNames.filter(item=>item[1]===code).map(item=>item[0]).join(' ');const haystack=normalizedSearch(`${displayName(p.name)} ${kinds[p.kind]||''} ${profileMode(p)==='turn'?'TURN':'VPN'} ${code} ${countries}`);return words.every(word=>haystack.includes(word))});
}
async function showImportedProfiles(){
 render(await api().Snapshot());$('#profile-search').value='';$('#home-search').value='';libraryFilter='all';page('profiles');renderProfiles();
 if(state.profiles.length)openServerPicker('all');
}
function openServerPicker(filter=activeMode){
 if(!state||state.busy||state.phase!=='disconnected')return;
 pickerFilter=filter||'all';$('#picker-search').value='';renderServerPicker();$('#profile-picker').showModal();$('#picker-search').focus();
}
function renderServerPicker(){
 const profiles=filteredProfiles(pickerFilter),matches=searchProfiles(profiles,$('#picker-search').value),list=$('#picker-list'),scroll=list.scrollTop;
 list.replaceChildren();$('#picker-results').textContent=`Найдено ${matches.length} из ${profiles.length}`;
 document.querySelectorAll('[data-picker-filter]').forEach(b=>{b.setAttribute('aria-pressed',String(b.dataset.pickerFilter===pickerFilter));b.disabled=state.busy||state.phase!=='disconnected'});
 const fragment=document.createDocumentFragment();
 for(const p of matches){const row=document.createElement('div');row.className=`server-option${p.selected?' selected':''}`;row.append(profileChoice(p));fragment.append(row)}
 if(!matches.length){const empty=document.createElement('div');empty.className='server-picker-empty';const title=document.createElement('h3');title.textContent='Ничего не найдено';const text=document.createElement('p');text.textContent='Попробуйте другую страну, протокол или вкладку «Все».';const reset=document.createElement('button');reset.className='secondary';reset.textContent='Показать все серверы';reset.onclick=()=>{pickerFilter='all';$('#picker-search').value='';renderServerPicker();$('#picker-search').focus()};empty.append(title,text,reset);fragment.append(empty)}
 list.append(fragment);list.scrollTop=scroll;
}
$('#open-picker').onclick=()=>openServerPicker();$('#close-picker').onclick=()=>$('#profile-picker').close();
$('#picker-import').onclick=()=>{$('#profile-picker').close();openImport()};
$('#picker-search').addEventListener('input',()=>{$('#picker-list').scrollTop=0;renderServerPicker()});
$('#home-search').addEventListener('input',renderPicker);
$('#clear-profile-search').onclick=()=>{$('#profile-search').value='';renderProfiles();$('#profile-search').focus()};
$('#clear-picker-search').onclick=()=>{$('#picker-search').value='';renderServerPicker();$('#picker-search').focus()};
document.querySelectorAll('[data-library-filter]').forEach(b=>b.onclick=()=>{libraryFilter=b.dataset.libraryFilter;renderProfiles()});
document.querySelectorAll('[data-picker-filter]').forEach(b=>b.onclick=()=>{pickerFilter=b.dataset.pickerFilter;$('#picker-list').scrollTop=0;renderServerPicker()});
$('#picker-search').addEventListener('keydown',event=>{if(event.key==='Enter'){event.preventDefault();$('#picker-list .profile-choice:not(:disabled)')?.click()}});
// A pasted subscription belongs in Import. Do not retain its token in search.
for(const id of ['#profile-search','#picker-search','#home-search'])$(id).addEventListener('paste',event=>{
 const source=event.clipboardData?.getData('text').trim()||'';
 if(/^(https:\/\/|(?:vless|vmess|trojan|ss|tuic|hysteria2?|hy2):\/\/)/i.test(source)){
  event.preventDefault();$(id).value='';if($('#profile-picker').open)$('#profile-picker').close();openImport(source);
 }
});
function setSettings(s){const locked=s.busy||s.phase!=='disconnected';$('#settings-form').querySelectorAll('input,textarea,select,button').forEach(e=>{if(!e.closest('#turn-fields'))e.disabled=locked});$('#turn-fields').disabled=locked||$('#server-defaults').checked;if(settingsDirty)return;const v=s.turn;$('#server-defaults').checked=v.serverDefaults;for(const key of ['transport','preset','dns','relay','connections','mtu','pace','window','interval'])$(`#turn-${key}`).value=v[key];$('#turn-congestion').checked=v.congestion;$('#routing-mode').value=s.routing.mode||'inherit';$('#routing-include').value=s.routing.include||'';$('#routing-exclude').value=s.routing.exclude||'';$('#turn-fields').disabled=v.serverDefaults||s.phase!=='disconnected';$('#settings-form').querySelectorAll('input,textarea,select,button').forEach(e=>{if(!e.closest('#turn-fields'))e.disabled=s.busy||s.phase!=='disconnected'})}
$('#settings-form').addEventListener('input',()=>{settingsDirty=true;$('#turn-fields').disabled=$('#server-defaults').checked});$('#settings-form').addEventListener('submit',async e=>{e.preventDefault();const turn={serverDefaults:$('#server-defaults').checked,congestion:$('#turn-congestion').checked};for(const k of ['transport','preset','dns','relay'])turn[k]=$(`#turn-${k}`).value.trim();for(const k of ['connections','mtu','pace','window','interval'])turn[k]=Number($(`#turn-${k}`).value);const routing={mode:$('#routing-mode').value,include:$('#routing-include').value,exclude:$('#routing-exclude').value};if(await call('SaveSettings',turn,routing)){settingsDirty=false;toast('Настройки сохранены');const fresh=await api().Snapshot();render(fresh)}});$('#recover-button').onclick=()=>call('Recover');
document.querySelectorAll('[data-probe]').forEach(e=>e.onclick=()=>call('Probe',e.dataset.probe));
function renderDiagnostic(d){if(!d){$('#diagnostic-title').textContent='Начните с проверки';$('#diagnostic-description').textContent='Подключите профиль и выберите сервис выше.';$('#diagnostic-metrics').replaceChildren();return;}$('#diagnostic-title').textContent=d.ip||({google:'Google доступен',youtube:'YouTube доступен'}[d.kind]||'HTTPS доступен');$('#diagnostic-description').textContent=`Проверено ${new Date(d.checkedAt*1000).toLocaleTimeString('ru-RU')} · Маршруты текущего профиля`;const metrics=$('#diagnostic-metrics');metrics.replaceChildren();for(const [key,label] of [['dnsMs','DNS'],['tcpMs','TCP'],['tlsMs','TLS'],['responseMs','Ответ'],['totalMs','Всего']]){if(d[key]===undefined)continue;const span=document.createElement('span');span.textContent=label;const strong=document.createElement('strong');strong.textContent=`${d[key]} мс`;span.append(strong);metrics.append(span)}}
function renderTraffic(s){
 const total=(Number(s.stats?.rx)||0)+(Number(s.stats?.tx)||0);
 if(s.phase!=='connected'){trafficSamples=Array(30).fill(0);previousTraffic=null}else{const delta=previousTraffic===null?0:Math.max(0,total-previousTraffic);trafficSamples.push(delta);trafficSamples.shift();previousTraffic=total}
 const maximum=Math.max(1,...trafficSamples),d=trafficSamples.map((n,i)=>`${i?'L':'M'}${(i*360/29).toFixed(1)} ${(43-n/maximum*35).toFixed(1)}`).join(' ');
 $('.traffic-spark path').setAttribute('d',d);
}
function render(s){state=s;const selectedProfile=s.profiles.find(p=>p.selected);const selectionKey=selectedProfile?`${selectedProfile.index}:${selectedProfile.kind}:${selectedProfile.name}`:null;if(activeMode===null||selectedProfile&&lastSelected!==selectionKey){activeMode=profileMode(selectedProfile)}lastSelected=selectionKey;if(selectedProfile)rememberedProfiles[profileMode(selectedProfile)]=selectedProfile.index;const connected=s.phase==='connected',busy=['connecting','disconnecting'].includes(s.phase),active=connected||s.phase==='captcha'||s.phase==='connecting';$('#version').textContent=`BLINK ${s.version}`;$('#about-version').textContent=`Blink ${s.version}`;$('#architecture').textContent=s.arch==='386'?'32-bit · x86':'64-bit · x64';$('#profile-count').textContent=s.profiles.length;const top=$('.top-status');top.classList.toggle('connected',connected);$('#top-status').textContent=connected?'Соединение активно':busy?'Подключение…':s.phase==='captcha'?'Ожидает проверки':'Не подключено';const orbit=$('#power-button');orbit.classList.toggle('connected',connected);orbit.classList.toggle('busy',busy);orbit.disabled=s.phase==='disconnecting'||s.busy&&!active;orbit.setAttribute('aria-label',active?'Отключить':'Подключить');const pill=$('#connection-pill');pill.classList.toggle('active',connected);pill.replaceChildren();pill.append(document.createElement('span'),document.createTextNode(connected?'Соединение активно':busy?'Соединяем…':s.phase==='captcha'?'Проверка':'Ожидает подключения'));$('#connection-title').textContent=connected?'Можно просто быть онлайн':busy?'Уже почти на месте':s.phase==='captcha'?'Один небольшой шаг':'Ваш момент тишины';$('#connection-subtitle').textContent=connected?'Хорошее соединение — когда его не замечаешь.':busy?'Готовим ваш маршрут. Это займёт немного времени.':s.phase==='captcha'?'Нужна ручная проверка.':'Подключитесь, чтобы открыть больше.';$('#connect-button span:last-child').textContent=active?'Отключиться':modeProfiles().length?'Подключиться':'Добавить профиль';$('#connect-button').disabled=s.phase==='disconnecting'||s.busy&&!active;$('#message').textContent=productText(s.message);const p=modeProfiles().find(p=>p.selected);$('#selected-name').textContent=p?displayName(p.name):(activeMode==='turn'?'Выберите TURN-профиль':'Выберите VPN-профиль');$('#selected-kind').textContent=p?`${kinds[p.kind]||p.kind}${(p.expiresAt??s.expiresAt)?' · до '+new Date((p.expiresAt??s.expiresAt)*1000).toLocaleDateString('ru-RU'):''}`:'Всё нужное для первого подключения.';setAvatar($('#server-avatar'),p);$('#rx').innerHTML=formatBytes(s.stats?.rx);$('#tx').innerHTML=formatBytes(s.stats?.tx);$('#refresh-button').disabled=!s.hasSubscription||s.busy||active;$('#recover-button').disabled=s.busy||active;$('#add-profile').disabled=s.busy||active;document.querySelectorAll('[data-probe]').forEach(e=>e.disabled=!connected||s.busy);renderTraffic(s);renderPicker();renderProfiles();renderSubscriptions();renderConnectionProgress();if($('#profile-picker').open)renderServerPicker();setSettings(s);renderDiagnostic(s.diagnostic);renderCaptcha(s.captcha)}
function renderSubscriptions(){
 const list=$('#subscription-list');list.replaceChildren();const sources=state.sources||[];
 for(const source of sources){
  const card=document.createElement('article');card.className='subscription-card';
  const icon=document.createElement('span');icon.className='tile-icon';icon.innerHTML=svg(source.kind==='turn'?'route':'shield');
  const copy=document.createElement('div');copy.className='subscription-copy';const title=document.createElement('strong');title.textContent=`${source.kind==='turn'?'TURN':'Обычный VPN'} · ${productText(source.name)}`;
  const count=document.createElement('small');count.textContent=`Серверов: ${source.count}${source.expiresAt?' · до '+new Date(source.expiresAt*1000).toLocaleDateString('ru-RU'):''}`;copy.append(title,count);
  const refresh=document.createElement('button');refresh.className='icon-button';refresh.innerHTML=svg('refresh');refresh.setAttribute('aria-label',`Обновить ${source.kind==='turn'?'TURN':'VPN'} ${productText(source.name)}`);refresh.disabled=!source.refreshable||state.busy||state.phase!=='disconnected';refresh.onclick=()=>call('RefreshSource',source.id);
  const remove=document.createElement('button');remove.className='icon-button';remove.innerHTML=svg('trash');remove.setAttribute('aria-label',`Удалить источник ${productText(source.name)}`);remove.disabled=state.busy||state.phase!=='disconnected';remove.onclick=async()=>{if(confirm('Удалить эту подписку и только её серверы?'))await call('DeleteSource',source.id)};
  card.append(icon,copy,refresh,remove);list.append(card);
 }
 if(!sources.length){const hint=document.createElement('p');hint.className='field-help';hint.textContent='Добавьте обе подписки: обычный VPN и TURN. Они будут храниться рядом, без замены друг друга.';list.append(hint)}
}
function renderConnectionProgress(){
 if(!state)return;const trace=state.connection,element=$('#connection-progress');
 element.hidden=!trace?.startedAt;
 if(trace?.startedAt){const active=['connecting','captcha','connected','disconnecting'].includes(state.phase);const elapsed=active?Math.max(0,Math.floor((Date.now()-trace.startedAt)/1000)):(trace.elapsedSeconds||0);element.textContent=`${productText(trace.detail||'Подготовка соединения')} · ${elapsed} с`}
 if(trace?.error){$('#connection-error').hidden=false;$('#connection-error').textContent=`${productText(trace.error)}${trace.code?' ['+trace.code+']':''}`}else if(trace){$('#connection-error').hidden=true}
 $('#log-path').textContent=state.logPath||'%APPDATA%\\Blink\\logs\\blink.log';$('#log-error').textContent=state.logError||'';
 $('#report-stage').textContent=trace?.detail?productText(trace.detail):'Попробуйте подключиться';$('#copy-connection-report').disabled=!trace?.startedAt;
 const list=$('#connection-events');list.replaceChildren();
 for(const event of trace?.events||[]){const row=document.createElement('li');const time=document.createElement('time');time.textContent=`${(event.at/1000).toFixed(1)} с`;const text=document.createElement('span');text.textContent=productText(event.code);const fields=document.createElement('small');fields.textContent=Object.entries(event.fields||{}).map(([key,value])=>`${key}: ${value}`).join(' · ');row.append(time,text,fields);list.append(row)}
}
$('#copy-connection-report').onclick=async()=>{if(await call('CopyConnectionReport'))toast('Отчёт подключения скопирован')};
setInterval(()=>{if(state?.connection?.startedAt&&['connecting','captcha','disconnecting'].includes(state.phase))renderConnectionProgress()},1000);
// Manual challenge adapter for the current TURN provider.
let captchaWidget=null;
function closeCaptcha(){captchaWidget?.destroy?.();captchaWidget=null;$('#captcha-container').replaceChildren();if($('#captcha-dialog').open)$('#captcha-dialog').close();captchaID=''}
function renderCaptcha(c){if(!c){closeCaptcha();return}if(c.id===captchaID)return;closeCaptcha();captchaID=c.id;$('#captcha-dialog').showModal();$('#captcha-legacy').hidden=!!c.url;if(c.url){mountModernCaptcha(c)}else if(c.image){const img=document.createElement('img');img.alt='Проверочный код';img.referrerPolicy='no-referrer';img.src=c.image;$('#captcha-container').append(img);$('#captcha-help').textContent='Введите символы с изображения. Ответ действует только для текущей проверки.'}}
async function mountModernCaptcha(c){$('#captcha-help').textContent='Загружаем ручную проверку…';if(!window.BlinkCaptcha){$('#captcha-help').textContent='Не удалось загрузить SDK проверки. Попробуйте заново или отмените подключение.';return}try{captchaWidget=window.BlinkCaptcha.mount(c,$('#captcha-container'),async token=>{if(captchaID!==c.id)return;await call('SubmitCaptcha',c.id,'token',token)},message=>{$('#captcha-help').textContent=message});$('#captcha-help').textContent='Пройдите проверку вручную. Blink продолжит тот же запрос.'}catch(e){$('#captcha-help').textContent='Не удалось открыть проверку. Проверьте доступ к обычной сети.'}}
$('#cancel-captcha').onclick=()=>call('Disconnect');$('#captcha-dialog').addEventListener('cancel',e=>{e.preventDefault();call('Disconnect')});$('#captcha-retry').onclick=()=>{const c=state?.captcha;if(!c)return;captchaID='';renderCaptcha(c)};$('#captcha-legacy').addEventListener('submit',async e=>{e.preventDefault();const answer=$('#captcha-answer').value.trim();$('#captcha-answer').value='';if(captchaID)await call('SubmitCaptcha',captchaID,'key',answer)});
async function boot(){if(!api()){setTimeout(boot,80);return}window.runtime?.EventsOn('blink:state',render);try{render(await api().Snapshot())}catch(e){toast(e)}}boot();

$('#open-log-folder').onclick=()=>call('OpenLogFolder');
