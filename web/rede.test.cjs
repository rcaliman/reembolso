const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const vm=require('node:vm');
function element(tag,text='') {
 return {tag,textContent:text,children:[],events:{},attributes:{},value:'',disabled:true,
  get options(){return this.children;},
  append(...els){this.children.push(...els);},replaceChildren(...els){this.children=els;},
  setAttribute(k,v){this.attributes[k]=v;},addEventListener(k,v){this.events[k]=v;}};
}
function all(el,tag){return [...(el.tag===tag?[el]:[]),...el.children.flatMap(c=>all(c,tag))];}
function text(el){return [el.textContent,...el.children.map(text)].join(' ');}
const settle=()=>new Promise(resolve=>setImmediate(resolve));
const provider={nome:'<Clínica>',nome_fantasia:'Clínica A',cidade:'COLATINA',uf:'ES',tipo_documento:'CNPJ',documento:'123',endereco:'Rua A | Centro',especialidades:['CARDIOLOGIA'],areas_atuacao:[],telefones:['2737170172'],mapa_url:'javascript:alert(1)',telefones_web:[{telefone:'27998091410',confianca:'alta',comparacao_banescaixa:'divergente',observacao:'Número adicional',fonte_url:'https://example.com/contato',consultado_em:'2026-11-26T12:00:00Z',metodo:'pagina_consultada'}]};
async function view(search='',customAPI) {
 const els=Object.fromEntries(['cidade','especialidade','rede-resultados','rede-status','error','buscar','limpar','rede-filtros'].map(id=>['#'+id,element('div')]));
 els['#cidade'].append(Object.assign(element('option','Todas'),{value:''}));
 els['#especialidade'].append(Object.assign(element('option','Todas'),{value:''}));
 const requests=[], history=[];
 const context={document:{querySelector:s=>els[s],createElement:element},window:{sessionReady:Promise.resolve()},location:{search},history:{replaceState(a,b,url){history.push(url);}},URL,URLSearchParams,Date,
 api:async url=>{requests.push(url);return customAPI ? customAPI(url,requests.length) : {cidades:['COLATINA','VITORIA'],especialidades:['CARDIOLOGIA'],prestadores:[provider],total:1};}};
 vm.runInNewContext(fs.readFileSync(__dirname+'/rede.js','utf8'),context);await settle();
 return {els,requests,history};
}
test('restaura filtros da URL e mantém fontes e números originais separados',async()=>{
 const v=await view('?cidade=COLATINA&especialidade=CARDIOLOGIA');
 assert.match(v.requests[0],/cidade=COLATINA&especialidade=CARDIOLOGIA/);
 assert.equal(v.els['#cidade'].options.length,3);
 assert.equal(v.els['#cidade'].value,'COLATINA');
 const cards=all(v.els['#rede-resultados'],'article');assert.equal(cards.length,1);
 assert.match(text(cards[0]),/Telefones da Banescaixa/);assert.match(text(cards[0]),/Contatos encontrados na web/);
 assert.equal(all(cards[0],'h3')[0].textContent,'<Clínica>');
 const links=all(cards[0],'a');assert(links.some(a=>a.href==='tel:+552737170172'));assert(links.some(a=>a.href==='https://example.com/contato'));
 assert(!links.some(a=>a.href?.startsWith('javascript:')));
 assert.equal(v.els['#rede-resultados'].attributes['aria-busy'],'false');
});
test('pesquisa combinada e limpeza de filtros',async()=>{
 const v=await view();v.els['#cidade'].value='COLATINA';v.els['#especialidade'].value='CARDIOLOGIA';
 v.els['#rede-filtros'].events.submit({preventDefault(){}});await settle();
 assert.match(v.requests.at(-1),/cidade=COLATINA&especialidade=CARDIOLOGIA/);
 v.els['#limpar'].events.click();await settle();assert.equal(v.requests.at(-1),'/api/rede-credenciada?');
});
test('respostas antigas não substituem a consulta mais recente',async()=>{
 let resolveOld;
 const v=await view('',async(url,n)=> n===2 ? new Promise(r=>resolveOld=r) : {cidades:['COLATINA'],especialidades:[],prestadores:[],total:0});
 v.els['#cidade'].value='COLATINA';v.els['#rede-filtros'].events.submit({preventDefault(){}});
 v.els['#limpar'].events.click();await settle();
 resolveOld({prestadores:[provider],total:1});await settle();
 assert.equal(all(v.els['#rede-resultados'],'article').length,0);
 assert.match(v.els['#rede-status'].textContent,/0 prestadores/);
});
test('falha inicial oferece mensagem e permite tentar novamente',async()=>{
 const v=await view('',async()=>{throw Error('Serviço indisponível');});
 assert.equal(v.els['#error'].textContent,'Serviço indisponível');assert.equal(v.els['#buscar'].disabled,false);
});
test('números incompletos e suspeitos ficam intactos e sem discagem',async()=>{
 const raw=['2799629','289996366','270800000861','279957309','272472121002'];
 const p={...provider,telefones:raw,telefones_web:[{...provider.telefones_web[0],telefone:'2799629'}]};
 const v=await view('',async()=>({cidades:['COLATINA'],especialidades:[],prestadores:[p],total:1}));
 const result=v.els['#rede-resultados'];
 assert.equal(all(result,'a').filter(a=>a.href?.startsWith('tel:')).length,0);
 for (const value of raw) assert(text(result).includes(value));
 assert.match(text(result),/Formato não reconhecido/);
});
test('fixos, celulares e 0800 completos têm discagem apropriada; baixa confiança fica sem link',async()=>{
 const p={...provider,telefones:['2737170172','27998091410','08001234567'],telefones_web:[{...provider.telefones_web[0],confianca:'baixa'}]};
 const v=await view('',async()=>({cidades:['COLATINA'],especialidades:[],prestadores:[p],total:1}));
 const links=all(v.els['#rede-resultados'],'a').filter(a=>a.href?.startsWith('tel:'));
 assert.deepEqual(links.map(a=>a.href),['tel:+552737170172','tel:+5527998091410','tel:08001234567']);
 assert.match(text(v.els['#rede-resultados']),/0800 123 4567/);
 assert.equal(all(all(v.els['#rede-resultados'],'details')[0],'a').filter(a=>a.href?.startsWith('tel:')).length,0);
});
