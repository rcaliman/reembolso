const V = FormValidation;
const specs = [
  ['solicitante', 'Solicitante', [
    ['inscricao', 'Inscrição', 17, 'registration', 'Até 17 dígitos'],
    ['nome', 'Nome', 65, 'text'],
    ['cpf', 'CPF', 14, 'cpf'],
    ['email', 'E-mail', 65, 'email', 'nome@dominio.com'],
    ['email_secundario', 'E-mail secundário (opcional)', 65, 'email', 'nome@dominio.com'],
    ['telefone', 'Telefone para contato', 15, 'phone', '(11) 99999-9999']
  ]],
  ['dependente', 'Dependente', [
    ['inscricao', 'Inscrição', 17, 'registration', 'Até 17 dígitos'],
    ['nome', 'Nome', 65, 'text'],
    ['cpf', 'CPF', 14, 'cpf']
  ]],
  ['prestador', 'Prestador de serviços', [
    ['nome', 'Nome / Razão social', 65, 'text'],
    ['descricao', 'Descrição (opcional, apenas para identificação)', 120, 'description', 'Ex.: Dra. Ana — cardiologista'],
    ['documento', 'CPF / CNPJ', 18, 'document', 'CPF ou CNPJ']
  ]],
  ['conta', 'Conta para crédito', [
    ['tipo', 'Tipo de conta'], ['banco', 'Banco', 35, 'bank', 'Nome ou código do banco'],
    ['agencia', 'Agência', 30, 'bankNumber', 'Ex.: 0001 ou 0001-X'],
    ['numero', 'Número da conta (com dígito)', 25, 'bankNumber', 'Ex.: 00012345-6'],
    ['titular', 'Nome do titular', 70, 'text'],
    ['cpf', 'CPF do titular', 14, 'cpf', '000.000.000-00']
  ]]
];
const container = document.querySelector('#sections'), form = document.querySelector('#form');
let records = {};
let accountEdited = false;
let applyingInvoice = false;
let invoiceLoading = false;
let savingPedido = false;
const invoiceSuggestions = new Map();
const manualInvoiceFields = new Set();
function invoiceControls() {
  return [...form.querySelectorAll('[name]'), ...specs.map(([key]) => document.querySelector(`#select-${key}`))];
}
function forgetInvoiceSection(key) {
  for (const input of invoiceControls()) {
    if (input.name?.startsWith(`${key}.`) || input === document.querySelector(`#select-${key}`)) {
      invoiceSuggestions.delete(input);
      if (input.name) manualInvoiceFields.add(input.name);
    }
  }
}
function clearInvoiceSuggestions() {
  for (const [input, value] of invoiceSuggestions) {
    if (input.value !== value) continue;
    input.value = input.name === 'conta.tipo' ? 'corrente' : '';
    if (input.dataset.kind) validateField(input);
  }
  invoiceSuggestions.clear();
}
function applyInvoiceSuggestions(action) {
  const before = new Map(invoiceControls().map(input => [input, input.value]));
  applyingInvoice = true;
  try { action(); } finally {
    applyingInvoice = false;
    for (const [input, value] of before) {
      if (input.value !== value) invoiceSuggestions.set(input, input.value);
    }
  }
}
function updateSubmitState() {
  document.querySelector('#submit').disabled = invoiceLoading || savingPedido;
}
function autofillAccount() {
  const cpf = form.elements['solicitante.cpf'].value;
  if (accountEdited || !V.validCPF(cpf)) return false;
  const digits = value => (value || '').replace(/\D/g, '');
  const account = (records.conta || []).find(row => digits(row.cpf) === digits(cpf));
  const select = document.querySelector('#select-conta');
  select.value = account ? String(account.id) : '';
  const wasApplying = applyingInvoice;
  const fromInvoice = wasApplying || invoiceSuggestions.get(form.elements['solicitante.cpf']) === cpf;
  applyingInvoice = true;
  try { select.dispatchEvent(new Event('change')); } finally { applyingInvoice = wasApplying; }
  if (!fromInvoice) {
    for (const input of invoiceSuggestions.keys()) {
      if (input === select || input.name?.startsWith('conta.')) invoiceSuggestions.delete(input);
    }
  }
  accountEdited = false;
  return Boolean(account);
}
for (const [i, [key, title, fields]] of specs.entries()) {
  const section = document.createElement('section');
  section.innerHTML = `<h2>0${i + 2} <span>${title}</span></h2>${key === 'dependente' ? '<label class="check"><input type="checkbox" id="hasDependent"> Atendimento de um dependente</label>' : ''}<div id="body-${key}"><label class="picker">Usar cadastro salvo<select id="select-${key}"><option value="">Preencher novos dados</option></select></label><div class="grid">${fields.map(([name, label, max, kind, placeholder = '']) => `<label>${label}${name === 'tipo' ? `<select name="${key}.${name}" required><option value="corrente">Conta corrente</option><option value="poupanca">Poupança</option></select>` : `<input name="${key}.${name}" maxlength="${max}" data-kind="${kind}" type="${kind === 'email' ? 'email' : kind === 'phone' ? 'tel' : 'text'}" placeholder="${placeholder}" ${['email', 'email_secundario', 'telefone', 'descricao'].includes(name) ? '' : 'required'}>`}</label>`).join('')}</div></div>`;
  container.append(section);
  document.querySelector(`#select-${key}`).addEventListener('change', e => {
    const record = records[key]?.find(r => String(r.id) === e.target.value);
    for (const [name] of fields) {
      const input = form.elements[`${key}.${name}`];
      input.value = record?.[name] ?? (name === 'tipo' ? 'corrente' : '');
      if (input.dataset.kind) {
        applyMask(input, true);
        validateField(input, Boolean(record));
      }
    }
    if (!applyingInvoice) forgetInvoiceSection(key);
    if (key === 'solicitante') autofillAccount();
    if (key === 'conta') accountEdited = true;
  });
}
for (const input of document.querySelectorAll('#body-conta input,#body-conta select')) {
  input.addEventListener('input', () => { accountEdited = true; forgetInvoiceSection('conta'); });
}
for (const event of ['input', 'blur']) {
  form.elements['solicitante.cpf'].addEventListener(event, autofillAccount);
}
function maskValue(input, blur) {
  switch (input.dataset.kind) {
    case 'cpf': return V.documentMask(input.value, true);
    case 'document': return V.documentMask(input.value);
    case 'phone': return V.phoneMask(input.value);
    case 'bankNumber': return input.value.toUpperCase();
    case 'money': return blur ? V.moneyMask(input.value) : input.value;
    default: return blur ? input.value.trim() : input.value;
  }
}
function applyMask(input, blur = false) {
  const old = input.value, caret = input.selectionStart;
  const next = maskValue(input, blur);
  if (next === old) return;
  input.value = next;
  if (!blur && caret !== null && ['cpf', 'document', 'phone'].includes(input.dataset.kind)) {
    // Reposiciona o cursor pelo número de caracteres úteis, inclusive ao editar no meio.
    const count = (old.slice(0, caret).match(/[A-Za-z0-9]/g) || []).length;
    let pos = 0, seen = 0;
    while (pos < next.length && seen < count) { if (/[A-Za-z0-9]/.test(next[pos])) seen++; pos++; }
    input.setSelectionRange(pos, pos);
  }
}
function validateField(input, show = false) {
  const message = input.disabled ? '' : V.message(input.dataset.kind, input.value, input.required, input.maxLength > 0 ? input.maxLength : 120);
  input.setCustomValidity(message);
  const error = document.getElementById(input.getAttribute('aria-describedby'));
  if (show || input.dataset.touched) {
    input.dataset.touched = 'true';
    input.setAttribute('aria-invalid', String(Boolean(message)));
    error.textContent = message;
  }
  return !message;
}
for (const input of form.querySelectorAll('[data-kind]')) {
  const error = document.createElement('small');
  error.id = `error-${input.name.replace('.', '-')}`;
  error.className = 'field-error'; error.setAttribute('aria-live', 'polite');
  input.setAttribute('aria-describedby', error.id); input.after(error);
  if (['cpf', 'registration'].includes(input.dataset.kind)) input.inputMode = 'numeric';
  if (input.dataset.kind === 'money') input.inputMode = 'decimal';
  if (['cpf', 'document', 'phone', 'bankNumber', 'registration'].includes(input.dataset.kind)) input.spellcheck = false;
  input.addEventListener('input', () => { invoiceSuggestions.delete(input); manualInvoiceFields.add(input.name); applyMask(input); validateField(input); });
  input.addEventListener('blur', () => { applyMask(input, true); validateField(input, true); });
}
function toggleDependent() {
  const enabled = document.querySelector('#hasDependent').checked;
  document.querySelector('#body-dependente').classList.toggle('hidden', !enabled);
  document.querySelectorAll('#body-dependente input,#body-dependente select').forEach(el => {
    el.disabled = !enabled;
    if (el.dataset.kind) {
      if (!enabled) {
        el.setCustomValidity(''); delete el.dataset.touched;
        el.removeAttribute('aria-invalid');
        document.getElementById(el.getAttribute('aria-describedby')).textContent = '';
      } else validateField(el);
    }
  });
}
document.querySelector('#hasDependent').addEventListener('change', toggleDependent);
toggleDependent();
const today = new Date();
form.elements['pedido.data'].value = `${today.getFullYear()}-${String(today.getMonth() + 1).padStart(2, '0')}-${String(today.getDate()).padStart(2, '0')}`;
async function load() {
  const res = await fetch('/api/cadastros');
  if (!res.ok) throw Error('Não foi possível carregar os cadastros. Recarregue a página.');
  const data = await res.json();
  for (const [key] of specs) {
    records[key] = V.sortRecords(key === 'conta' ? data.contas : key === 'prestador' ? data.prestadores : data.pessoas, key);
    const select = document.querySelector(`#select-${key}`);
    for (const row of records[key]) {
      const option = document.createElement('option'); option.value = row.id;
      option.textContent = key === 'conta' ? `${row.titular} · ${row.banco} · ${row.agencia} / ${row.numero}` : key === 'prestador' ? [row.descricao, row.nome, row.documento].filter(Boolean).join(' · ') : `${row.nome} · ${row.inscricao}`;
      select.append(option);
    }
  }
  autofillAccount();
}
const cadastrosReady = load().then(() => true).catch(e => { document.querySelector('#error').textContent = e.message; return false; });
const notaInput = document.querySelector('#nota');
const documentosInput = document.querySelector('#documentos');
function validateAttachments() {
  const notes = [...notaInput.files], documents = [...documentosInput.files];
  const validatePDFs = files => files.some(f => f.size > 10 * 1024 * 1024) ? 'Cada documento deve ter até 10 MB.' : files.some(f => !/\.pdf$/i.test(f.name)) ? 'Selecione apenas arquivos PDF.' : '';
  const noteError = notes.length > 1 ? 'Selecione apenas uma nota fiscal.' : validatePDFs(notes);
  const documentsError = notes.length + documents.length > 3 ? 'Selecione até 3 documentos no total, incluindo a nota fiscal.' : validatePDFs(documents);
  notaInput.setCustomValidity(noteError);
  documentosInput.setCustomValidity(documentsError);
  document.querySelector('#error').textContent = noteError || documentsError;
  return !noteError && !documentsError;
}
// Alterar laudos não apaga as sugestões nem dispara a leitura da nota novamente.
documentosInput.addEventListener('change', validateAttachments);
let extractionVersion = 0;
let extractionController;
const normalizeDocument = value => (value || '').replace(/[^A-Za-z0-9]/g, '').toUpperCase();
notaInput.addEventListener('change', async () => {
  const version = ++extractionVersion;
  extractionController?.abort();
  invoiceLoading = false;
  clearInvoiceSuggestions();
  updateSubmitState();
  const file = notaInput.files[0], status = document.querySelector('#nota-status');
  validateAttachments();
  status.textContent = '';
  if (!file || notaInput.validationMessage) return;
  const before = Object.fromEntries([...form.querySelectorAll('[name]')].map(el => [el.name, el.value]));
  const dependentBefore = document.querySelector('#hasDependent').checked;
  const unchanged = key => [...form.querySelectorAll(`[name^="${key}."]`)].every(el => el.value === before[el.name] && !manualInvoiceFields.has(el.name));
  function fill(key, values) {
    for (const [name, value] of Object.entries(values)) {
      const input = form.elements[`${key}.${name}`];
      if (!input || !value) continue;
      input.value = value; applyMask(input, true); validateField(input, true);
    }
  }
  function selectRecord(key, row) {
    const select = document.querySelector(`#select-${key}`);
    select.value = row.id; select.dispatchEvent(new Event('change'));
  }
  extractionController = new AbortController();
  invoiceLoading = true; updateSubmitState();
  status.textContent = 'Lendo a nota fiscal…';
  try {
    const payload = new FormData(); payload.append('nota', file);
    const res = await fetch('/api/notas/extrair', { method: 'POST', body: payload, signal: extractionController.signal });
    const data = await res.json();
    if (!res.ok) throw Error(data.error || 'Não foi possível ler a nota. Preencha manualmente.');
    await cadastrosReady;
    if (version !== extractionVersion) return;
    const messages = [];
    applyInvoiceSuggestions(() => {
      if ((data.prestador.nome || data.prestador.documento) && unchanged('prestador')) {
        const matches = (records.prestador || []).filter(row => data.prestador.documento && normalizeDocument(row.documento) === normalizeDocument(data.prestador.documento));
        if (matches.length === 1) { selectRecord('prestador', matches[0]); messages.push('Emitente encontrado no cadastro.'); }
        else {
          document.querySelector('#select-prestador').value = '';
          document.querySelector('#select-prestador').dispatchEvent(new Event('change'));
          fill('prestador', { ...data.prestador, descricao: '' });
          form.elements['prestador.descricao'].value = '';
          messages.push(matches.length > 1 ? 'Há vários prestadores com esse documento; confira o cadastro.' : 'Emitente preenchido; confira os dados.');
        }
      }
      const key = document.querySelector('#hasDependent').checked ? 'dependente' : 'solicitante';
      if ((data.tomador.nome || data.tomador.documento) && unchanged(key) && dependentBefore === document.querySelector('#hasDependent').checked) {
        const cpf = V.validCPF(data.tomador.documento || '') ? data.tomador.documento : '';
        const people = records[key] || [];
        let matches = cpf ? people.filter(row => normalizeDocument(row.cpf) === normalizeDocument(cpf)) : [];
        if (matches.length === 1) { selectRecord(key, matches[0]); messages.push('Tomador encontrado pelo CPF.'); }
        else if (!matches.length) {
          document.querySelector(`#select-${key}`).value = '';
          document.querySelector(`#select-${key}`).dispatchEvent(new Event('change'));
          fill(key, { nome: data.tomador.nome, cpf });
          messages.push('Tomador preenchido. Informe a inscrição e os demais dados.');
        } else messages.push('Há mais de uma pessoa com o CPF do tomador. Selecione o cadastro correto.');
      }
      if (autofillAccount()) messages.push('Conta para crédito selecionada pelo CPF do solicitante.');
      if (data.pedido.valor && !manualInvoiceFields.has('pedido.valor') && form.elements['pedido.valor'].value === before['pedido.valor']) {
        fill('pedido', { valor: data.pedido.valor }); messages.push('Valor preenchido.');
      }
    });
    status.textContent = messages.length ? messages.join(' ') + ' Revise os campos antes de salvar.' : 'Não foram preenchidos novos dados. PDF sem texto, modelo não reconhecido ou campos alterados durante a leitura. Preencha manualmente.';
  } catch (error) { if (version === extractionVersion) status.textContent = error.message; }
  finally { if (version === extractionVersion) { invoiceLoading = false; updateSubmitState(); } }
});
// Validação manual reúne erros por campo antes de qualquer gravação.
form.noValidate = true;
form.addEventListener('submit', async e => {
  e.preventDefault();
  if (invoiceLoading) { document.querySelector('#error').textContent = 'Aguarde a leitura da nota antes de salvar.'; return; }
  if (savingPedido) return;
  const attachmentsValid = validateAttachments();
  let firstInvalid;
  for (const input of form.querySelectorAll('[data-kind]')) {
    if (input.disabled) continue;
    applyMask(input, true);
    if (!validateField(input, true)) firstInvalid ??= input;
  }
  if (!attachmentsValid) {
    const input = notaInput.validationMessage ? notaInput : documentosInput;
    input.focus(); input.reportValidity(); return;
  }
  if (firstInvalid || !form.checkValidity()) {
    document.querySelector('#error').textContent = 'Confira os campos destacados antes de gerar o pedido.';
    if (firstInvalid) { firstInvalid.focus(); firstInvalid.reportValidity(); } else form.reportValidity();
    return;
  }
  savingPedido = true; updateSubmitState();
  notaInput.disabled = true;
  documentosInput.disabled = true;
  document.querySelector('#error').textContent = '';
  try {
    const data = {};
    for (const [key, value] of new FormData(form)) {
      const [section, name] = key.split('.'); (data[section] ??= {})[name] = value;
    }
    data.pedido.valor = V.moneyValue(form.elements['pedido.valor'].value);
    if (data.dependente) { data.dependente.email = ''; data.dependente.telefone = ''; }
    const payload = new FormData(); payload.append('dados', JSON.stringify(data));
    for (const nota of notaInput.files) payload.append('nota', nota);
    for (const documento of documentosInput.files) payload.append('documentos', documento);
    const res = await fetch('/api/pedidos', { method: 'POST', body: payload });
    const result = await res.json(); if (!res.ok) throw Error(result.error || 'Erro ao salvar.');
    location.href = `/imprimir?id=${result.id}`;
  } catch (error) { document.querySelector('#error').textContent = error.message; savingPedido = false; notaInput.disabled = false; documentosInput.disabled = false; updateSubmitState(); }
});
