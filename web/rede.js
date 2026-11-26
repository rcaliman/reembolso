'use strict';
const citySelect = document.querySelector('#cidade');
const specialtySelect = document.querySelector('#especialidade');
const results = document.querySelector('#rede-resultados');
const statusElement = document.querySelector('#rede-status');
const errorElement = document.querySelector('#error');
let requestVersion = 0;
let filtersReady = false;
function node(tag, text, className) {
  const el = document.createElement(tag);
  if (text !== undefined) el.textContent = text;
  if (className) el.className = className;
  return el;
}
function phoneInfo(value) {
  // Reconhece formatos completos sem acrescentar ou remover dígitos da fonte.
  if (/^0800\d{7}$/.test(value)) return {label: `${value.slice(0,4)} ${value.slice(4,7)} ${value.slice(7)}`, href: `tel:${value}`};
  if (/^[1-9]{2}[2-5]\d{7}$/.test(value)) return {label: `(${value.slice(0,2)}) ${value.slice(2,6)}-${value.slice(6)}`, href: `tel:+55${value}`};
  if (/^[1-9]{2}9\d{8}$/.test(value)) return {label: `(${value.slice(0,2)}) ${value.slice(2,7)}-${value.slice(7)}`, href: `tel:+55${value}`};
  return {label: value, href: null};
}
function phoneLink(value, allowDial = true) {
  const info = phoneInfo(value);
  if (info.href && allowDial) {
    const link = node('a', info.label);
    link.href = info.href;
    return link;
  }
  const span = node('span', undefined, 'rede-phone-text');
  span.append(node('strong', info.label));
  if (!info.href) span.append(node('small', ' · Formato não reconhecido; confirme o número antes de ligar.'));
  return span;
}
function safeURL(value) {
  try { const url = new URL(value); return ['https:', 'http:'].includes(url.protocol) ? url.href : null; }
  catch { return null; }
}
function renderProvider(provider) {
  const card = node('article', undefined, 'rede-card');
  card.append(node('h3', provider.nome));
  if (provider.nome_fantasia && provider.nome_fantasia !== provider.nome) card.append(node('p', provider.nome_fantasia, 'rede-fantasia'));
  card.append(node('p', `${provider.tipo_documento}: ${provider.documento || 'Não informado'}`));
  card.append(node('p', provider.especialidades.join(' · '), 'rede-especialidades'));
  if (provider.areas_atuacao.length) card.append(node('p', `Área de atuação: ${provider.areas_atuacao.join(' · ')}`));
  card.append(node('p', provider.endereco.replaceAll(' | ', ', ')));
  const mapsURL = safeURL(provider.mapa_url);
  if (mapsURL) {
    const map = node('a', 'Ver no mapa'); map.href = mapsURL; map.target = '_blank'; map.rel = 'noopener noreferrer'; card.append(map);
  }
  const contacts = node('div', undefined, 'rede-phones');
  contacts.append(node('h4', 'Telefones da Banescaixa'));
  if (!provider.telefones.length) contacts.append(node('p', 'Nenhum telefone informado.'));
  for (const value of provider.telefones) contacts.append(phoneLink(value));
  card.append(contacts);
  if (provider.telefones_web.length) {
    const details = node('details'); details.append(node('summary', 'Contatos encontrados na web'));
    details.append(node('p', 'Podem ser contatos adicionais ou de outro endereço. Consulte as ressalvas de cada fonte.'));
    const list = node('ul');
    for (const contact of provider.telefones_web) {
      const entry = node('li');
      entry.append(phoneLink(contact.telefone, contact.confianca !== 'baixa'));
      entry.append(node('span', ` · ${contact.comparacao_banescaixa === 'coincidente' ? 'Coincide com a Banescaixa' : 'Diferente ou adicional'} · Confiança ${contact.confianca}`));
      entry.append(node('p', contact.observacao));
      const sourceURL = safeURL(contact.fonte_url);
      if (sourceURL) {
        const source = node('a', 'Consultar fonte'); source.href = sourceURL; source.target = '_blank'; source.rel = 'noopener noreferrer'; entry.append(source);
      }
      entry.append(node('small', ` · Consulta em ${new Date(contact.consultado_em).toLocaleDateString('pt-BR')}${contact.metodo === 'resultado_de_busca' ? ' · Resultado de busca' : ''}`));
      list.append(entry);
    }
    details.append(list); card.append(details);
  }
  return card;
}
function renderResults(providers) {
  results.replaceChildren();
  if (!providers.length) { results.append(node('p', 'Nenhum prestador encontrado para os filtros selecionados.')); return; }
  let city = null, grid;
  for (const provider of providers) {
    if (provider.cidade !== city) {
      city = provider.cidade;
      const section = node('section', undefined, 'rede-city');
      section.append(node('h2', `${city} · ${provider.uf}`));
      grid = node('div', undefined, 'rede-grid'); section.append(grid); results.append(section);
    }
    grid.append(renderProvider(provider));
  }
}
function fillOptions(select, values) {
  select.replaceChildren(select.options[0]);
  for (const value of values) { if (Array.from(select.options).some(option => option.value === value)) continue; const option = node('option', value); option.value = value; select.append(option); }
}
async function search(updateHistory = true) {
  const version = ++requestVersion;
  const query = new URLSearchParams();
  if (citySelect.value) query.set('cidade', citySelect.value);
  if (specialtySelect.value) query.set('especialidade', specialtySelect.value);
  if (updateHistory) history.replaceState(null, '', `/rede-credenciada${query.size ? '?' + query : ''}`);
  results.setAttribute('aria-busy', 'true'); results.replaceChildren();
  errorElement.textContent = ''; statusElement.textContent = 'Buscando prestadores…';
  try {
    const data = await api('/api/rede-credenciada?' + query);
    if (version !== requestVersion) return;
    if (!filtersReady) {
      fillOptions(citySelect, data.cidades); fillOptions(specialtySelect, data.especialidades);
      for (const [select, key] of [[citySelect, 'cidade'], [specialtySelect, 'especialidade']]) {
        const value = new URLSearchParams(location.search).get(key) || '';
        if (value && !Array.from(select.options).some(option => option.value === value)) {
          const option = node('option', value); option.value = value; select.append(option);
        }
        select.value = value; select.disabled = false;
      }
      document.querySelector('#buscar').disabled = false; document.querySelector('#limpar').disabled = false;
      filtersReady = true;
    }
    renderResults(data.prestadores);
    statusElement.textContent = `${data.total} ${data.total === 1 ? 'prestador encontrado' : 'prestadores encontrados'}. ${citySelect.value ? 'Em ordem alfabética.' : 'Por cidade e em ordem alfabética.'}`;
  } catch (error) {
    if (version !== requestVersion) return;
    errorElement.textContent = error.message; statusElement.textContent = 'Não foi possível carregar os resultados.';
    // Permite repetir mesmo se o primeiro carregamento falhar.
    document.querySelector('#buscar').disabled = false;
  } finally { if (version === requestVersion) results.setAttribute('aria-busy', 'false'); }
}
document.querySelector('#rede-filtros').addEventListener('submit', event => { event.preventDefault(); search(filtersReady); });
document.querySelector('#limpar').addEventListener('click', () => { citySelect.value = ''; specialtySelect.value = ''; search(); });
window.sessionReady.then(() => {
  // Na primeira consulta os filtros da URL são enviados antes de preencher os selects.
  const params = new URLSearchParams(location.search);
  for (const [select, key] of [[citySelect, 'cidade'], [specialtySelect, 'especialidade']]) {
    const value = params.get(key); if (value) { const option = node('option', value); option.value = value; select.append(option); select.value = value; }
  }
  return search(false);
}).catch(() => {});
