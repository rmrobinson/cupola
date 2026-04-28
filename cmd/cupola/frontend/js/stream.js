/**
 * Stream — SSE client with session ID, domain dispatch, and system event handling.
 *
 * Usage:
 *   Stream.on('notes', (data, rawEvent) => { ... });
 *   Stream.connect();  // called by main.js on DOMContentLoaded
 */
const Stream = (() => {
  // Stable session ID for subscription lifecycle management (Phase 5).
  const SESSION_ID = (typeof crypto !== 'undefined' && crypto.randomUUID)
    ? crypto.randomUUID()
    : 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, c => {
        const r = Math.random() * 16 | 0;
        return (c === 'x' ? r : (r & 0x3 | 0x8)).toString(16);
      });

  const listeners = {};           // domain → [fn, ...]
  const failing = new Set();      // collector IDs currently in error state
  const connectHandlers = [];     // called each time a connection opens
  let es = null;

  function connect() {
    if (es) es.close();
    es = new EventSource(`/api/v1/stream?session_id=${SESSION_ID}`);
    es.onopen = () => { connectHandlers.forEach(fn => fn()); };
    es.onmessage = onMessage;
    es.onerror = () => {
      es.close();
      setTimeout(connect, 3000);
    };
  }

  function onConnect(fn) {
    connectHandlers.push(fn);
  }

  function onMessage(e) {
    let evt;
    try { evt = JSON.parse(e.data); } catch { return; }
    if (evt.domain === 'system') { handleSystem(evt); return; }
    (listeners[evt.domain] || []).forEach(fn => fn(evt.data, evt));
  }

  function handleSystem(evt) {
    if (evt.status === 'error') {
      failing.add(evt.collector_id);
    } else {
      failing.delete(evt.collector_id);
    }
    const banner = document.getElementById('alert-banner');
    if (!banner) return;
    if (failing.size === 0) {
      banner.textContent = '';
      banner.classList.add('hidden');
    } else {
      banner.textContent = 'Source unavailable: ' + [...failing].join(', ');
      banner.classList.remove('hidden');
    }
  }

  function on(domain, fn) {
    (listeners[domain] = listeners[domain] || []).push(fn);
  }

  function off(domain, fn) {
    if (!listeners[domain]) return;
    listeners[domain] = listeners[domain].filter(f => f !== fn);
  }

  return { connect, on, off, onConnect, SESSION_ID };
})();
