const { test } = require('node:test');
const assert = require('node:assert/strict');
require('./validation.js');
const V = globalThis.FormValidation;

test('CPF e CNPJ: números, pontuação, zeros iniciais e exemplo oficial alfanumérico', () => {
  for (const cpf of ['52998224725', '529.982.247-25', '01234567890']) assert.equal(V.validCPF(cpf), true, cpf);
  for (const cpf of ['', '00000000000', '11111111111', '52998224724', '529982247250']) assert.equal(V.validCPF(cpf), false, cpf);
  for (const cnpj of ['11222333000181', '11.222.333/0001-81', '04.252.011/0001-10', '12.ABC.345/01DE-35', '12abc34501de35']) assert.equal(V.validCNPJ(cnpj), true, cnpj);
  for (const cnpj of ['00000000000000', '11111111111111', '12.ABC.345/01DE-34', '12.ABC.345/01DE-3A']) assert.equal(V.validCNPJ(cnpj), false, cnpj);
});
test('máscaras progressivas preservam zeros e mudam de CPF para CNPJ', () => {
  assert.equal(V.documentMask('52998224725'), '529.982.247-25');
  assert.equal(V.documentMask('01234567890', true), '012.345.678-90');
  assert.equal(V.documentMask('11222333000181'), '11.222.333/0001-81');
  assert.equal(V.documentMask('12abc34501de35'), '12.ABC.345/01DE-35');
  assert.equal(V.documentMask('5299', true), '529.9');
  assert.equal(V.documentMask(''), '');
  assert.equal(V.phoneMask('1132345678'), '(11) 3234-5678');
  assert.equal(V.phoneMask('11999999999'), '(11) 99999-9999');
  assert.equal(V.validPhone('(11) 3234-5678'), true);
  assert.equal(V.validPhone('(11) 99999-9999'), true);
  assert.equal(V.validPhone('(11) 9999-9999'), false);
});
test('moeda brasileira: digitação natural, colagem, decimais e limites', () => {
  for (const [raw, canonical] of [['250','250.00'], ['250,5','250.50'], ['1.234,56','1234.56'], ['R$ 1.234,56','1234.56'], ['250.50','250.50'], ['1.234','1234.00'], ['999.999.999,99','999999999.99'], ['0,01','0.01']]) assert.equal(V.moneyValue(raw), canonical, raw);
  for (const raw of ['-10','1e3','12,345','1.00,10','1.000.000.000,00','']) assert.equal(V.moneyValue(raw), null, raw);
  assert.equal(V.moneyMask('250'), '250,00');
  assert.equal(V.moneyMask('1234,5'), '1.234,50');
  assert.notEqual(V.message('money','0,00',true,14), '');
});
test('campos opcionais, datas reais, agência X e e-mail', () => {
  assert.equal(V.message('phone','',false,15), '');
  assert.notEqual(V.message('text','   ',true,65), '');
  assert.notEqual(V.message('registration','123A',true,17), '');
  assert.equal(V.message('bankNumber','0001-X',true,30), '');
  assert.notEqual(V.message('bankNumber','12--3',true,30), '');
  assert.equal(V.message('text',"Clínica D'Ávila",true,65), '');
  for (const value of ['nome+teste@example.com', 'nome.sobrenome@dominio.com.br']) assert.equal(V.validEmail(value), true);
  for (const value of ['nome@dominio', 'Nome <nome@example.com>', 'nome..teste@example.com']) assert.equal(V.validEmail(value), false);
  assert.equal(V.validDate('2024-02-29'), true);
  for (const value of ['2026-02-29', '2026-04-31', '0000-01-01']) assert.equal(V.validDate(value), false);
});
test('listas alfabéticas por nome ou titular com acentos e caixa', () => {
  const rows = ['Zélia','bruno','Álvaro','amanda'].map((nome,id) => ({id,nome,titular:nome,banco:'Banco'}));
  for (const key of ['solicitante','dependente','prestador','conta']) assert.deepEqual(V.sortRecords(rows,key).map(r=>r.nome), ['Álvaro','amanda','bruno','Zélia']);
  assert.equal(rows[0].nome,'Zélia');
});

test('matrículas de até 17 dígitos, incluindo zeros iniciais', () => {
  for (const value of ['00001234','0123456789012345','01234567890123456']) assert.equal(V.message('registration',value,true,17),'',value);
  for (const value of ['012345678901234567','123A','']) assert.notEqual(V.message('registration',value,true,17),'',value);
});

test('prestadores ordenados pela descrição exibida, com razão social como alternativa e desempate', () => {
  const rows = [
    {id:1,nome:'Alfa Clínica',descricao:'Zélia'},
    {id:2,nome:'Zeta Clínica',descricao:'Álvaro'},
    {id:3,nome:'bruno',descricao:''},
    {id:4,nome:'amanda'},
    {id:5,nome:'Carlos',descricao:'   '},
    {id:6,nome:'Beta Clínica',descricao:'álvaro'}
  ];
  assert.deepEqual(V.sortRecords(rows,'prestador').map(r=>r.id),[6,2,4,3,5,1]);
  assert.equal(rows[0].id,1);
});
