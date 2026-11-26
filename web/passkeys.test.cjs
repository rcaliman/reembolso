const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const vm=require('node:vm');
const source=fs.readFileSync(__dirname+'/passkeys.js','utf8');
const settle=()=>new Promise(resolve=>setImmediate(resolve));
function node(){return {events:{},value:'',textContent:'',disabled:true,children:[],addEventListener(k,fn){this.events[k]=fn;},setAttribute(){},append(...els){this.children.push(...els);},replaceChildren(...els){this.children=els;}};}
function helpers(){const context=vm.createContext({document:{querySelector:()=>null},window:{},navigator:{},atob,btoa});vm.runInContext(source,context);return context.Passkeys;}
function credential(registration=false){return {id:'AQID',rawId:Uint8Array.from([1,2,3]).buffer,type:'public-key',getClientExtensionResults:()=>({}),
 response:registration?{clientDataJSON:Uint8Array.from([4,5]).buffer,attestationObject:Uint8Array.from([6,7]).buffer,getTransports:()=>['internal']}
 :{clientDataJSON:Uint8Array.from([4,5]).buffer,authenticatorData:Uint8Array.from([6,7]).buffer,signature:Uint8Array.from([8,9]).buffer,userHandle:Uint8Array.from([10,11]).buffer}};}
test('base64url preserva bytes binários inclusive respostas grandes',()=>{
 const P=helpers(),bytes=Uint8Array.from({length:64000},(_,i)=>i%256);
 const encoded=P.encode(bytes.buffer);assert.doesNotMatch(encoded,/[+/=]/);
 assert.deepEqual([...P.decode(encoded)],[...bytes]);
});
test('opções decodificam desafios, usuário e descritores sem alterar a resposta do servidor',()=>{
 const P=helpers(),data={publicKey:{challenge:'AQID',user:{name:'Ana',id:'BAU'},excludeCredentials:[{type:'public-key',id:'Bgc'}]}};
 const before=JSON.stringify(data),result=P.options(data,true);
 assert.deepEqual([...result.publicKey.challenge],[1,2,3]);assert.deepEqual([...result.publicKey.user.id],[4,5]);
 assert.deepEqual([...result.publicKey.excludeCredentials[0].id],[6,7]);assert.equal(JSON.stringify(data),before);
});
test('respostas incluem os dados assinados, handle e transportes nos campos WebAuthn',()=>{
 const P=helpers(),login=P.serialize(credential()),registration=P.serialize(credential(true));
 assert.equal(login.response.signature,'CAk');assert.equal(login.response.userHandle,'Cgs');
 assert.equal(registration.response.attestationObject,'Bgc');assert.deepEqual([...registration.response.transports],['internal']);
 assert.equal(login.rawId,'AQID');assert.throws(()=>P.serialize(null),/não foi concluída/);
});
async function loginView({secure=true,cancel=false}={}){
 const button=node(),help=node(),error=node(),calls=[];let requested=0,redirect;
 const elements={'#passkey-login':button,'#passkey-help':help,'#error':error};
 const context={document:{querySelector:s=>elements[s]||null},window:{isSecureContext:secure,PublicKeyCredential:function(){}},
  navigator:{credentials:{async get(options){requested++;assert.deepEqual([...options.publicKey.challenge],[1,2,3]);if(cancel){const failure=Error();failure.name='NotAllowedError';throw failure;}return credential();}}},
  atob,btoa,fetch:async(url,options)=>{calls.push({url,options});assert.equal(options.cache,'no-store');return {ok:true,json:async()=>url.endsWith('/config')?{enabled:true}:url.endsWith('/begin')?{publicKey:{challenge:'AQID'}}:{id:1}};},
  location:{replace(path){redirect=path;}}};
 vm.runInNewContext(source,context);await settle();return {button,help,error,calls,requested:()=>requested,redirect:()=>redirect};
}
test('login solicita confirmação somente após clique e envia a assinatura para o servidor',async()=>{
 const v=await loginView();assert.equal(v.requested(),0);assert.equal(v.button.disabled,false);
 await v.button.events.click();
 assert.equal(v.requested(),1);assert.equal(v.redirect(),'/pedidos');
 const finish=v.calls.find(call=>call.url.endsWith('/finish'));
 assert.equal(finish.options.method,'POST');assert.equal(finish.options.credentials,'same-origin');
 assert.equal(JSON.parse(finish.options.body).response.signature,'CAk');
});
test('cancelamento permite repetir e HTTP inseguro mantém a opção desabilitada',async()=>{
 const canceled=await loginView({cancel:true});await canceled.button.events.click();
 assert.equal(canceled.redirect(),undefined);assert.equal(canceled.button.disabled,false);
 assert.match(canceled.error.textContent,/cancelada/);assert.equal(canceled.calls.some(call=>call.url.endsWith('/finish')),false);
 const insecure=await loginView({secure:false});assert.equal(insecure.button.disabled,true);assert.equal(insecure.calls.length,0);assert.match(insecure.help.textContent,/HTTPS/);
});

test('cadastro confirma senha, limpa o campo e só salva após confirmação no aparelho',async()=>{
 const form=node(),button=node(),password=node(),name=node(),list=node(),help=node(),error=node(),status=node(),calls=[];
 form.elements={nome:name};name.value='Meu celular';password.value='senha-atual';form.reset=()=>{name.value='';password.value='';};
 const elements={'#passkey-register':form,'#passkey-add':button,'#passkey-password':password,'#passkey-list':list,'#passkey-help':help,'#passkey-error':error,'#passkey-status':status};
 let confirmed=false;
 const context={document:{querySelector:s=>elements[s]||null,createElement:node},window:{isSecureContext:true,PublicKeyCredential:function(){}},navigator:{credentials:{async create(input){assert.equal(password.value,'');assert.deepEqual([...input.publicKey.user.id],[4,5]);confirmed=true;return credential(true);}}},
  fetch:async(url,options)=>{calls.push({url,options});if(url.endsWith('/finish'))assert.equal(confirmed,true);return {ok:true,json:async()=>url.endsWith('/config')?{enabled:true}:url.endsWith('/begin')?{publicKey:{challenge:'AQID',user:{id:'BAU'}}}:url==='/api/passkeys'?[]:{ok:true}};},atob,btoa};
 vm.runInNewContext(source,context);await settle();assert.equal(button.disabled,false);
 await form.events.submit({preventDefault(){}});
 const begin=calls.find(call=>call.url.endsWith('/begin'));
 assert.deepEqual(JSON.parse(begin.options.body),{nome:'Meu celular',senha:'senha-atual'});
 const finish=calls.find(call=>call.url.endsWith('/finish'));
 assert.equal(JSON.parse(finish.options.body).response.attestationObject,'Bgc');
 assert.equal(password.value,'');assert.equal(button.disabled,false);assert.match(status.textContent,/Passkey cadastrada/);
});
