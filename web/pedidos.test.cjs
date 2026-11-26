const {test} = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
function element(tag, text) {
  return {tag, textContent:text || '', children:[], dataset:{}, value:'', events:{}, attributes:{}, isConnected:true,
    append(...els) {this.children.push(...els);}, prepend(...els) {this.children.unshift(...els);}, replaceChildren(...els) {this.children=els;},
    setAttribute(name,value) {this.attributes[name]=value;}, addEventListener(name, fn) {this.events[name]=fn;},
    focus(){this.focused=true;},
    setCustomValidity(message) {this.validationMessage=message;}, reportValidity() {},
    querySelectorAll(selector) {return this.children.flatMap(child => [
      ...(selector==='[data-year]' && child.dataset.year ? [child] : []), ...child.querySelectorAll(selector)
    ]);}
  };
}
function all(el,tag) {return [ ...(el.tag===tag ? [el] : []), ...el.children.flatMap(child=>all(child,tag)) ];}
function text(el) {return [el.textContent,...el.children.map(text)].join(' ');}
const settle = () => new Promise(resolve=>setImmediate(resolve));
async function view({year = 2026, search = '', singleYear = false, onRead, onSave, onDelete, confirm = true, role = 'admin'} = {}) {
 const elements=Object.fromEntries(['solicitante','dependente','pedidos','resumo','error','limpar','anos','pedido-status'].map(id=>['#'+id,element('div')]));
 const person=(nome,inscricao)=>({nome,inscricao});
 const rows=[
  {id:3,data:'2026-01-02',valor:'0.20',reembolso_versao:0,reembolsado_centavos:null,solicitante:person('Ana','01'),dependente:person('João','02')},
  {id:2,data:'2026-01-01',valor:'0.10',reembolso_versao:0,reembolsado_centavos:0,solicitante:person('Ana','01'),dependente:null},
  {id:1,data:'2025-12-31',valor:'100.00',reembolso_versao:0,reembolsado_centavos:5000,solicitante:person('Bia','03'),dependente:person('João','02')}
 ];
 if (singleYear) rows.splice(0, 2);
 const requests=[], reads=[], events={};
 const location={search};
 const urls=[search || '/pedidos']; let position=0;
 const navigate=url=>{location.search=new URL(url,'http://localhost').search;};
 const history={
  pushState(_state,_title,url){urls.splice(++position);urls.push(url);navigate(url);},
  replaceState(_state,_title,url){urls[position]=url;navigate(url);},
  back(){if(position>0){navigate(urls[--position]);events.popstate();}},
  forward(){if(position<urls.length-1){navigate(urls[++position]);events.popstate();}}
 };
 const context=vm.createContext({Date:class extends Date { getFullYear() {return year;} }, URLSearchParams, location, history,
  window:{sessionReady:Promise.resolve({perfil:role}),confirm:()=>confirm,addEventListener:(name,fn)=>{events[name]=fn;}},
  document:{querySelector:s=>elements[s],createElement:element},
  Option:function(label,value){const el=element('option',label);el.value=value;return el;},
  api:async(url,options)=>{
   if(!options){
    reads.push(url);
    if(url==='/api/pedidos/filtros') {
     const people=role=>[...new Map(rows.filter(p=>p[role]).map(p=>[JSON.stringify(p[role]),p[role]])).values()];
     return {anos:[...new Set(rows.map(p=>p.data.slice(0,4)))],solicitante:people('solicitante'),dependente:people('dependente')};
    }
    const data=JSON.parse(JSON.stringify(rows.filter(p=>p.data.startsWith(new URL(url,'http://localhost').searchParams.get('ano')))));
    return onRead ? onRead(url,data) : data;
   }
   requests.push({url,options});
   if(options.method==='DELETE') {
    if(onDelete) await onDelete(url,options);
    rows.splice(rows.findIndex(p=>p.id===Number(url.split('/')[3])),1);
    return {ok:true};
   }
   if(onSave) return onSave(url,options);
   const value=options.data.valor_reembolsado;
   const cents=value==='' ? null : Math.round(Number(value)*100);
   const row=rows.find(p=>p.id===Number(url.split('/')[3]));
   assert.equal(options.data.versao,row.reembolso_versao);
   row.observacoes=options.data.observacoes; row.reembolsado_centavos=cents; row.reembolso_versao++;
   return {observacoes:row.observacoes,reembolsado_centavos:cents,reembolso_versao:row.reembolso_versao};
  },
  jsonRequest:(method,data)=>({method,data})});
 vm.runInContext(fs.readFileSync(__dirname+'/validation.js','utf8'),context);
 vm.runInContext(fs.readFileSync(__dirname+'/pedidos.js','utf8'),context);
 await new Promise(resolve=>setImmediate(resolve));
 return {elements,requests,reads,location,history};
}
test('anos, centavos exatos, filtros combinados e opção sem dependente',async()=>{
 const {elements:e}=await view();
 assert.deepEqual(all(e['#pedidos'],'h2').map(el=>el.textContent),['2026']);
 assert.match(text(e['#resumo']),/0,30/); assert.match(text(e['#resumo']),/0,00/);
 assert.deepEqual(all(e['#anos'],'a').map(el=>el.textContent),['2026','2025']);
 assert.match(text(e['#resumo']),/1 com reembolso não informado/);
 e['#solicitante'].value=JSON.stringify(['01','Ana']);e['#solicitante'].events.change();
 assert.deepEqual(all(e['#pedidos'],'h2').map(el=>el.textContent),['2026']);
 assert.match(text(e['#resumo']),/0,30/);
 e['#dependente'].value=JSON.stringify(['02','João']);e['#dependente'].events.change();
 assert.match(text(e['#resumo']),/1 pedido/); assert.match(text(e['#resumo']),/0,20/);
 e['#dependente'].value='sem-dependente';e['#dependente'].events.change();
 assert.match(text(e['#resumo']),/0,10/);
 e['#solicitante'].value=JSON.stringify(['03','Bia']);e['#solicitante'].events.change();
 assert.match(text(e['#pedidos']),/Nenhum pedido encontrado/);
 e['#limpar'].events.click();assert.match(text(e['#resumo']),/2 pedido/);
});
test('valor em reais é salvo pela API e atualiza os totais',async()=>{
 const {elements:e,requests}=await view();
 const form=all(e['#pedidos'],'form')[0], input=all(form,'input')[1];
 input.value='R$ 123,45';await form.events.submit({preventDefault(){}});
 assert.deepEqual(JSON.parse(JSON.stringify(requests[0])),{url:'/api/pedidos/3/reembolso',options:{method:'PATCH',data:{valor_reembolsado:'123.45',observacoes:'',versao:0}}});
 assert.match(text(e['#resumo']),/123,45/);assert.match(text(form),/Valor salvo/);
 assert.equal(input.disabled,false);
 input.value='-1';await form.events.submit({preventDefault(){}});
 assert.equal(requests.length,1);assert.match(input.validationMessage,/Informe/);
});

test('links trocam o ano, mantêm filtros e atualizam os totais',async()=>{
 const {elements:e}=await view();
 e['#dependente'].value=JSON.stringify(['02','João']);e['#dependente'].events.change();
 all(e['#anos'],'a').find(el=>el.textContent==='2025').events.click({preventDefault(){}});await settle();
 assert.deepEqual(all(e['#pedidos'],'h2').map(el=>el.textContent),['2025']);
 assert.equal(e['#dependente'].value,JSON.stringify(['02','João']));
 assert.match(text(e['#resumo']),/100,00/);assert.match(text(e['#resumo']),/50,00/);
 all(e['#anos'],'a').find(el=>el.textContent==='2026').events.click({preventDefault(){}});await settle();
 assert.match(text(e['#resumo']),/0,20/);assert.match(text(e['#resumo']),/1 pedido/);
});
test('ano corrente sem pedidos continua sendo a página inicial',async()=>{
 const {elements:e}=await view({year:2027});
 assert.deepEqual(all(e['#pedidos'],'h2').map(el=>el.textContent),['2027']);
 assert.match(text(e['#pedidos']),/Nenhum pedido encontrado em 2027/);
 assert.deepEqual(all(e['#anos'],'a').map(el=>el.textContent),['2027','2026','2025']);
});
test('único ano disponível e link direto para outro ano',async()=>{
 const single=await view({singleYear:true});
 assert.deepEqual(all(single.elements['#pedidos'],'h2').map(el=>el.textContent),['2025']);
 assert.equal(all(single.elements['#anos'],'a').length,0);
 const direct=await view({search:'?ano=2025'});
 assert.deepEqual(all(direct.elements['#pedidos'],'h2').map(el=>el.textContent),['2025']);
 const invalid=await view({search:'?ano=invalido'});
 assert.deepEqual(all(invalid.elements['#pedidos'],'h2').map(el=>el.textContent),['2026']);
});

test('rascunhos sobrevivem aos filtros e à troca de ano; totais usam apenas valores salvos',async()=>{
 const {elements:e}=await view();
 let input=all(e['#pedidos'],'input')[1];input.value='123,45';input.events.input();
 e['#dependente'].value='sem-dependente';e['#dependente'].events.change();
 e['#limpar'].events.click();
 assert.equal(all(e['#pedidos'],'input')[1].value,'123,45');
 assert.match(text(e['#resumo']),/0,00/);
 all(e['#anos'],'a').find(el=>el.textContent==='2025').events.click({preventDefault(){}});await settle();
 all(e['#anos'],'a').find(el=>el.textContent==='2026').events.click({preventDefault(){}});await settle();
 assert.equal(all(e['#pedidos'],'input')[1].value,'123,45');
 assert.equal(all(e['#pedidos'],'strong').length,0); // Totais aparecem somente no resumo.
});
test('URL, voltar e avançar acompanham o ano; busca pede apenas o ano escolhido',async()=>{
 const {elements:e,location,history,reads}=await view();
 assert.equal(location.search,'?ano=2026');
 assert.deepEqual(reads,['/api/pedidos/filtros','/api/pedidos?ano=2026']);
 all(e['#anos'],'a').find(el=>el.textContent==='2025').events.click({preventDefault(){}});await settle();
 assert.equal(location.search,'?ano=2025');
 history.back();await settle();
 assert.deepEqual(all(e['#pedidos'],'h2').map(el=>el.textContent),['2026']);
 history.forward();await settle();
 assert.deepEqual(all(e['#pedidos'],'h2').map(el=>el.textContent),['2025']);
 let prevented=false;
 all(e['#anos'],'a')[0].events.click({ctrlKey:true,preventDefault(){prevented=true;}});
 assert.equal(prevented,false);
});
test('erro ao salvar preserva o rascunho para nova tentativa e permite limpar valor',async()=>{
 let fail=true;
 const {elements:e}=await view({onSave:async()=>{
  if(fail) throw Error('Falha de conexão');
  return {reembolsado_centavos:null,reembolso_versao:1};
 }});
 let form=all(e['#pedidos'],'form')[0],input=all(form,'input')[1];
 input.value='12,34';input.events.input();await form.events.submit({preventDefault(){}});
 assert.match(text(form),/Falha de conexão/);assert.equal(input.disabled,false);
 e['#limpar'].events.click();form=all(e['#pedidos'],'form')[0];input=all(form,'input')[1];
 assert.equal(input.value,'12,34');
 fail=false;input.value='';input.events.input();await form.events.submit({preventDefault(){}});
 assert.equal(input.value,'');assert.match(text(form),/Observações salvas/);
});
test('resposta atrasada de outro ano não substitui o ano selecionado',async()=>{
 let release;
 const {elements:e,location}=await view({onRead:(url,data)=>url.endsWith('2025') ? new Promise(resolve=>{release=()=>resolve(data);}) : data});
 all(e['#anos'],'a').find(el=>el.textContent==='2025').events.click({preventDefault(){}});await settle();
 all(e['#anos'],'a').find(el=>el.textContent==='2026').events.click({preventDefault(){}});await settle();
 release();await settle();
 assert.equal(location.search,'?ano=2026');
 assert.deepEqual(all(e['#pedidos'],'h2').map(el=>el.textContent),['2026']);
 assert.match(text(e['#resumo']),/0,30/);
});

test('gravação em andamento continua protegida ao recriar a tabela',async()=>{
 let resolveSave;
 const {elements:e,requests}=await view({onSave:()=>new Promise(resolve=>{resolveSave=resolve;})});
 const oldForm=all(e['#pedidos'],'form')[0],input=all(oldForm,'input')[1];
 input.value='123,45';input.events.input();
 const pending=oldForm.events.submit({preventDefault(){}});
 oldForm.isConnected=false;
 e['#limpar'].events.click();
 const newForm=all(e['#pedidos'],'form')[0];
 assert.equal(all(newForm,'input')[1].disabled,true);
 assert.equal(all(newForm,'input')[0].disabled,true);
 await newForm.events.submit({preventDefault(){}});assert.equal(requests.length,1);
 resolveSave({reembolsado_centavos:12345,reembolso_versao:1});await pending;
 assert.equal(all(e['#pedidos'],'input')[1].disabled,false);
 assert.equal(all(e['#pedidos'],'input')[1].value,'123,45');
 assert.match(text(e['#resumo']),/123,45/);
});
test('leitura iniciada antes de salvar não restaura o valor anterior',async()=>{
 let resolveSave,resolveRead,delayRead=false;
 const {elements:e}=await view({
  onSave:()=>new Promise(resolve=>{resolveSave=resolve;}),
  onRead:(url,rows)=>delayRead && url.endsWith('2026') ? new Promise(resolve=>{resolveRead=()=>resolve(rows);}) : rows
 });
 const form=all(e['#pedidos'],'form')[0],input=all(form,'input')[1];
 input.value='123,45';input.events.input();const pending=form.events.submit({preventDefault(){}});
 form.isConnected=false;
 all(e['#anos'],'a').find(el=>el.textContent==='2025').events.click({preventDefault(){}});await settle();
 delayRead=true;
 all(e['#anos'],'a').find(el=>el.textContent==='2026').events.click({preventDefault(){}});await settle();
 resolveSave({reembolsado_centavos:12345,reembolso_versao:1});await pending;
 resolveRead();await settle();
 assert.equal(all(e['#pedidos'],'input')[1].value,'123,45');
 assert.match(text(e['#resumo']),/123,45/);
});

test('cartões mantêm cabeçalhos acessíveis e um único formulário por pedido',async()=>{
 const {elements:e}=await view();
 const table=all(e['#pedidos'],'table')[0];
 assert.equal(table.attributes.role,'table');
 const headers=all(table,'th'), groups=all(table,'tbody');
 assert.equal(headers.length,5);assert.equal(groups.length,2);
 for (const group of groups) {
  const [row,editRow]=group.children;
  assert.equal(group.children.length,2);
  assert.equal(row.attributes.role,'row');
  for (const [index,cell] of row.children.entries()) {
   assert.equal(cell.attributes.role,'cell');
   assert.equal(cell.attributes.headers,headers[index].id);
  }
  assert.equal(editRow.children[0].colSpan,5);
  assert.equal(all(row,'form').length,0);
  assert.equal(all(editRow,'form').length,1);
  const inputs=all(editRow,'input');
  assert.equal(inputs.length,2);assert.equal(inputs[0].type,'text');
  assert.match(inputs[0].attributes['aria-label'],/Observações do pedido/);
  assert.match(inputs[1].attributes['aria-label'],/Valor reembolsado/);
  assert.match(all(editRow,'button')[0].attributes['aria-label'],/Salvar reembolso do pedido/);
  const labels=all(row,'span').filter(el=>el.className==='cell-label');
  assert.deepEqual(labels.map(el=>el.textContent),['Solicitante','Dependente','Valor total']);
  assert.equal(labels.every(el=>el.attributes['aria-hidden']==='true'),true);
 }
});

test('conflito preserva rascunho e exige revisão antes de nova gravação',async()=>{
 const {elements:e,requests}=await view({onSave:async()=>{
  const error=Error('Alterado em outra sessão');error.status=409;
  error.data={observacoes:'Retornar amanhã',reembolsado_centavos:4200,reembolso_versao:1};throw error;
 }});
 let form=all(e['#pedidos'],'form')[0],input=all(form,'input')[1];
 input.value='12,34';input.events.input();await form.events.submit({preventDefault(){}});
 form=all(e['#pedidos'],'form')[0];input=all(form,'input')[1];
 assert.equal(input.value,'12,34');assert.match(text(form),/42,00/);
 assert.match(text(form),/Retornar amanhã/);
 assert.equal(all(form,'input')[0].readOnly,true);
 assert.equal(all(form,'button')[0].disabled,true);
 await form.events.submit({preventDefault(){}});assert.equal(requests.length,1);
 all(form,'button').find(b=>b.textContent==='Usar dados atuais').events.click();
 form=all(e['#pedidos'],'form')[0];
 assert.equal(all(form,'input')[1].value,'42,00');
 assert.equal(all(form,'input')[0].value,'Retornar amanhã');
 assert.equal(all(form,'button')[0].disabled,false);
});

test('versão do rascunho não muda quando uma nova consulta traz alterações remotas',async()=>{
 let changed=false;
 const {elements:e,requests}=await view({onRead:(url,rows)=>{
  if(changed && url.endsWith('2026')) rows[0].reembolso_versao=1;
  return rows;
 },onSave:async()=>({reembolsado_centavos:1234,reembolso_versao:1})});
 let input=all(e['#pedidos'],'input')[1];input.value='12,34';input.events.input();
 all(e['#anos'],'a').find(el=>el.textContent==='2025').events.click({preventDefault(){}});await settle();
 changed=true;
 all(e['#anos'],'a').find(el=>el.textContent==='2026').events.click({preventDefault(){}});await settle();
 const form=all(e['#pedidos'],'form')[0];
 await form.events.submit({preventDefault(){}});
 assert.equal(requests[0].options.data.versao,0);
});

test('exclusão confirmada atualiza tabela, totais e anos sem acrescentar colunas',async()=>{
 const {elements:e,requests}=await view();
 assert.equal(all(e['#pedidos'],'th').length,5);
 const buttons=all(e['#pedidos'],'button').filter(b=>b.className==='danger pedido-delete');
 assert.equal(buttons.length,2);
 await buttons[0].events.click();
 assert.equal(requests[0].options.method,'DELETE');
 assert.equal(requests[0].options.data.versao,0);
 assert.equal(requests[0].url,'/api/pedidos/3');
 assert.match(text(e['#resumo']),/1 pedido/);assert.match(text(e['#resumo']),/0,10/);
 assert.match(text(e['#pedido-status']),/Pedido #3 excluído/);
 assert.equal(e['#pedido-status'].focused,true);
 all(e['#anos'],'a').find(a=>a.textContent==='2025').events.click({preventDefault(){}});await settle();
 await all(e['#pedidos'],'button').find(b=>b.className==='danger pedido-delete').events.click();
 assert.equal(all(e['#anos'],'a').some(a=>a.textContent==='2025'),false);
 assert.match(text(e['#pedidos']),/Nenhum pedido encontrado em 2025/);
});

test('cancelamento, falha e usuário normal não excluem pedidos',async()=>{
 const canceled=await view({confirm:false});
 await all(canceled.elements['#pedidos'],'button').find(b=>b.className==='danger pedido-delete').events.click();
 assert.equal(canceled.requests.length,0);
 const failed=await view({onDelete:async()=>{throw Error('Falha ao excluir');}});
 await all(failed.elements['#pedidos'],'button').find(b=>b.className==='danger pedido-delete').events.click();
 assert.match(text(failed.elements['#error']),/Falha ao excluir/);
 assert.match(text(failed.elements['#resumo']),/2 pedido/);
 assert.equal(all(failed.elements['#pedidos'],'button').find(b=>b.className==='danger pedido-delete').disabled,false);
 const normal=await view({role:'normal'});
 assert.equal(all(normal.elements['#pedidos'],'button').filter(b=>b.className==='danger pedido-delete').length,0);
});

test('exclusão pendente bloqueia novas gravações e não é desfeita por leitura atrasada',async()=>{
 let releaseDelete,releaseRead,delay=false;
 const {elements:e,requests}=await view({
  onDelete:()=>new Promise(resolve=>{releaseDelete=resolve;}),
  onRead:(url,rows)=>delay && url.endsWith('2026')?new Promise(resolve=>{releaseRead=()=>resolve(rows);}):rows
 });
 const button=all(e['#pedidos'],'button').find(b=>b.className==='danger pedido-delete');
 const pending=button.events.click();await settle();
 const form=all(e['#pedidos'],'form')[0];
 assert.equal(all(form,'input')[1].disabled,true);
 await form.events.submit({preventDefault(){}});assert.equal(requests.length,1);
 await button.events.click();assert.equal(requests.length,1);
 all(e['#anos'],'a').find(a=>a.textContent==='2025').events.click({preventDefault(){}});await settle();
 delay=true;
 all(e['#anos'],'a').find(a=>a.textContent==='2026').events.click({preventDefault(){}});await settle();
 releaseDelete();await pending;
 releaseRead();await settle();
 assert.equal(all(e['#pedidos'],'form').length,1);
 assert.match(text(e['#resumo']),/0,10/);
});

test('observações sem valor persistem, sobrevivem aos filtros e podem ser apagadas', async()=>{
 const {elements:e,requests}=await view();
 let form=all(e['#pedidos'],'form')[0], notes=all(form,'input')[0];
 notes.value='Cobrar retorno\nEnviar documento <pendente>'; notes.events.input();
 e['#limpar'].events.click();
 form=all(e['#pedidos'],'form')[0]; notes=all(form,'input')[0];
 assert.equal(notes.value,'Cobrar retorno\nEnviar documento <pendente>');
 assert.equal(form.children[0].children[0],notes);
 await form.events.submit({preventDefault(){}});
 assert.equal(requests[0].options.data.valor_reembolsado,'');
 assert.equal(requests[0].options.data.observacoes,notes.value);
 all(e['#anos'],'a').find(a=>a.textContent==='2025').events.click({preventDefault(){}}); await settle();
 all(e['#anos'],'a').find(a=>a.textContent==='2026').events.click({preventDefault(){}}); await settle();
 form=all(e['#pedidos'],'form')[0]; notes=all(form,'input')[0];
 assert.equal(notes.value,'Cobrar retorno\nEnviar documento <pendente>');
 notes.value=''; notes.events.input(); await form.events.submit({preventDefault(){}});
 assert.equal(requests[1].options.data.observacoes,'');
 assert.match(text(e['#resumo']),/1 com reembolso não informado/);
});

test('salvar somente valor preserva quebras de linha de observações antigas', async()=>{
 const original='Primeira linha\nSegunda linha';
 const {elements:e,requests}=await view({onRead:(_url,rows)=>rows.map(p=>({...p,observacoes:original}))});
 let form=all(e['#pedidos'],'form')[0];
 // Simula a normalização nativa de input[type=text] feita pelo navegador.
 all(form,'input')[0].value='Primeira linhaSegunda linha';
 const input=all(form,'input')[1];input.value='12,34';input.events.input();
 await form.events.submit({preventDefault(){}});
 assert.equal(requests[0].options.data.observacoes,original);
 e['#limpar'].events.click();form=all(e['#pedidos'],'form')[0];
 const notes=all(form,'input')[0];notes.value='Texto editado';notes.events.input();
 await form.events.submit({preventDefault(){}});
 assert.equal(requests[1].options.data.observacoes,'Texto editado');
});
