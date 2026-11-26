const requester = document.querySelector('#solicitante');
const dependent = document.querySelector('#dependente');
const listing = document.querySelector('#pedidos');
let pedidos = [];
let availableYears = [];
const drafts = new Map();
const noteDrafts = new Map();
const draftVersions = new Map();
const conflicts = new Map();
const saving = new Set();
const deleting = new Set();
const deletedIds = new Set();
const deleteButtons = new Map();
let canDeletePedidos = false;
let filtersVersion = 0;
const savedMessages = new Map();
const recentSaves = new Map();
let saveVersion = 0;
let loadVersion = 0;
let loadingYear = false;
const currentYear = String(new Date().getFullYear());
let selectedYear = currentYear;
function renderYears() {
  const footer = document.querySelector('#anos');
  footer.replaceChildren();
  const years = [...availableYears];
  if (!years.length || (years.length === 1 && years[0] === selectedYear)) return;
  if (!years.includes(currentYear)) years.push(currentYear);
  years.sort().reverse();
  const nav = node('nav');
  nav.setAttribute('aria-label', 'Anos dos pedidos');
  nav.append(node('span', 'Consultar ano:'));
  for (const year of years) {
    const link = node('a', year);
    link.href = `/pedidos?ano=${year}`;
    if (year === selectedYear) link.setAttribute('aria-current', 'page');
    link.addEventListener('click', e => {
      if (e.ctrlKey || e.metaKey || e.shiftKey || e.altKey || e.button > 0) return;
      e.preventDefault();
      if (year === selectedYear) return;
      history.pushState(null, '', link.href);
      loadYear(year);
    });
    nav.append(link);
  }
  footer.append(nav);
}
const personKey = person => person ? JSON.stringify([person.inscricao, person.nome]) : 'sem-dependente';
const money = cents => (cents / 100).toLocaleString('pt-BR', {style: 'currency', currency: 'BRL'});
const centsValue = value => {
  const [whole, decimal = ''] = value.split('.');
  return Number(whole) * 100 + Number(decimal.padEnd(2, '0'));
};
function node(tag, text, className) {
  const el = document.createElement(tag);
  if (text !== undefined) el.textContent = text;
  if (className) el.className = className;
  return el;
}
function populate(select, role, people) {
  const sorted = FormValidation.sortRecords(people, role);
  if (role === 'dependente') select.append(new Option('Sem dependente (atendimento do solicitante)', 'sem-dependente'));
  for (const person of sorted) select.append(new Option(`${person.nome} · ${person.inscricao}`, personKey(person)));
  select.disabled = false;
}
function totals(rows) {
  return rows.reduce((total, p) => {
    total.valor += centsValue(p.valor);
    total.reembolsado += p.reembolsado_centavos ?? 0;
    if (p.reembolsado_centavos === null) total.pendentes++;
    return total;
  }, {valor: 0, reembolsado: 0, pendentes: 0});
}
function summary(rows) {
  const t = totals(rows);
  const el = node('div', undefined, 'totals');
  el.append(node('span', `${rows.length} pedido(s)`), node('strong', `Valor total: ${money(t.valor)}`), node('strong', `Valor reembolsado: ${money(t.reembolsado)}`), node('small', `${t.pendentes} com reembolso não informado`));
  return el;
}
function reimbursement(p) {
  const form = node('form', undefined, 'reembolso-form');
  const noteLabel = node('label', 'Observações');
  noteLabel.className = 'observacoes-label';
  const notes = node('input');
  notes.type = 'text'; notes.maxLength = 2000;
  notes.placeholder = 'Problemas ou próximas ações';
  notes.value = noteDrafts.get(p.id) ?? p.observacoes ?? '';
  notes.disabled = saving.has(p.id) || deleting.has(p.id);
  notes.readOnly = conflicts.has(p.id);
  notes.setAttribute('aria-label', `Observações do pedido ${p.id}`);
  noteLabel.append(notes);
  const label = node('label', 'Valor reembolsado (R$)');
  const input = node('input');
  input.type = 'text'; input.inputMode = 'decimal'; input.maxLength = 18;
  input.placeholder = 'Não informado';
  const formatted = () => p.reembolsado_centavos === null ? '' : FormValidation.moneyMask((p.reembolsado_centavos / 100).toFixed(2));
  input.value = drafts.get(p.id) ?? formatted();
  input.disabled = saving.has(p.id) || deleting.has(p.id);
  input.readOnly = conflicts.has(p.id);
  input.setAttribute('aria-label', `Valor reembolsado do pedido ${p.id} (R$)`);
  const feedback = node('small'); feedback.id = `reembolso-status-${p.id}`;
  feedback.setAttribute('role', 'status'); input.setAttribute('aria-describedby', feedback.id);
  notes.setAttribute('aria-describedby', feedback.id);
  notes.addEventListener('input', () => {
    if (!draftVersions.has(p.id)) draftVersions.set(p.id, p.reembolso_versao);
    noteDrafts.set(p.id, notes.value); feedback.textContent = ''; savedMessages.delete(p.id);
  });
  input.addEventListener('input', () => { if (!draftVersions.has(p.id)) draftVersions.set(p.id, p.reembolso_versao); drafts.set(p.id, input.value); input.setCustomValidity(''); feedback.textContent = ''; savedMessages.delete(p.id); });
  input.addEventListener('blur', () => { input.value = FormValidation.moneyMask(input.value); if (drafts.has(p.id)) drafts.set(p.id, input.value); });
  const button = node('button', 'Salvar');
  button.setAttribute('aria-label', `Salvar reembolso do pedido ${p.id}`);
  form.setAttribute('aria-label', `Reembolso do pedido ${p.id}`);
  button.type = 'submit'; button.disabled = saving.has(p.id) || deleting.has(p.id);
  feedback.textContent = saving.has(p.id) ? 'Salvando…' : savedMessages.get(p.id) ?? '';
  label.append(input); form.append(noteLabel, label, button, feedback);
  if (conflicts.has(p.id)) {
    const current = conflicts.get(p.id);
    const reload = node('button', 'Usar dados atuais');
    reload.type = 'button';
    const currentValue = current.reembolsado_centavos === null ? 'não informado' : money(current.reembolsado_centavos);
    feedback.textContent = `Outro usuário alterou este pedido. Valor atual: ${currentValue}. Observações atuais: ${current.observacoes || "sem observações"}. Seu rascunho foi mantido. Use os dados atuais para continuar.`;
    button.disabled = true;
    reload.addEventListener('click', () => {
      p.observacoes = current.observacoes ?? '';
      p.reembolsado_centavos = current.reembolsado_centavos;
      p.reembolso_versao = current.reembolso_versao;
      drafts.delete(p.id); noteDrafts.delete(p.id); draftVersions.delete(p.id); conflicts.delete(p.id); savedMessages.delete(p.id);
      render();
    });
    form.append(reload);
  }
  form.addEventListener('submit', async e => {
    e.preventDefault();
    if (saving.has(p.id) || deleting.has(p.id) || deletedIds.has(p.id) || conflicts.has(p.id)) return;
    if (!draftVersions.has(p.id)) draftVersions.set(p.id, p.reembolso_versao);
    drafts.set(p.id, input.value);
    // Inputs de uma linha removem quebras de linha ao exibir notas antigas.
    // Preserve o texto original enquanto o usuário não editar as observações.
    const submittedNotes = noteDrafts.get(p.id) ?? p.observacoes ?? '';
    noteDrafts.set(p.id, submittedNotes);
    const value = input.value.trim() === '' ? '' : FormValidation.moneyValue(input.value);
    if (value === null) {
      input.setCustomValidity('Informe de R$ 0,00 a R$ 999.999.999,99, com até duas casas decimais.');
      input.reportValidity(); return;
    }
    saving.add(p.id);
    if (deleteButtons.has(p.id)) deleteButtons.get(p.id).disabled = true;
    button.disabled = true; input.disabled = true; notes.disabled = true; feedback.textContent = 'Salvando…';
    try {
      const result = await api(`/api/pedidos/${p.id}/reembolso`, jsonRequest('PATCH', {valor_reembolsado: value, observacoes: submittedNotes, versao: draftVersions.get(p.id)}));
      p.observacoes = result.observacoes ?? submittedNotes;
      p.reembolsado_centavos = result.reembolsado_centavos;
      p.reembolso_versao = result.reembolso_versao;
      const visible = pedidos.find(row => row.id === p.id);
      if (visible) { visible.observacoes = p.observacoes; visible.reembolsado_centavos = result.reembolsado_centavos; visible.reembolso_versao = result.reembolso_versao; }
      recentSaves.set(p.id, {version: ++saveVersion, notes: p.observacoes, cents: result.reembolsado_centavos, dbVersion: result.reembolso_versao});
      drafts.delete(p.id); noteDrafts.delete(p.id); draftVersions.delete(p.id);
      const message = value === '' ? 'Valor removido. Observações salvas.' : 'Valor salvo. Observações salvas.';
      savedMessages.set(p.id, message);
      input.value = formatted(); feedback.textContent = message;
      updateSummaries();
    } catch (error) {
      feedback.textContent = error.message; savedMessages.set(p.id, error.message);
      if (error.status === 409 && error.data) {
        conflicts.set(p.id, error.data);
        const visible = pedidos.find(row => row.id === p.id);
        if (visible) { visible.observacoes = error.data.observacoes ?? ''; visible.reembolsado_centavos = error.data.reembolsado_centavos; visible.reembolso_versao = error.data.reembolso_versao; }
      }
    }
    finally {
      saving.delete(p.id); button.disabled = false; input.disabled = false; notes.disabled = false;
      if (deleteButtons.has(p.id)) deleteButtons.get(p.id).disabled = deleting.has(p.id);
      // Se a navegação recriou o formulário durante a gravação, atualiza seus controles.
      if (!form.isConnected || conflicts.has(p.id)) render();
    }
  });
  return form;
}
function deletePedidoButton(p) {
  const button = node('button', deleting.has(p.id) ? 'Excluindo…' : 'Excluir pedido', 'danger pedido-delete');
  button.type = 'button';
  button.disabled = deleting.has(p.id) || saving.has(p.id);
  button.setAttribute('aria-label', `Excluir pedido ${p.id}`);
  button.addEventListener('click', async () => {
    if (deleting.has(p.id) || saving.has(p.id) || deletedIds.has(p.id)) return;
    const date = p.data.split('-').reverse().join('/');
    if (!window.confirm(`Excluir o pedido #${p.id}, de ${p.solicitante.nome}, em ${date}? O pedido e seus documentos serão removidos definitivamente. Esta ação não pode ser desfeita.`)) return;
    deleting.add(p.id);
    document.querySelector('#error').textContent = '';
    document.querySelector('#pedido-status').textContent = '';
    render();
    const focusAtStart = document.activeElement;
    let removed = false;
    try {
      const result = await api(`/api/pedidos/${p.id}`, jsonRequest('DELETE', {versao: p.reembolso_versao}));
      removed = true;
      deletedIds.add(p.id);
      pedidos = pedidos.filter(row => row.id !== p.id);
      for (const state of [drafts, noteDrafts, draftVersions, conflicts, savedMessages, recentSaves]) state.delete(p.id);
      document.querySelector('#pedido-status').textContent = `Pedido #${p.id} excluído.${result.warning ? ` ${result.warning}` : ''}`;
      const version = ++filtersVersion;
      try {
        const filters = await api('/api/pedidos/filtros');
        if (version === filtersVersion) availableYears = filters.anos;
      } catch (error) {
        document.querySelector('#error').textContent = 'Pedido excluído. Recarregue a página para atualizar os anos disponíveis.';
      }
    } catch (error) { document.querySelector('#error').textContent = error.message; }
    finally {
      deleting.delete(p.id);
      render();
      if (!loadingYear && selectedYear === p.data.slice(0, 4) && document.activeElement === focusAtStart) {
        const target = document.querySelector(removed ? '#pedido-status' : '#error');
        target.focus({preventScroll: true});
      }
    }
  });
  deleteButtons.set(p.id, button);
  return button;
}
function filtered() {
  return pedidos.filter(p => p.data.slice(0, 4) === selectedYear
    && (!requester.value || personKey(p.solicitante) === requester.value)
    && (!dependent.value || personKey(p.dependente) === dependent.value));
}
function updateSummaries() {
  const rows = filtered();
  document.querySelector('#resumo').replaceChildren(summary(rows));
}
function render() {
  if (loadingYear) return;
  const rows = filtered(); listing.replaceChildren();
  deleteButtons.clear();
  if (!rows.length) listing.append(node('p', availableYears.length ? `Nenhum pedido encontrado em ${selectedYear} para os filtros selecionados.` : 'Nenhum pedido de reembolso cadastrado.'));
  const section = node('section'); section.append(node('h2', selectedYear));
  const wrap = node('div', undefined, 'table-wrap'); wrap.tabIndex = 0;
  wrap.setAttribute('role', 'region'); wrap.setAttribute('aria-label', `Pedidos de ${selectedYear}`);
  const table = node('table', undefined, 'responsive-table pedidos-table');
  table.setAttribute('role', 'table');
  const caption = node('caption', `Pedidos de reembolso de ${selectedYear}`, 'visually-hidden');
  const head = node('thead'), header = node('tr');
  head.setAttribute('role', 'rowgroup'); header.setAttribute('role', 'row');
  const columns = ['Pedido / data', 'Solicitante', 'Dependente', 'Valor total', 'Ações'];
  for (const [index, name] of columns.entries()) {
    const th = node('th', name); th.scope = 'col'; th.id = `pedidos-col-${index}`;
    th.setAttribute('role', 'columnheader'); header.append(th);
  }
  table.append(caption);
  head.append(header); table.append(head);
  for (const p of rows) {
    const body = node('tbody', undefined, 'pedido-group'); body.setAttribute('role', 'rowgroup');
    const tr = node('tr'), date = node('td', `Pedido #${p.id}`, 'compact pedido-number');
    tr.setAttribute('role', 'row');
    date.append(node('small', p.data.split('-').reverse().join('/'), 'pedido-date')); tr.append(date);
    for (const role of ['solicitante', 'dependente']) {
      const person = p[role], cell = node('td', person?.nome ?? 'Sem dependente', 'record-name');
      if (person) cell.append(node('small', person.inscricao, 'pedido-date'));
      tr.append(cell);
    }
    tr.append(node('td', money(centsValue(p.valor)), 'compact'));
    const action = node('td', undefined, 'pedido-action'), link = node('a', 'Abrir / imprimir');
    link.setAttribute('aria-label', `Abrir ou imprimir pedido ${p.id}`); link.href = `/imprimir?id=${p.id}`;
    const actions = node('div', undefined, 'pedido-actions');
    actions.append(link);
    if (canDeletePedidos) actions.append(deletePedidoButton(p));
    action.append(actions); tr.append(action);
    for (const [index, cell] of [...tr.children].entries()) {
      cell.setAttribute('role', 'cell'); cell.setAttribute('headers', `pedidos-col-${index}`);
      if (index >= 1 && index <= 3) {
        const label = node('span', columns[index], 'cell-label');
        label.setAttribute('aria-hidden', 'true'); cell.prepend(label);
      }
    }
    const editRow = node('tr', undefined, 'pedido-edit-row');
    editRow.setAttribute('role', 'row');
    editRow.setAttribute('aria-label', `Observações e reembolso do pedido ${p.id}`);
    const edit = node('td', undefined, 'reembolso-cell');
    edit.colSpan = columns.length; edit.setAttribute('role', 'cell');
    edit.append(reimbursement(p)); editRow.append(edit);
    body.append(tr, editRow); table.append(body);
  }
  wrap.append(table); section.append(wrap); listing.append(section);
  updateSummaries();
  renderYears();
}
requester.addEventListener('change', render); dependent.addEventListener('change', render);
document.querySelector('#limpar').addEventListener('click', () => { requester.value = ''; dependent.value = ''; render(); });
function initialYear() {
  const requested = new URLSearchParams(location.search).get('ano');
  return requested && (availableYears.includes(requested) || requested === currentYear)
    ? requested : availableYears.length === 1 ? availableYears[0] : currentYear;
}
async function loadYear(year) {
  const version = ++loadVersion;
  const saveVersionAtStart = saveVersion;
  selectedYear = year;
  loadingYear = true;
  pedidos = [];
  listing.setAttribute('aria-busy', 'true');
  listing.replaceChildren(node('p', `Carregando pedidos de ${year}…`));
  document.querySelector('#resumo').replaceChildren();
  document.querySelector('#error').textContent = '';
  renderYears();
  try {
    const rows = await api(`/api/pedidos?ano=${year}`);
    if (version !== loadVersion) return;
    // Uma leitura iniciada antes de um salvamento pode trazer o valor anterior.
    for (const row of rows) {
      const saved = recentSaves.get(row.id);
      if (saved && saved.version > saveVersionAtStart && saved.dbVersion >= row.reembolso_versao) { row.observacoes = saved.notes; row.reembolsado_centavos = saved.cents; row.reembolso_versao = saved.dbVersion; }
    }
    loadingYear = false;
    pedidos = rows.filter(row => !deletedIds.has(row.id)); render();
  } catch (error) {
    if (version !== loadVersion) return;
    document.querySelector('#error').textContent = error.message;
    listing.replaceChildren(node('p', 'Não foi possível carregar os pedidos. Recarregue a página para tentar novamente.'));
  } finally {
    if (version === loadVersion) { loadingYear = false; listing.setAttribute('aria-busy', 'false'); }
  }
}
window.addEventListener('popstate', () => loadYear(initialYear()));
async function loadPedidos() {
  try {
    const user = await window.sessionReady;
    canDeletePedidos = user?.perfil === 'admin';
    const filters = await api('/api/pedidos/filtros');
    availableYears = filters.anos;
    populate(requester, 'solicitante', filters.solicitante);
    populate(dependent, 'dependente', filters.dependente);
    const year = initialYear();
    history.replaceState(null, '', `/pedidos?ano=${year}`);
    await loadYear(year);
  } catch (error) {
    document.querySelector('#error').textContent = error.message;
    listing.replaceChildren(node('p', 'Não foi possível carregar os pedidos. Recarregue a página para tentar novamente.'));
    listing.setAttribute('aria-busy', 'false');
  }
}
loadPedidos();
