/**
 * WidgetPicker — shows a modal to add widgets, grouped by domain availability.
 * Fetches GET /api/v1/domains on open to determine which widgets can be added.
 */
const WidgetPicker = (() => {
  let _keyHandler = null;
  let _previousFocus = null;

  function show(onPick) {
    _previousFocus = document.activeElement;
    fetch('/api/v1/domains')
      .then(r => r.ok ? r.json() : { domains: [] })
      .then(data => render(new Set(data.domains || []), onPick))
      .catch(() => render(new Set(), onPick));
  }

  // A widget is available if any of its declared domains are active.
  // Supports both a single `domain` string and a `domains` string array.
  function isAvailable(def, availDomains) {
    if (def.domains) return def.domains.some(d => availDomains.has(d));
    return availDomains.has(def.domain);
  }

  function render(availDomains, onPick) {
    const all = window.CupolaWidgets || [];
    const byLabel = (a, b) => humanLabel(a.type).localeCompare(humanLabel(b.type));
    const available = all.filter(w => isAvailable(w, availDomains)).sort(byLabel);
    const unavailable = all.filter(w => !isAvailable(w, availDomains)).sort(byLabel);

    const picker = document.getElementById('widget-picker');
    picker.setAttribute('role', 'dialog');
    picker.setAttribute('aria-modal', 'true');
    picker.setAttribute('aria-labelledby', 'widget-picker-title');
    picker.innerHTML = `
      <div class="picker-card">
        <button class="picker-close" id="btn-picker-close" aria-label="Close">&times;</button>
        <h2 class="picker-title" id="widget-picker-title">Add widget</h2>
        ${sectionHTML('Available', available, false)}
        ${unavailable.length ? sectionHTML('Not available on this instance', unavailable, true) : ''}
      </div>
    `;
    picker.classList.remove('hidden');

    picker.querySelector('#btn-picker-close').addEventListener('click', hide);
    picker.addEventListener('click', e => {
      if (e.target === picker) hide();
    });

    if (_keyHandler) document.removeEventListener('keydown', _keyHandler);
    _keyHandler = e => {
      if (e.key === 'Escape') {
        hide();
        return;
      }
      if (e.key === 'Tab') trapFocus(e, picker);
    };
    document.addEventListener('keydown', _keyHandler);

    picker.querySelectorAll('.picker-item:not(.unavailable)').forEach(btn => {
      btn.addEventListener('click', () => {
        const type = btn.dataset.type;
        const def = all.find(w => w.type === type);
        if (!def) return;
        hide();
        onPick(def);
      });
    });

    const first = picker.querySelector('.picker-item:not(.unavailable), #btn-picker-close');
    first?.focus();
  }

  function sectionHTML(label, defs, isUnavailable) {
    if (!defs.length) return '';
    const cls = isUnavailable ? ' unavailable' : '';
    const items = defs.map(d => {
      const domainStr = d.domains ? d.domains.join(', ') : d.domain;
      return `
        <button class="picker-item${cls}" data-type="${esc(d.type)}"
          title="${isUnavailable ? `Requires domain: ${esc(domainStr)}` : ''}">
          <span class="picker-item-type">${esc(humanLabel(d.type))}</span>
          <span class="picker-item-domain">${esc(domainStr)}</span>
        </button>
      `;
    }).join('');
    return `<p class="picker-section-label">${label}</p><div class="picker-grid">${items}</div>`;
  }

  function hide() {
    const picker = document.getElementById('widget-picker');
    picker.classList.add('hidden');
    picker.removeAttribute('role');
    picker.removeAttribute('aria-modal');
    picker.removeAttribute('aria-labelledby');
    if (_keyHandler) {
      document.removeEventListener('keydown', _keyHandler);
      _keyHandler = null;
    }
    if (_previousFocus && document.contains(_previousFocus)) {
      _previousFocus.focus();
    }
    _previousFocus = null;
  }

  function trapFocus(e, root) {
    const focusable = [...root.querySelectorAll('button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])')]
      .filter(el => !el.disabled && el.offsetParent !== null);
    if (focusable.length === 0) return;
    const first = focusable[0];
    const last = focusable[focusable.length - 1];
    if (e.shiftKey && document.activeElement === first) {
      e.preventDefault();
      last.focus();
    } else if (!e.shiftKey && document.activeElement === last) {
      e.preventDefault();
      first.focus();
    }
  }

  function humanLabel(type) {
    return type.replace(/-/g, ' ').replace(/\b\w/g, c => c.toUpperCase());
  }

  function esc(s) {
    return String(s || '').replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;');
  }

  return { show, hide };
})();
