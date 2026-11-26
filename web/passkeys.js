(() => {
  function decode(value) {
    const normalized = value.replace(/-/g, '+').replace(/_/g, '/');
    const text = atob(normalized.padEnd(Math.ceil(normalized.length / 4) * 4, '='));
    return Uint8Array.from(text, char => char.charCodeAt(0));
  }
  function encode(buffer) {
    if (buffer == null) return null;
    const bytes = new Uint8Array(buffer);
    let text = '';
    for (let start = 0; start < bytes.length; start += 4096) text += String.fromCharCode(...bytes.subarray(start, start + 4096));
    return btoa(text).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
  }
  function options(data, registration) {
    const publicKey = {...data.publicKey, challenge: decode(data.publicKey.challenge)};
    if (registration) publicKey.user = {...publicKey.user, id: decode(publicKey.user.id)};
    for (const key of ['allowCredentials', 'excludeCredentials']) {
      if (publicKey[key]) publicKey[key] = publicKey[key].map(credential => ({...credential, id: decode(credential.id)}));
    }
    return {publicKey};
  }
  function serialize(credential) {
    if (!credential) throw Error('A confirmação no aparelho não foi concluída.');
    const response = {clientDataJSON: encode(credential.response.clientDataJSON)};
    if (credential.response.attestationObject) {
      response.attestationObject = encode(credential.response.attestationObject);
      response.transports = credential.response.getTransports?.() || [];
    } else {
      response.authenticatorData = encode(credential.response.authenticatorData);
      response.signature = encode(credential.response.signature);
      response.userHandle = encode(credential.response.userHandle);
    }
    return {id: credential.id, rawId: encode(credential.rawId), type: credential.type,
      authenticatorAttachment: credential.authenticatorAttachment || undefined,
      clientExtensionResults: credential.getClientExtensionResults?.() || {}, response};
  }
  async function request(url, method = 'GET', data) {
    const response = await fetch(url, {method, credentials: 'same-origin', cache: 'no-store',
      ...(data === undefined ? {} : {headers: {'Content-Type': 'application/json'}, body: JSON.stringify(data)})});
    const result = await response.json();
    if (!response.ok) throw Error(result.error || 'Não foi possível confirmar a passkey.');
    return result;
  }
  function message(error) {
    if (error.name === 'NotAllowedError' || error.name === 'AbortError') return 'Confirmação cancelada ou tempo esgotado. Tente novamente ou use sua senha.';
    if (error.name === 'InvalidStateError') return 'Este aparelho já tem uma passkey para a conta. Use a passkey existente ou escolha outro dispositivo.';
    if (error.name === 'SecurityError') return 'Acesse o endereço HTTPS configurado para esta aplicação.';
    return error.message || 'Não foi possível confirmar a passkey. Tente novamente.';
  }
  const supported = () => window.isSecureContext && typeof window.PublicKeyCredential === 'function' && Boolean(navigator.credentials);
  // As funções também permitem testar o transporte binário sem depender de sensores reais.
  globalThis.Passkeys = {decode, encode, options, serialize, message};

  const login = document.querySelector('#passkey-login');
  const form = document.querySelector('#passkey-register');
  if (!login && !form) return;
  const help = document.querySelector('#passkey-help');
  const error = document.querySelector(form ? '#passkey-error' : '#error');
  const status = document.querySelector('#passkey-status');
  const button = login || document.querySelector('#passkey-add');
  let enabled = false;
  async function configure() {
    if (!supported()) {
      help.textContent = window.isSecureContext ? 'Este navegador não oferece suporte a passkeys. Você pode continuar usando sua senha.' : 'A biometria requer HTTPS com certificado confiável. Você pode continuar usando sua senha.';
      return;
    }
    try {
      const config = await request('/api/passkeys/config');
      enabled = config.enabled;
      button.disabled = !enabled;
      help.textContent = enabled
        ? login ? 'Use uma passkey já cadastrada em Minha conta. O aparelho pode pedir biometria ou PIN.' : 'Cadastre uma passkey para este endereço. Sua senha continuará disponível como alternativa.'
        : config.message;
    } catch (failure) { help.textContent = 'Não foi possível verificar a biometria. Recarregue a página ou use sua senha.'; }
  }
  login?.addEventListener('click', async () => {
    if (!enabled || login.disabled) return;
    login.disabled = true; error.textContent = '';
    try {
      const data = await request('/api/passkeys/login/begin', 'POST', {});
      const credential = await navigator.credentials.get(options(data, false));
      await request('/api/passkeys/login/finish', 'POST', serialize(credential));
      location.replace('/pedidos');
    } catch (failure) { error.textContent = message(failure); }
    finally { login.disabled = !enabled; }
  });
  async function loadKeys() {
    const list = document.querySelector('#passkey-list');
    list.setAttribute('aria-busy', 'true');
    try {
      const keys = await request('/api/passkeys'); list.replaceChildren();
      if (!keys.length) {
        const empty = document.createElement('li'); empty.textContent = 'Nenhuma passkey cadastrada.'; list.append(empty);
      }
      for (const key of keys) {
        const item = document.createElement('li'), description = document.createElement('div');
        const name = document.createElement('strong'); name.textContent = key.nome;
        const detail = document.createElement('small');
        detail.textContent = `${key.rp_id} · ${key.usado_em ? 'Último uso: ' + formatDate(key.usado_em) : 'Ainda não utilizada'}`;
        description.append(name, detail);
        const remove = document.createElement('button'); remove.type = 'button'; remove.className = 'danger'; remove.textContent = 'Remover';
        remove.setAttribute('aria-label', `Remover passkey ${key.nome}`);
        remove.addEventListener('click', async () => {
          const password = document.querySelector('#passkey-password');
          error.textContent = ''; status.textContent = '';
          if (!password.value) { error.textContent = 'Informe a senha atual para remover a passkey.'; password.focus(); return; }
          if (!confirm(`Remover a passkey “${key.nome}”? As sessões abertas com ela serão encerradas.`)) return;
          remove.disabled = true;
          try {
            await request(`/api/passkeys/${key.id}`, 'DELETE', {senha: password.value});
            status.textContent = 'Passkey removida. As sessões abertas com ela foram encerradas.';
            await loadKeys();
          } catch (failure) { error.textContent = message(failure); }
          finally { remove.disabled = false; password.value = ''; }
        });
        item.append(description, remove); list.append(item);
      }
    } catch (failure) { error.textContent = message(failure); }
    finally { list.setAttribute('aria-busy', 'false'); }
  }
  function formatDate(value) {
    const date = new Date(value.replace(' ', 'T') + 'Z');
    return isNaN(date) ? value : date.toLocaleString('pt-BR');
  }
  form?.addEventListener('submit', async event => {
    event.preventDefault();
    if (!enabled || button.disabled) return;
    button.disabled = true; error.textContent = ''; status.textContent = '';
    const password = document.querySelector('#passkey-password');
    try {
      const data = await request('/api/passkeys/register/begin', 'POST', {nome: form.elements.nome.value, senha: password.value});
      password.value = '';
      const credential = await navigator.credentials.create(options(data, true));
      await request('/api/passkeys/register/finish', 'POST', serialize(credential));
      status.textContent = 'Passkey cadastrada. Você já pode usá-la na tela de entrada.';
      form.reset(); await loadKeys();
    } catch (failure) { error.textContent = message(failure); }
    finally { button.disabled = !enabled; password.value = ''; }
  });
  configure();
  if (form) loadKeys();
})();
