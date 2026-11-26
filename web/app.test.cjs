const {test} = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
require('./validation.js');
const source = fs.readFileSync(`${__dirname}/app.js`, 'utf8');
const emptyInvoice = () => ({prestador:{},tomador:{},pedido:{}});
const invoice = () => ({prestador:{nome:'Clínica Exemplo',documento:'11.222.333/0001-81'},tomador:{nome:'Maria da Silva',documento:'529.982.247-25'},pedido:{valor:'450.00'}});
const settle = () => new Promise(resolve => setImmediate(resolve));

async function view({onExtract, records = {pessoas:[],prestadores:[],contas:[]}} = {}) {
  const elements = {}, fields = {}, requests = [];
  function element() {
    return {_value:'',get value(){return this._value;},set value(value){this._value=String(value);},dataset:{},events:{},files:[],children:[],attributes:{},classList:{toggle(){}},
      setCustomValidity(value){this.validationMessage=value;},
      setAttribute(key,value){this.attributes[key]=value;},getAttribute(key){return this.attributes[key];},removeAttribute(key){delete this.attributes[key];},
      append(child){this.children.push(child);},after(){},focus(){},reportValidity(){},setSelectionRange(){},
      addEventListener(type,fn){(this.events[type]??=[]).push(fn);},
      dispatchEvent(event){for(const fn of this.events[event.type]||[])fn({target:this,...event});},
      set innerHTML(html) {
        for (const match of html.matchAll(/<(?:input|select)\b([^>]+)>/g)) {
          const attrs=match[1],id=attrs.match(/\bid="([^"]+)"/),name=attrs.match(/\bname="([^"]+)"/),input=element();
          if(id) elements['#'+id[1]]=input;
          if(name) {
            input.name=name[1]; input.dataset.kind=attrs.match(/data-kind="([^"]+)"/)?.[1];
            input.required=attrs.includes('required'); input.maxLength=Number(attrs.match(/maxlength="(\d+)"/)?.[1]||65);
            input.value=input.name==='conta.tipo'?'corrente':'';
            fields[input.name]=input;
          }
        }
      }
    };
  }
  const form=element();form.elements=fields;
  form.querySelectorAll=selector=>Object.values(fields).filter(input=>selector==='[name]' || selector==='[data-kind]'&&input.dataset.kind || selector.includes('name^=')&&input.name.startsWith(selector.match(/name\^="([^"]+)"/)[1]));
  form.checkValidity=()=>Object.values(fields).every(input=>input.disabled||!input.validationMessage&&(!input.required||input.value));
  for(const [name,kind] of [['pedido.valor','money'],['pedido.local','text'],['pedido.data','date']]) fields[name]=Object.assign(element(),{name,dataset:{kind},maxLength:65,required:true});
  elements['#form']=form;
  for(const id of ['sections','hasDependent','body-dependente','nota','documentos','nota-status','error','submit'])elements['#'+id]=element();
  const document={querySelector:selector=>elements[selector]??(elements[selector]=element()),
    querySelectorAll:selector=>Object.values(fields).filter(input=>selector.includes('body-conta')?input.name.startsWith('conta.'):selector.includes('body-dependente')?input.name.startsWith('dependente.'):false),
    createElement:element,getElementById:id=>elements['#'+id]??(elements['#'+id]=element())};
  const location={};
  class FormDataMock {
    constructor(form){this.items=form?Object.values(fields).filter(input=>!input.disabled).map(input=>[input.name,input.value]):[];}
    append(...args){this.items.push(args);}
    [Symbol.iterator](){return this.items[Symbol.iterator]();}
  }
  const context=vm.createContext({document,FormValidation:globalThis.FormValidation,Date,AbortController,
    Event:class{constructor(type){this.type=type;}},FormData:FormDataMock,location,
    fetch:async(url,options)=>{
      requests.push({url,options});
      if(url==='/api/cadastros')return {ok:true,json:async()=>records};
      if(url==='/api/notas/extrair')return onExtract?onExtract(options):{ok:true,json:async()=>invoice()};
      return {ok:true,json:async()=>({id:1})};
    }});
  vm.runInContext(source,context);await settle();
  async function fire(input,type) {for(const fn of input.events[type]||[])await fn({target:input,preventDefault(){}});}
  return {fields,elements,requests,location,
    upload:async name=>{elements['#nota'].files=name?[{name,size:100}]:[];await fire(elements['#nota'],'change');},
    input:async(name,value)=>{fields[name].value=value;await fire(fields[name],'input');},
    uploadDocuments:async files=>{elements['#documentos'].files=files;await fire(elements['#documentos'],'change');},
    submit:()=>fire(form,'submit'),
    blur:name=>fire(fields[name],'blur'),
    select:async(key,id)=>{elements[`#select-${key}`].value=String(id);await fire(elements[`#select-${key}`],'change');}
  };
}

test('trocar a nota limpa sugestões antigas mesmo quando a nova leitura falha', async()=>{
  let count=0;
  const v=await view({onExtract:async()=> ++count===1?{ok:true,json:async()=>invoice()}:{ok:false,json:async()=>({error:'Não foi possível ler.'})}});
  await v.upload('A.pdf');assert.equal(v.fields['pedido.valor'].value,'450,00');
  await v.upload('B.pdf');
  for(const name of ['prestador.nome','prestador.documento','solicitante.cpf','pedido.valor'])assert.equal(v.fields[name].value,'',name);
  assert.equal(v.elements['#submit'].disabled,false);
});

test('extração parcial não conserva campos omitidos da nota anterior; remover o PDF também limpa', async()=>{
  let count=0;
  const v=await view({onExtract:async()=>({ok:true,json:async()=>++count===1?invoice():{...emptyInvoice(),prestador:{nome:'Outra Clínica'}}})});
  await v.upload('A.pdf');await v.upload('B.pdf');
  assert.equal(v.fields['prestador.nome'].value,'Outra Clínica');assert.equal(v.fields['prestador.documento'].value,'');
  assert.equal(v.fields['solicitante.cpf'].value,'');assert.equal(v.fields['pedido.valor'].value,'');
  await v.upload(null);assert.equal(v.fields['prestador.nome'].value,'');
});

test('correções manuais sobrevivem à troca da nota e não são sobrescritas pela nova extração', async()=>{
  const v=await view();await v.upload('A.pdf');
  await v.input('pedido.valor','900,00');await v.input('prestador.nome','Clínica corrigida');
  await v.upload('B.pdf');
  assert.equal(v.fields['pedido.valor'].value,'900,00');assert.equal(v.fields['prestador.nome'].value,'Clínica corrigida');
});

test('não permite salvar durante a leitura, cancela a anterior e ignora a resposta fora de ordem', async()=>{
  const pending=[];
  const v=await view({onExtract:options=>new Promise(resolve=>pending.push({options,resolve}))});
  const a=v.upload('A.pdf');await settle();assert.equal(v.elements['#submit'].disabled,true);
  await v.submit();assert.equal(v.requests.filter(r=>r.url==='/api/pedidos').length,0);
  const b=v.upload('B.pdf');await settle();assert.equal(pending[0].options.signal.aborted,true);
  pending[1].resolve({ok:true,json:async()=>({...emptyInvoice(),pedido:{valor:'200.00'}})});await b;
  pending[0].resolve({ok:true,json:async()=>invoice()});await a;
  assert.equal(v.fields['pedido.valor'].value,'200,00');assert.equal(v.fields['prestador.nome'].value,'');
  assert.equal(v.elements['#submit'].disabled,false);
});

test('limpa também a conta derivada da nota, preservando uma conta alterada manualmente', async()=>{
  const records={prestadores:[],pessoas:[{id:1,nome:'Maria da Silva',inscricao:'123',cpf:'529.982.247-25'}],
    contas:[{id:1,titular:'Maria da Silva',cpf:'529.982.247-25',banco:'Banco',tipo:'corrente',agencia:'0001',numero:'123-4'}]};
  const v=await view({records});await v.upload('A.pdf');assert.equal(v.fields['conta.numero'].value,'123-4');
  await v.blur('solicitante.cpf');
  await v.upload(null);assert.equal(v.fields['conta.numero'].value,'');
  await v.upload('A.pdf');await v.input('conta.numero','999-9');await v.upload(null);
  assert.equal(v.fields['conta.numero'].value,'999-9');assert.equal(v.fields['conta.cpf'].value,'529.982.247-25');
});

const pdfFile = name => ({name,size:100});
async function fillRequired(v) {
  const values = {
    'solicitante.inscricao':'01234567890123456','solicitante.nome':'Maria da Silva','solicitante.cpf':'529.982.247-25','solicitante.email':'maria@example.com','solicitante.telefone':'(11) 99999-9999',
    'prestador.nome':'Clínica Exemplo','prestador.documento':'11.222.333/0001-81',
    'conta.tipo':'corrente','conta.banco':'Banco Exemplo','conta.agencia':'1234','conta.numero':'12345-6','conta.titular':'Maria da Silva','conta.cpf':'529.982.247-25',
    'pedido.valor':'250,50','pedido.local':'São Paulo / SP','pedido.data':'2026-10-22'
  };
  for(const [name,value] of Object.entries(values)) await v.input(name,value);
}
test('laudos não disparam extração nem apagam sugestões da nota',async()=>{
 const v=await view();await v.upload('nota.pdf');
 const before=v.fields['pedido.valor'].value;
 await v.uploadDocuments([pdfFile('laudo.pdf'),pdfFile('exame.pdf')]);
 assert.equal(v.fields['pedido.valor'].value,before);
 assert.equal(v.requests.filter(r=>r.url==='/api/notas/extrair').length,1);
 assert.equal(v.elements['#documentos'].validationMessage,'');
});
test('nota e laudos são enviados em campos próprios; ambos bloqueiam durante o salvamento',async()=>{
 const v=await view();await v.upload('nota.pdf');
 await v.uploadDocuments([pdfFile('laudo.pdf'),pdfFile('exame.pdf')]);await fillRequired(v);await v.submit();
 const saved=v.requests.find(r=>r.url==='/api/pedidos');assert.ok(saved);
 const files=saved.options.body.items.filter(([key])=>key!=='dados').map(([key,file])=>[key,file.name]);
 assert.equal(JSON.stringify(files),JSON.stringify([['nota','nota.pdf'],['documentos','laudo.pdf'],['documentos','exame.pdf']]));
 assert.equal(v.elements['#nota'].disabled,true);assert.equal(v.elements['#documentos'].disabled,true);
 assert.equal(v.location.href,'/imprimir?id=1');
});
test('permite três laudos sem nota e não executa extração automática',async()=>{
 const v=await view();await v.uploadDocuments(['a.pdf','b.pdf','c.pdf'].map(pdfFile));await fillRequired(v);await v.submit();
 assert.equal(v.requests.some(r=>r.url==='/api/notas/extrair'),false);
 const saved=v.requests.find(r=>r.url==='/api/pedidos');assert.ok(saved);
 assert.equal(saved.options.body.items.filter(([key])=>key==='documentos').length,3);
});
test('limite de três é combinado e se recupera ao remover nota ou laudo',async()=>{
 const v=await view();await fillRequired(v);await v.uploadDocuments(['a.pdf','b.pdf','c.pdf'].map(pdfFile));await v.upload('nota.pdf');
 assert.match(v.elements['#documentos'].validationMessage,/até 3/);await v.submit();
 assert.equal(v.requests.some(r=>r.url==='/api/pedidos'),false);
 await v.upload(null);assert.equal(v.elements['#documentos'].validationMessage,'');
 await v.upload('nota.pdf');await v.uploadDocuments(['a.pdf','b.pdf'].map(pdfFile));
 assert.equal(v.elements['#documentos'].validationMessage,'');
});
test('PDF inválido ou grande entre laudos bloqueia gravação sem perder dados da nota',async()=>{
 for(const file of [{name:'laudo.txt',size:100},{name:'laudo.pdf',size:10*1024*1024+1}]){
  const v=await view();await v.upload('nota.pdf');await fillRequired(v);
  await v.uploadDocuments([pdfFile('ok.pdf'),file]);assert.ok(v.elements['#documentos'].validationMessage);
  await v.submit();assert.equal(v.requests.some(r=>r.url==='/api/pedidos'),false);
  assert.equal(v.fields['pedido.valor'].value,'250,50');
  await v.uploadDocuments([]);assert.equal(v.elements['#documentos'].validationMessage,'');
 }
});
test('anexar laudo durante a leitura mantém o bloqueio até a nota terminar',async()=>{
 let resolve;
 const v=await view({onExtract:()=>new Promise(done=>{resolve=done;})});
 const pending=v.upload('nota.pdf');await settle();
 await v.uploadDocuments([pdfFile('laudo.pdf')]);await v.submit();
 assert.equal(v.elements['#submit'].disabled,true);assert.equal(v.requests.some(r=>r.url==='/api/pedidos'),false);
 resolve({ok:true,json:async()=>invoice()});await pending;
 assert.equal(v.fields['pedido.valor'].value,'450,00');assert.equal(v.elements['#submit'].disabled,false);
});
