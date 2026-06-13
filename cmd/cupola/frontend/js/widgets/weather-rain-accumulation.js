(function () {
  'use strict';
  window.CupolaWidgets = window.CupolaWidgets || [];

  const DAYS = [
    { value: 'sunday',    label: 'Sunday'    },
    { value: 'monday',    label: 'Monday'    },
    { value: 'tuesday',   label: 'Tuesday'   },
    { value: 'wednesday', label: 'Wednesday' },
    { value: 'thursday',  label: 'Thursday'  },
    { value: 'friday',    label: 'Friday'    },
    { value: 'saturday',  label: 'Saturday'  },
  ];

  // Color and watering advice based on accumulated mm.
  function rainfallLevel(mm) {
    if (mm <= 0)   return { color: '#e17055', advice: 'Water now'       };
    if (mm < 10)   return { color: '#fdcb6e', advice: 'Consider watering' };
    if (mm < 25)   return { color: '#74b9ff', advice: 'Probably okay'   };
    return               { color: '#00b894', advice: 'No watering needed' };
  }

  function capitalise(s) {
    return s ? s.charAt(0).toUpperCase() + s.slice(1) : '';
  }

  function fmtDate(iso) {
    if (!iso) return '';
    return new Date(iso).toLocaleDateString(undefined, { weekday: 'long', month: 'long', day: 'numeric' });
  }

  function render(container, state, config) {
    const since = config?.since || 'sunday';

    if (!state) {
      container.innerHTML = `
        <div class="widget-unavailable">
          <span class="widget-unavailable-label">Source unavailable</span>
          <span style="font-size:10px;opacity:.5">weather.rain_accumulation</span>
        </div>`;
      return;
    }

    const entry = state.entries?.[since];

    if (!entry) {
      container.innerHTML = `
        <div class="widget-rain-accum">
          <div class="ra-waiting">Waiting for data since ${capitalise(since)}…</div>
        </div>`;
      return;
    }

    const mm    = entry.rain_mm ?? 0;
    const level = rainfallLevel(mm);
    const from  = fmtDate(entry.period_start);

    container.innerHTML = `
      <div class="widget-rain-accum">
        <div class="ra-amount" style="color:${level.color}">${mm.toFixed(1)}<span class="ra-unit">mm</span></div>
        <div class="ra-since">since ${capitalise(since)}</div>
        <div class="ra-from">${from}</div>
        <div class="ra-advice" style="color:${level.color}">${level.advice}</div>
      </div>`;
  }

  window.CupolaWidgets.push({
    type:        'weather-rain-accumulation',
    label:       'Rain Since Day',
    domain:      'weather.rain_accumulation',
    defaultSize: { w: 3, h: 3 },

    configSchema: [
      {
        key:     'since',
        label:   'Since',
        type:    'select',
        default: 'sunday',
        options: DAYS,
      },
    ],

    subscriptionParams(config) {
      return { since: config?.since || 'sunday' };
    },

    render(container, state, config)    { render(container, state, config); },
    onUpdate(container, data, config)   { render(container, data,  config); },
  });
})();
