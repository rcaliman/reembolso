const {test} = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');

function element(tag) {
  return {tag, children: [], style: {}, attributes: {}, className: '',
    classList: { remove() {} },
    append(child) { this.children.push(child); },
    setAttribute(name, value) { this.attributes[name] = value; },
    async decode() { this.decoded = true; }
  };
}
async function render(invoicePages, extras = {}) {
  const sheet = element('article'), pages = element('div'), toolbar = element('nav'), error = element('div');
  sheet.className = 'sheet hidden'; sheet.append(element('img')); pages.append(sheet);
  const button = element('button');
  let resolveReady;
  const ready = new Promise(resolve => {resolveReady = resolve;});
  Object.defineProperty(button, 'disabled', {set(value) { if (!value) resolveReady(); }});
  const document = {
    title: '', fonts: {ready: Promise.resolve()}, createElement: element,
    body: {append() {throw Error('Folhas não devem ser inseridas junto aos controles.');}},
    get images() {return pages.children.flatMap(p => p.children.filter(c => c.tag === 'img'));},
    querySelector(selector) {return {'.sheet':sheet, '.print-pages':pages, '.print-toolbar':toolbar, '#print':button, '#error':error}[selector];},
    querySelectorAll() {return [];}
  };
  const data = {
    solicitante: {inscricao:'01234567890123456',nome:'Pessoa Exemplo'},
    prestador: {descricao:'Dra. Ana — cardiologista',nome:'Clínica Exemplo',documento:'11.222.333/0001-81'},
    conta: {tipo:'corrente',banco:'001',agencia:'0001',numero:'12345-6',titular:'Pessoa Exemplo',cpf:'529.982.247-25'},
    pedido: {data:'2026-10-16',local:'São Paulo',valor:'250.50'},
    ...extras,
    ...(invoicePages ? {nota: {paginas:String(invoicePages)}} : {})
  };
  let printed = false;
  vm.runInNewContext(fs.readFileSync(__dirname + '/print.js','utf8'), {
    document, location: {search:'?id=1'}, URLSearchParams,
    fetch: async () => ({ok:true,json:async () => data}), window: {print() {printed=true;}}
  });
  let timer;
  try {await Promise.race([ready,new Promise((_,reject)=>{timer=setTimeout(()=>reject(Error(error.textContent || 'Impressão não ficou pronta.')),1000);})]);}
  finally {clearTimeout(timer);}
  return {pages,sheet,button,document,printed:()=>printed};
}
test('sem nota: somente a folha do formulário e imagens prontas para imprimir', async()=>{
  const view=await render(0);
  assert.equal(view.pages.children.length,1);
  assert.equal(view.pages.children[0],view.sheet);
  assert.equal(view.document.images.every(img=>img.decoded),true);
  view.button.onclick(); assert.equal(view.printed(),true);
});
test('com nota: formulário primeiro, páginas da nota depois, no mesmo contêiner', async()=>{
  const view=await render(2);
  assert.equal(view.pages.children.length,3);
  assert.equal(view.pages.children[0],view.sheet);
  for(let i=1;i<=2;i++) {
    const page=view.pages.children[i];
    assert.equal(page.className,'sheet invoice-sheet');
    assert.equal(page.children[0].src,`/api/pedidos/1/nota/paginas/${i}`);
  }
  assert.equal(view.document.images.every(img=>img.decoded),true);
});

test('descrição do prestador não aparece no formulário final', async()=>{
  const view=await render(0);
  const texts=view.sheet.children.map(el=>el.textContent).filter(Boolean);
  assert.ok(texts.includes('Clínica Exemplo'));
  assert.ok(texts.includes('11.222.333/0001-81'));
  assert.equal(texts.some(text=>text.includes('Dra. Ana')),false);
});

 test('imprime os três documentos na ordem e aguarda todas as imagens', async()=>{
 const view=await render(2,{nota2:{nome:'Laudo.pdf',paginas:'1'},nota3:{nome:'Exame.pdf',paginas:'2'}});
 assert.equal(view.pages.children.length,6);
 assert.equal(view.pages.children[3].children[0].src,'/api/pedidos/1/documentos/2/paginas/1');
 assert.equal(view.pages.children[5].children[0].src,'/api/pedidos/1/documentos/3/paginas/2');
 assert.equal(view.document.images.every(img=>img.decoded),true);
 });
