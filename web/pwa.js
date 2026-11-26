// Instalação é uma melhoria progressiva: o site funciona sem essas APIs.
(() => {
  const standalone = window.matchMedia?.('(display-mode: standalone)').matches || navigator.standalone;
  let installed = Boolean(standalone);
  let installPrompt;
  const install = document.createElement('button');
  install.type = 'button';
  install.className = 'user-nav-item user-nav-install';
  install.textContent = 'Instalar app';
  install.hidden = installed;
  const ios = /iPad|iPhone|iPod/.test(navigator.userAgent) || (/Macintosh/.test(navigator.userAgent) && navigator.maxTouchPoints > 1);
  const instructions = !window.isSecureContext
    ? 'Para instalar no celular, acesse este aplicativo por HTTPS com um certificado confiável.'
    : ios
      ? 'No Safari, abra Compartilhar e escolha Adicionar à Tela de Início. Se aparecer, mantenha Abrir como App ativado.'
      : 'No menu do navegador, procure Instalar aplicativo ou Adicionar à tela inicial. A opção depende do navegador e do aparelho.';
  let dialog;
  function showInstructions(message = instructions) {
    if (!dialog) {
      dialog = document.createElement('dialog');
      dialog.className = 'pwa-install-dialog';
      dialog.setAttribute('aria-labelledby', 'pwa-install-title');
      const title = document.createElement('h2');
      title.id = 'pwa-install-title'; title.textContent = 'Instalar app';
      const text = document.createElement('p');
      const close = document.createElement('button');
      close.type = 'button'; close.textContent = 'Entendi';
      close.addEventListener('click', () => dialog.close());
      dialog.append(title, text, close);
      document.body.append(dialog);
    }
    dialog.querySelector('p').textContent = message;
    if (typeof dialog.showModal === 'function') {
      if (!dialog.open) dialog.showModal();
    } else { window.alert(message); }
  }
  function attach(container) {
    if (!container || install.parentNode === container) return;
    container.insertBefore(install, container.querySelector('.user-nav-logout'));
  }
  // Os scripts podem terminar em qualquer ordem; a opção só pertence ao menu.
  window.pwaInstall = {attach};
  window.sessionReady?.then(() => attach(document.querySelector('#user-nav-links'))).catch(() => {});

  window.addEventListener('beforeinstallprompt', event => {
    event.preventDefault(); installPrompt = event;
  });
  install.addEventListener('click', async () => {
    if (installed || install.disabled) return;
    if (!installPrompt) { showInstructions(); return; }
    const prompt = installPrompt; installPrompt = null; install.disabled = true;
    try {
      await prompt.prompt();
      await prompt.userChoice;
    } catch (error) {
      showInstructions('Não foi possível abrir a instalação. Tente novamente pelo menu do navegador.');
    } finally { install.disabled = false; }
  });
  window.addEventListener('appinstalled', () => {
    installed = true; installPrompt = null; install.hidden = true;
    if (dialog?.open) dialog.close();
  });

  if (!('serviceWorker' in navigator) || !window.isSecureContext) return;
  let requestedUpdate = false;
  navigator.serviceWorker.addEventListener('controllerchange', () => {
    if (requestedUpdate) { requestedUpdate = false; location.reload(); }
  });
  function offerUpdate(registration) {
    if (!registration.waiting || document.querySelector('#pwa-update')) return;
    const notice = document.createElement('div'); notice.id = 'pwa-update'; notice.className = 'pwa-update';
    const message = document.createElement('p'); message.setAttribute('role', 'status');
    message.textContent = 'Nova versão disponível. Salve o que estiver preenchendo antes de atualizar.';
    const update = document.createElement('button'); update.type = 'button'; update.textContent = 'Atualizar aplicativo';
    update.addEventListener('click', () => {
      if (!registration.waiting) { notice.remove(); return; }
      requestedUpdate = true; update.disabled = true;
      registration.waiting.postMessage({type: 'ACTIVATE_UPDATE'});
    });
    notice.append(message, update);
    (document.querySelector('main') || document.body).prepend(notice);
  }
  navigator.serviceWorker.register('/sw.js', {scope: '/', updateViaCache: 'none'}).then(registration => {
    offerUpdate(registration);
    registration.addEventListener('updatefound', () => {
      const worker = registration.installing;
      worker?.addEventListener('statechange', () => {
        if (worker.state === 'installed' && navigator.serviceWorker.controller) offerUpdate(registration);
      });
    });
  }).catch(error => {
    // Uma falha no modo instalável não impede o acesso normal aos pedidos.
    console.warn('Não foi possível preparar o modo PWA.', error);
  });
})();
