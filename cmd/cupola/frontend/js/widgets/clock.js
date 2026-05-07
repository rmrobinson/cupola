(function () {
  window.CupolaWidgets = window.CupolaWidgets || [];
  window.CupolaWidgets.push({
    type: 'clock',
    domain: 'astro',
    defaultSize: { w: 3, h: 2 },
    subscriptionParams: () => null,

    render(container, _state, config) {
      container.innerHTML = `
        <div class="widget-clock">
          <div class="clock-time"></div>
          <div class="clock-date"></div>
        </div>
      `;
      const tz = config?.timezone || undefined;
      const h12 = config?.format === '12h';
      const timeEl = container.querySelector('.clock-time');
      const dateEl = container.querySelector('.clock-date');

      // Separate display logic from the interval so the initial call
      // never references `id` before it is initialised (avoids TDZ error
      // when render() is called before the cell is inserted into the DOM).
      const update = () => {
        const now = new Date();
        const hm  = now.toLocaleTimeString(undefined, {
          hour: '2-digit', minute: '2-digit', hour12: h12, timeZone: tz,
        });
        const sec = String(now.getSeconds()).padStart(2, '0');
        timeEl.innerHTML = `<span class="clock-hm">${hm}</span><span class="clock-sec">:${sec}</span>`;
        dateEl.textContent = now.toLocaleDateString(undefined, {
          weekday: 'long', year: 'numeric', month: 'long', day: 'numeric',
          timeZone: tz,
        });
      };

      update(); // Display immediately; id is not yet assigned, so don't reference it.

      const id = setInterval(() => {
        // Callback only runs after id is assigned, so clearInterval(id) is safe.
        if (!document.contains(timeEl)) { clearInterval(id); return; }
        update();
      }, 1000);

      const cell = container.closest('[data-widget-id]');
      if (cell) cell.dataset.tickInterval = String(id);
    },

    onUpdate(_container, _data, _config) {},
  });
})();
