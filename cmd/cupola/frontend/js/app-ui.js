const AppUI = (() => {
  let toastTimer = null;

  function notify(message, opts = {}) {
    const toast = document.getElementById('toast');
    if (!toast) return;
    toast.textContent = message;
    toast.classList.toggle('toast-error', opts.type === 'error');
    toast.classList.remove('hidden');
    clearTimeout(toastTimer);
    toastTimer = setTimeout(() => {
      toast.textContent = '';
      toast.classList.add('hidden');
      toast.classList.remove('toast-error');
    }, opts.duration || 4500);
  }

  function reportError(message, err) {
    if (err) console.error(message, err);
    notify(message, { type: 'error' });
  }

  function registerServiceWorker() {
    if (!('serviceWorker' in navigator)) return;
    navigator.serviceWorker.register('/sw.js').catch(err => {
      console.warn('[cupola] service worker registration failed', err);
    });
  }

  return { notify, reportError, registerServiceWorker };
})();
