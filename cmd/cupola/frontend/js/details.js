(function () {
  'use strict';

  let root = null;
  let lastFocus = null;
  let activeController = null;
  let requestSeq = 0;

  const FIELD_LABELS = {
    advisory_status: 'Advisory',
    affected_routes: 'Affected routes',
    agency_id: 'Agency',
    alert_type: 'Type',
    area: 'Area',
    at_half_mast: 'At half-mast',
    ends_at: 'Ends',
    expires: 'Expires',
    flow_cms: 'Flow',
    level_m: 'Level',
    location: 'Location',
    onset: 'Onset',
    published_at: 'Published',
    road_name: 'Road',
    since: 'Since',
    source_id: 'Source',
    starts_at: 'Starts',
    temp_c: 'Temperature',
    type: 'Type',
    until: 'Until',
    updated_at: 'Updated',
    waterway_name: 'Waterway',
  };

  const UNIT_SYMBOLS = {
    celsius: '°C',
    m: 'm',
    m3_per_s: 'm³/s',
  };

  const DOMAIN_LABELS = {
    'flag.status': 'Flag status',
    'municipal.alerts': 'Municipal alert',
    'traffic.incidents': 'Traffic incident',
    'transit.alerts': 'Transit alert',
    'waterway.conditions': 'Waterway',
    'weather.alerts': 'Weather alert',
  };

  function esc(s) {
    return String(s || '').replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
  }

  function escAttr(s) {
    return esc(s).replace(/"/g, '&quot;');
  }

  function fmtValue(value) {
    if (typeof value === 'boolean') return value ? 'Yes' : 'No';
    if (!value) return '';
    const ts = Date.parse(value);
    if (!Number.isNaN(ts) && /^\d{4}-\d{2}-\d{2}T/.test(value)) {
      return new Date(ts).toLocaleString(undefined, {
        month: 'short', day: 'numeric', hour: 'numeric', minute: '2-digit',
      });
    }
    return value;
  }

  function labelFor(key) {
    return FIELD_LABELS[key] || key.replace(/_/g, ' ');
  }

  function unitFor(unit) {
    return UNIT_SYMBOLS[unit] || unit || '';
  }

  function titleCase(s) {
    return String(s || '')
      .replace(/[_-]/g, ' ')
      .replace(/\w\S*/g, word => word.charAt(0).toUpperCase() + word.slice(1));
  }

  function fieldValue(d, key) {
    const f = (d.fields || []).find(field => field.key === key);
    return f ? f.value : null;
  }

  function titleFor(d) {
    if (d.domain === 'flag.status') {
      return fieldValue(d, 'at_half_mast') ? 'Flag at half-mast' : 'Flag at full mast';
    }
    if (d.domain === 'traffic.incidents') return titleCase(d.title);
    return d.title || DOMAIN_LABELS[d.domain] || 'Details';
  }

  function safeHTTPURL(raw) {
    if (!raw) return '';
    try {
      const u = new URL(raw, window.location.href);
      if (u.protocol !== 'http:' && u.protocol !== 'https:') return '';
      return u.href;
    } catch {
      return '';
    }
  }

  function isOpen() {
    return root && !root.classList.contains('hidden');
  }

  function focusableElements() {
    if (!root) return [];
    return [...root.querySelectorAll('a[href], button:not([disabled]), [tabindex]:not([tabindex="-1"])')]
      .filter(el => el.offsetParent !== null);
  }

  function ensureRoot() {
    if (root) return root;
    root = document.createElement('div');
    root.id = 'detail-root';
    root.className = 'detail-root hidden';
    root.setAttribute('aria-hidden', 'true');
    document.body.appendChild(root);

    root.addEventListener('click', e => {
      if (e.target.classList.contains('detail-backdrop') ||
          e.target.closest('.detail-close')) {
        close();
      }
    });

    document.addEventListener('keydown', e => {
      if (!isOpen()) return;
      if (e.key === 'Escape') {
        close();
        return;
      }
      if (e.key !== 'Tab') return;
      const focusable = focusableElements();
      if (focusable.length === 0) {
        e.preventDefault();
        return;
      }
      const first = focusable[0];
      const last = focusable[focusable.length - 1];
      if (e.shiftKey && document.activeElement === first) {
        e.preventDefault();
        last.focus();
      } else if (!e.shiftKey && document.activeElement === last) {
        e.preventDefault();
        first.focus();
      }
    });

    return root;
  }

  function triggerFromEvent(e) {
    const explicit = e.target.closest('.detail-open-action');
    const trigger = explicit || e.target.closest('[data-detail-domain][data-detail-id]');
    if (!trigger) return;
    if (!explicit && e.target.closest('a,button,input,select,textarea')) return;
    open(trigger.dataset.detailDomain, trigger.dataset.detailId);
  }

  function renderShell(content) {
    const el = ensureRoot();
    el.innerHTML = `
      <div class="detail-backdrop"></div>
      <aside class="detail-drawer" role="dialog" aria-modal="true" aria-label="Details">
        ${content}
      </aside>
    `;
    el.classList.remove('hidden');
    el.setAttribute('aria-hidden', 'false');
    el.querySelector('.detail-close')?.focus();
  }

  function renderLoading() {
    renderShell(`
      <div class="detail-header">
        <div>
          <div class="detail-kicker">Details</div>
          <h2>Loading</h2>
        </div>
        <button class="detail-close" type="button" aria-label="Close">&times;</button>
      </div>
      <div class="detail-body"><p class="detail-muted">Loading details...</p></div>
    `);
  }

  function renderError(message) {
    renderShell(`
      <div class="detail-header">
        <div>
          <div class="detail-kicker">Details</div>
          <h2>Unavailable</h2>
        </div>
        <button class="detail-close" type="button" aria-label="Close">&times;</button>
      </div>
      <div class="detail-body"><p class="detail-error">${esc(message)}</p></div>
    `);
  }

  function renderDetail(d) {
    const severity = d.severity && d.severity !== 'none'
      ? `<span class="detail-severity">${esc(d.severity)}</span>` : '';
    const fields = (d.fields || []).map(f => {
      const unit = unitFor(f.unit);
      const value = `${fmtValue(f.value)}${unit ? ` ${unit}` : ''}`;
      return `
      <div class="detail-field">
        <dt>${esc(labelFor(f.key))}</dt>
        <dd>${esc(value)}</dd>
      </div>
    `;
    }).join('');
    const location = d.location
      ? `<div class="detail-field"><dt>Location</dt><dd>${Number(d.location.lat).toFixed(5)}, ${Number(d.location.lon).toFixed(5)}</dd></div>`
      : '';
    const sourceURL = safeHTTPURL(d.source_url);
    const source = sourceURL
      ? `<a class="detail-source" href="${escAttr(sourceURL)}" target="_blank" rel="noopener">Open source page</a>`
      : '';

    renderShell(`
      <div class="detail-header">
        <div>
          <div class="detail-kicker">${esc(DOMAIN_LABELS[d.domain] || d.domain || 'Details')}</div>
          <h2>${esc(titleFor(d))}</h2>
          ${d.subtitle ? `<div class="detail-subtitle">${esc(d.subtitle)}</div>` : ''}
        </div>
        <button class="detail-close" type="button" aria-label="Close">&times;</button>
      </div>
      <div class="detail-body">
        ${severity}
        ${d.description ? `<p class="detail-description">${esc(d.description)}</p>` : ''}
        ${fields || location ? `<dl class="detail-fields">${fields}${location}</dl>` : ''}
        ${source}
      </div>
    `);
  }

  async function open(domain, id) {
    if (!domain || !id) return;
    const seq = ++requestSeq;
    if (activeController) activeController.abort();
    activeController = new AbortController();
    lastFocus = document.activeElement;
    renderLoading();
    try {
      const url = `/api/v1/details/${encodeURIComponent(domain)}?id=${encodeURIComponent(id)}`;
      const res = await fetch(url, { signal: activeController.signal });
      if (!res.ok) throw new Error(`Details request failed (${res.status})`);
      const detail = await res.json();
      if (seq !== requestSeq) return;
      renderDetail(detail);
    } catch (err) {
      if (err.name === 'AbortError') return;
      if (seq !== requestSeq) return;
      console.error('[cupola] detail fetch failed', err);
      renderError('Details are not available for this item.');
    }
  }

  function close() {
    if (!root) return;
    requestSeq++;
    if (activeController) activeController.abort();
    activeController = null;
    root.classList.add('hidden');
    root.setAttribute('aria-hidden', 'true');
    root.innerHTML = '';
    if (lastFocus && typeof lastFocus.focus === 'function') lastFocus.focus();
    lastFocus = null;
  }

  document.addEventListener('click', triggerFromEvent);
  document.addEventListener('keydown', e => {
    if (e.key !== 'Enter' && e.key !== ' ') return;
    if (!e.target.matches('[data-detail-domain][data-detail-id]')) return;
    e.preventDefault();
    triggerFromEvent(e);
  });

  window.CupolaDetails = { open, close };
})();
