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
  const cautioning = new Set();   // collector IDs in caution state (e.g. connectivity)
  const connectHandlers = [];     // called each time a connection opens
  let es = null;
  let _lastDataAt = null;         // timestamp of last received SSE message
  let _everConnected = false;     // true after first successful open
  let _offline = false;           // true while SSE is down after first connect
  let _offlineTimer = null;

  function connect() {
    if (es) es.close();
    es = new EventSource(`/api/v1/stream?session_id=${SESSION_ID}`);
    es.onopen = () => {
      _everConnected = true;
      if (_offline) {
        _offline = false;
        if (_offlineTimer) { clearInterval(_offlineTimer); _offlineTimer = null; }
        _syncBanner();
      }
      connectHandlers.forEach(fn => fn());
    };
    es.onmessage = onMessage;
    es.onerror = () => {
      es.close();
      if (_everConnected && !_offline) {
        _offline = true;
        _syncBanner();
        _offlineTimer = setInterval(_syncBanner, 30_000);
      }
      setTimeout(connect, 3000);
    };
  }

  function onConnect(fn) {
    connectHandlers.push(fn);
  }

  function onMessage(e) {
    _lastDataAt = Date.now();
    let evt;
    try { evt = JSON.parse(e.data); } catch { return; }
    if (evt.domain === 'system') { handleSystem(evt); return; }
    (listeners[evt.domain] || []).forEach(fn => fn(evt.data, evt));
  }

  function handleSystem(evt) {
    if (evt.status === 'error') {
      failing.add(evt.collector_id);
      cautioning.delete(evt.collector_id);
    } else if (evt.status === 'caution') {
      cautioning.add(evt.collector_id);
      failing.delete(evt.collector_id);
    } else {
      failing.delete(evt.collector_id);
      cautioning.delete(evt.collector_id);
    }
    _syncBanner();
  }

  function _syncBanner() {
    const banner = document.getElementById('alert-banner');
    if (!banner) return;
    if (_offline) {
      const ago = _lastDataAt
        ? (() => {
            const mins = Math.round((Date.now() - _lastDataAt) / 60_000);
            return mins < 1 ? 'moments ago' : `${mins} min ago`;
          })()
        : 'unknown';
      banner.textContent = `Offline — last data: ${ago}`;
      banner.className = 'alert-banner alert-banner--error';
    } else if (cautioning.has('connectivity')) {
      // Internet down is the root cause of collector failures — show it first.
      banner.textContent = 'No internet connectivity — local data only';
      banner.className = 'alert-banner alert-banner--caution';
    } else if (failing.size > 0) {
      banner.textContent = 'Source unavailable: ' + [...failing].join(', ');
      banner.className = 'alert-banner alert-banner--error';
    } else if (cautioning.size > 0) {
      banner.textContent = 'No internet connectivity — local data only';
      banner.className = 'alert-banner alert-banner--caution';
    } else {
      banner.textContent = '';
      banner.className = 'alert-banner hidden';
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
