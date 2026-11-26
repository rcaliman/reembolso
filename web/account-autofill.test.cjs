const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
require('./validation.js');

function view(accounts, cpf = '529.982.247-25') {
  const source = fs.readFileSync(`${__dirname}/app.js`, 'utf8');
  const fields = Object.fromEntries(['tipo', 'banco', 'agencia', 'numero', 'titular', 'cpf'].map(key => [key, '']));
  let changes = 0;
  const select = {
    value: '',
    dispatchEvent() {
      changes++;
      const row = accounts.find(row => String(row.id) === this.value);
      for (const key of Object.keys(fields)) fields[key] = row?.[key] || '';
    }
  };
  const form = { elements: { 'solicitante.cpf': { value: cpf }, 'dependente.cpf': { value: '012.345.678-90' } } };
  const context = vm.createContext({ V: globalThis.FormValidation, form, Event, document: { querySelector: () => select } });
  vm.runInContext(source.slice(source.indexOf('let records = {};'), source.indexOf('for (const [i,')), context);
  context.accounts = accounts;
  vm.runInContext('records.conta = accounts;', context);
  return { form, select, fields, changes: () => changes,
    fill: () => vm.runInContext('autofillAccount()', context),
    edit: () => vm.runInContext('accountEdited = true', context) };
}
const account = (id, cpf, banco = 'Banco Exemplo') => ({ id, cpf, banco, tipo: 'corrente', agencia: '0001', numero: `123-${id}`, titular: 'Maria da Silva' });

test('preenche todos os dados da primeira conta com o CPF do solicitante, ignorando a máscara', () => {
  const v = view([account(1, '01234567890'), account(2, '52998224725'), account(3, '529.982.247-25')]);
  assert.equal(v.fill(), true);
  assert.equal(v.select.value, '2');
  assert.deepEqual(v.fields, accountFields(account(2, '52998224725')));
});
function accountFields({ id, ...fields }) { return fields; }

test('troca a conta automática ao mudar o solicitante e limpa se não há conta correspondente', () => {
  const v = view([account(1, '52998224725'), account(2, '01234567890')]);
  v.fill();
  v.form.elements['solicitante.cpf'].value = '01234567890';
  v.fill();
  assert.equal(v.select.value, '2');
  v.form.elements['solicitante.cpf'].value = '07405275761';
  assert.equal(v.fill(), false);
  assert.equal(v.select.value, '');
  assert.equal(v.fields.numero, '');
});

test('preserva a conta escolhida ou editada manualmente', () => {
  const v = view([account(1, '52998224725')]);
  v.fill(); v.fields.numero = '987-6'; v.edit();
  v.form.elements['solicitante.cpf'].value = '01234567890';
  assert.equal(v.fill(), false);
  assert.equal(v.fields.numero, '987-6');
  assert.equal(v.changes(), 1);
});

test('CPF vazio ou inválido não preenche e uma lista vazia permite preenchimento manual', () => {
  for (const cpf of ['', '529982247', '11111111111']) {
    const v = view([account(1, cpf)], cpf);
    assert.equal(v.fill(), false); assert.equal(v.changes(), 0);
  }
  const v = view([]);
  assert.equal(v.fill(), false);
  assert.equal(v.fields.numero, '');
});
