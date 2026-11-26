const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const vm=require('node:vm');
function element(tag){return {tag,children:[],events:{},attributes:{},append(...els){this.children.push(...els);for(const el of els)el.parentNode=this;},insertBefore(el,before){const index=this.children.indexOf(before);if(index<0)this.append(el);else{this.children.splice(index,0,el);el.parentNode=this;}},querySelector(selector){return this.children.find(el=>selector[0]==='.'?el.className===selector.slice(1):el.tag===selector);},showModal(){this.open=true;},close(){this.open=false;},prepend(...els){this.children.unshift(...els);},setAttribute(k,v){this.attributes[k]=v;},addEventListener(k,v){this.events[k]=v;},remove(){this.removed=true;}};}
function all(el,tag){return [...(el.tag===tag?[el]:[]),...el.children.flatMap(child=>all(child,tag))];}
const settle=()=>new Promise(resolve=>setImmediate(resolve));
async function view({secure=true,ios=false,standalone=false,waiting=false}={}){
 const main=element('main'),menu=element('div'),body=element('body'),events={},swEvents={},registrationEvents={},messages=[];
 const logout=element('button');logout.className='user-nav-logout';menu.append(logout);body.append(menu,main);
 const registration={waiting:waiting?{postMessage:message=>messages.push(message)}:null,addEventListener:(k,v)=>registrationEvents[k]=v};
 let reloads=0,registrations=0;
 const context={
  document:{body,querySelector(selector){if(selector==='main')return main;if(selector==='#user-nav-links')return menu;return all(main,'div').find(el=>el.id==='pwa-update'&&!el.removed);},createElement:element},
  navigator:{userAgent:ios?'iPhone':'Browser',serviceWorker:{controller:{},addEventListener:(k,v)=>swEvents[k]=v,async register(url,options){assert.equal(url,'/sw.js');assert.equal(options.updateViaCache,'none');registrations++;return registration;}}},
  window:{sessionReady:Promise.resolve(),isSecureContext:secure,matchMedia:()=>({matches:standalone}),addEventListener:(k,v)=>events[k]=v},
  location:{reload(){reloads++;}},console
 };
 vm.runInNewContext(fs.readFileSync(__dirname+'/pwa.js','utf8'),context);
 await settle();
 return {main,menu,body,events,swEvents,messages,registration,registrationEvents,registrations:()=>registrations,reloads:()=>reloads};
}
test('HTTP inseguro orienta usar HTTPS e não tenta registrar o service worker',async()=>{
 const v=await view({secure:false});assert.equal(v.registrations(),0);
 assert.equal(all(v.main,'button').length,0);
 await all(v.menu,'button')[0].events.click();
 assert.match(all(v.body,'dialog')[0].querySelector('p').textContent,/HTTPS/);
});
test('iOS recebe instruções e o app já instalado não mostra convite',async()=>{
 const ios=await view({ios:true});
 assert.equal(all(ios.body,'dialog').length,0);
 await all(ios.menu,'button')[0].events.click();
 const dialog=all(ios.body,'dialog')[0];assert.match(dialog.querySelector('p').textContent,/Safari/);
 assert.equal(dialog.open,true);dialog.querySelector('button').events.click();assert.equal(dialog.open,false);
 const installed=await view({standalone:true});assert.equal(all(installed.menu,'button')[0].hidden,true);
 assert.equal(installed.registrations(),1);
});
test('instalação depende de gesto do usuário e some após instalar',async()=>{
 const v=await view();let prompted=0;
 v.events.beforeinstallprompt({preventDefault(){},async prompt(){prompted++;v.events.appinstalled();},userChoice:Promise.resolve({outcome:'accepted'})});
 const button=all(v.menu,'button')[0];assert.equal(button.hidden,false);assert.equal(prompted,0);
 await button.events.click();assert.equal(prompted,1);assert.equal(button.hidden,true);
 assert.equal(all(v.body,'dialog').length,0);
});
test('nova versão só ativa e recarrega depois de clicar em atualizar',async()=>{
 const v=await view({waiting:true});
 const notice=all(v.main,'div').find(el=>el.id==='pwa-update');
 assert.ok(notice);assert.match(all(notice,'p')[0].textContent,/Salve/);
 v.swEvents.controllerchange();assert.equal(v.reloads(),0);assert.equal(v.messages.length,0);
 all(notice,'button')[0].events.click();assert.equal(v.messages[0].type,'ACTIVATE_UPDATE');
 v.swEvents.controllerchange();assert.equal(v.reloads(),1);
 v.swEvents.controllerchange();assert.equal(v.reloads(),1);
});
