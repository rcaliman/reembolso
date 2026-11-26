// Compartilhado pelas telas autenticadas. Redireciona quando a sessão expira.
const nativeFetch=window.fetch.bind(window);
window.fetch=async(...args)=>{const res=await nativeFetch(...args);if(res.status===401){location.replace('/login');throw Error('Sua sessão expirou. Entre novamente.')}return res};
async function api(url,options={}){const res=await fetch(url,options);const data=await res.json();if(!res.ok){const error=Error(data.error||'Não foi possível concluir a operação.');error.status=res.status;error.data=data;throw error;}return data}
function jsonRequest(method,data){return {method,headers:{'Content-Type':'application/json'},body:JSON.stringify(data)}}
function createNavigation(user) {
  const nav = document.createElement('nav');
  nav.className = 'user-nav';
  nav.setAttribute('aria-label', 'Navegação principal');

  const name = document.createElement('span');
  name.className = 'user-nav-name';
  name.textContent = `${user.login} · ${user.perfil === 'admin' ? 'Administrador' : 'Usuário'}`;

  const toggle = document.createElement('button');
  toggle.type = 'button';
  toggle.className = 'user-nav-toggle';
  toggle.textContent = 'Menu';
  toggle.setAttribute('aria-controls', 'user-nav-links');

  const links = document.createElement('div');
  links.id = 'user-nav-links';
  links.className = 'user-nav-links';
  for (const [label, url] of [
    ['Novo pedido', '/'], ['Pedidos', '/pedidos'], ['Rede credenciada', '/rede-credenciada'], ['Minha conta', '/conta'],
    ...(user.perfil === 'admin' ? [['Administração', '/admin']] : [])
  ]) {
    const link = document.createElement('a');
    link.className = 'user-nav-item';
    link.href = url;
    link.textContent = label;
    if (location.pathname === url) link.setAttribute('aria-current', 'page');
    links.append(link);
  }
  const logout = document.createElement('button');
  logout.type = 'button';
  logout.textContent = 'Sair';
  logout.className = 'user-nav-logout';
  logout.onclick = async () => {
    try {
      await api('/api/logout', {method: 'POST'});
      location.replace('/login');
    } catch (error) { alert(error.message); }
  };
  links.append(logout);
  window.pwaInstall?.attach(links);
  nav.append(name, toggle, links);

  const mobile = window.matchMedia('(max-width: 768px)');
  function setExpanded(expanded) {
    links.hidden = !expanded;
    toggle.setAttribute('aria-expanded', String(expanded));
  }
  function updateLayout() {
    const focusToggle = mobile.matches && links.contains(document.activeElement);
    const focusLink = !mobile.matches && document.activeElement === toggle;
    toggle.hidden = !mobile.matches;
    setExpanded(!mobile.matches);
    if (focusToggle) toggle.focus();
    if (focusLink) links.querySelector('a').focus();
  }
  toggle.addEventListener('click', () => setExpanded(links.hidden));
  nav.addEventListener('keydown', event => {
    if (event.key === 'Escape' && mobile.matches && !links.hidden) {
      event.preventDefault();
      setExpanded(false);
      toggle.focus();
    }
  });
  mobile.addEventListener('change', updateLayout);
  updateLayout();
  return nav;
}

window.sessionReady = api('/api/me').then(user => {
  const skip = document.createElement('a');
  skip.href = '#conteudo';
  skip.className = 'skip-link';
  skip.textContent = 'Pular para o conteúdo';
  document.body.prepend(skip, createNavigation(user));
  return user;
});
window.sessionReady.catch(error => {
  const element = document.querySelector('#error');
  if (element) element.textContent = error.message;
});
