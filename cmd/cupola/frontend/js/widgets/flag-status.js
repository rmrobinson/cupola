(function () {
  window.CupolaWidgets = window.CupolaWidgets || [];

  function esc(s) {
    return String(s || '').replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;');
  }

  function fmtDate(iso) {
    if (!iso) return null;
    return new Date(iso).toLocaleDateString(undefined, {
      year: 'numeric', month: 'long', day: 'numeric',
    });
  }

  // Convert ISO 3166-1 alpha-2 code to a flag emoji using Unicode regional
  // indicator symbols (U+1F1E6–U+1F1FF, offset 127397 from ASCII A=65).
  function countryFlag(code) {
    if (!code || code.length !== 2) return null;
    return [...code.toUpperCase()]
      .map(c => String.fromCodePoint(0x1F1E6 + c.charCodeAt(0) - 65))
      .join('');
  }

  function render(container, data) {
    if (!data) {
      container.innerHTML = `<div class="widget-unavailable"><span class="widget-unavailable-label">No flag data</span></div>`;
      return;
    }
    const halfMast = data.at_half_mast;
    const since    = fmtDate(data.since);
    const until    = fmtDate(data.until);
    const emoji    = countryFlag(window.CupolaConfig?.country_code);

    container.innerHTML = `
      <div class="widget-flag">
        <div class="flag-icon${halfMast ? ' flag-half' : ''}"
             aria-label="${halfMast ? 'Flag at half-mast' : 'Flag at full mast'}">
          <div class="flag-pole"></div>
          <div class="flag-cloth${halfMast ? ' flag-cloth-half' : ''}${emoji ? ' flag-cloth-emoji' : ''}"
               ${emoji ? `aria-label="${esc(window.CupolaConfig.country_code)} flag"` : ''}
          >${emoji ? emoji : ''}</div>
        </div>

        <div class="flag-status-label">
          ${halfMast ? 'Flag at half-mast' : 'Flag at full mast'}
        </div>

        ${halfMast && data.reason ? `
          <div class="flag-reason">${esc(data.reason)}</div>
        ` : ''}

        ${halfMast ? `
          <div class="flag-dates">
            ${since  ? `<span class="flag-date-item">Since ${esc(since)}</span>` : ''}
            ${until
              ? `<span class="flag-date-item">Until ${esc(until)}</span>`
              : since ? `<span class="flag-date-item flag-date-indefinite">Until further notice</span>` : ''}
          </div>
        ` : ''}
      </div>
    `;
  }

  window.CupolaWidgets.push({
    type: 'flag-status',
    domain: 'flag.status',
    defaultSize: { w: 2, h: 4 },
    subscriptionParams: () => null,
    render(container, state, _config) { render(container, state); },
    onUpdate(container, data, _config)  { render(container, data); },
  });
})();
