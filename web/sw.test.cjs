const {test}=require('node:test');
const assert=require('node:assert/strict');
const vm=require('node:vm');
const fs=require('node:fs');
function worker(){
 const events={},stores=new Map();let skip=0,claimed=0,offline=false;
 const cache=name=>{
  if(!stores.has(name))stores.set(name,new Map());
  const values=stores.get(name);
  return {async addAll(requests){for(const req of requests){assert.equal(req.credentials,'omit');values.set(new URL(req.url).pathname,{body:new URL(req.url).pathname});}},async match(url){return values.get(url);}};
 };
 class Request{constructor(url,options={}){this.url=new URL(url,'https://reembolso.example').href;Object.assign(this,options);}}
 vm.runInNewContext(fs.readFileSync(__dirname+'/sw.js','utf8'),{
  self:{location:{origin:'https://reembolso.example'},clients:{async claim(){claimed++;}},async skipWaiting(){skip++;},addEventListener:(name,fn)=>events[name]=fn},
  caches:{async open(name){return cache(name);},async keys(){return [...stores.keys()];},async delete(name){return stores.delete(name);}},
  Request,URL,fetch:async request=>{if(offline)throw Error('offline');return {body:'network:'+request.url};}
 });
 const lifecycle=async name=>{let waiting;events[name]({waitUntil(p){waiting=p;}});await waiting;};
 async function fetchRequest(path,{method='GET',mode='navigate'}={}){
  let response;
  events.fetch({request:{url:new URL(path,'https://reembolso.example').href,method,mode},respondWith(p){response=p;}});
  return response ? await response : undefined;
 }
 return {events,stores,lifecycle,fetchRequest,offline:()=>offline=true,skips:()=>skip,claimed:()=>claimed};
}
test('cache offline contém somente arquivos públicos e atualização espera confirmação',async()=>{
 const sw=worker();await sw.lifecycle('install');
 const cached=[...sw.stores.values()][0];
 assert.deepEqual([...cached.keys()].sort(),['/offline.html','/icon-192.png','/icon-512.png','/icon-maskable-512.png','/apple-touch-icon.png'].sort());
 assert.equal(sw.skips(),0);
 let pending;sw.events.message({data:{type:'ACTIVATE_UPDATE'},waitUntil(p){pending=p;}});await pending;
 assert.equal(sw.skips(),1);
});
test('navegação usa a rede e recorre à página pública offline sem guardar pedidos',async()=>{
 const sw=worker();await sw.lifecycle('install');
 const cached=[...sw.stores.values()][0],count=cached.size;
 assert.match((await sw.fetchRequest('/pedidos?ano=2026')).body,/network:/);
 assert.equal(cached.size,count);
 sw.offline();
 assert.equal((await sw.fetchRequest('/pedidos?ano=2026')).body,'/offline.html');
 assert.equal((await sw.fetchRequest('/icon-192.png',{mode:'no-cors'})).body,'/icon-192.png');
 assert.equal(cached.size,count);
});
test('API, notas, outros domínios e gravações nunca são interceptados nem reenviados',async()=>{
 const sw=worker();await sw.lifecycle('install');sw.offline();
 for(const path of ['/api/me','/api/pedidos','/api/pedidos/1/nota','/api/pedidos/1/nota/paginas/1','https://outro.example/']) {
  assert.equal(await sw.fetchRequest(path),undefined);
 }
 for(const method of ['POST','PATCH','DELETE']) assert.equal(await sw.fetchRequest('/pedidos',{method}),undefined);
 assert.equal(await sw.fetchRequest('/session.js',{mode:'same-origin'}),undefined);
});
test('ativação remove somente versões anteriores desta PWA',async()=>{
 const sw=worker();await sw.lifecycle('install');
 const current=[...sw.stores.keys()][0];
 sw.stores.set('reembolso-pwa-antigo',new Map());sw.stores.set('outro-aplicativo',new Map());
 await sw.lifecycle('activate');
 assert.deepEqual([...sw.stores.keys()].sort(),[current,'outro-aplicativo'].sort());
 assert.equal(sw.claimed(),1);
});
