'use strict';
// Pinned official SDK validates postMessage origin and source against its own iframe.
// Only a manual, visible challenge is used. Tokens remain memory-only.
window.BlinkCaptcha={mount(challenge,container,onSuccess,onError){
 const link=new URL(challenge.url);
 const host=link.hostname.toLowerCase();
 if(link.protocol!=='https:'||link.username||link.password||link.port&&link.port!=='443'||!(host==='vk.com'||host==='vk.ru'||host.endsWith('.vk.com')||host.endsWith('.vk.ru'))||link.hash){throw new Error('Invalid CAPTCHA origin')}
 if(challenge.expires_at*1000<=Date.now())throw new Error('Expired CAPTCHA');
 const widget=new window.VKIDCaptchaSDK.CaptchaWidget();
 let active=true,submitted=false;
 const stop=()=>{active=false;widget.destroy();container.replaceChildren()};
 widget.show({link:link.href,container,view:'block',scheme:'dark',lang:'ru',onClose:()=>{if(active)onError('Проверка закрыта. Можно открыть её заново или отменить подключение.')}}).then(token=>{
  if(!active||submitted||challenge.expires_at*1000<=Date.now())return;
  if(typeof token!=='string'||!token.length||token.length>16384||/[\r\n\0]/.test(token)){onError('Сервис проверки вернул некорректный ответ');return}
  submitted=true;onSuccess(token);
 }).catch(()=>{if(active)onError('Проверка не завершена. Повторите её или отмените подключение.')});
 return {destroy:stop};
}};
