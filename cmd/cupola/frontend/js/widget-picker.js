/**
 * WidgetPicker — shows a modal to add widgets, grouped by domain availability.
 * Fetches GET /api/v1/domains on open to determine which widgets can be added.
 */
const WidgetPicker = (() => {
  function show(onPick) {
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
    const available = all.filter(w => isAvailable(w, availDomains));
    const unavailable = all.filter(w => !isAvailable(w, availDomains));

    const picker = document.getElementById('widget-picker');
    picker.innerHTML = `
      <div class="picker-card">
        <button class="picker-close" id="btn-picker-close">&times;</button>
        <h2 class="picker-title">Add widget</h2>
        ${sectionHTML('Available', available, false)}
        ${unavailable.length ? sectionHTML('Not available on this instance', unavailable, true) : ''}
      </div>
    `;
    picker.classList.remove('hidden');

    picker.querySelector('#btn-picker-close').addEventListener('click', hide);
    picker.addEventListener('click', e => {
      if (e.target === picker) hide();
    });

    picker.querySelectorAll('.picker-item:not(.unavailable)').forEach(btn => {
      btn.addEventListener('click', () => {
        const type = btn.dataset.type;
        const def = all.find(w => w.type === type);
        if (!def) return;
        hide();
        onPick(def);
      });
    });
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
    document.getElementById('widget-picker').classList.add('hidden');
  }

  function humanLabel(type) {
    return type.replace(/-/g, ' ').replace(/\b\w/g, c => c.toUpperCase());
  }

  function esc(s) {
    return String(s || '').replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;');
  }

  return { show, hide };
})();
