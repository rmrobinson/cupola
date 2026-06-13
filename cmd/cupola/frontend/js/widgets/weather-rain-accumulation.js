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

  const SOIL_TYPES = [
    { value: 'loam',  label: 'Loam',  targetMM: 25 },
    { value: 'sandy', label: 'Sandy', targetMM: 35 },
    { value: 'clay',  label: 'Clay',  targetMM: 18 },
  ];

  function soilTarget(soilType) {
    return (SOIL_TYPES.find(s => s.value === soilType) || SOIL_TYPES[0]).targetMM;
  }

  // Returns watering advice based on fraction of weekly target received.
  function rainfallLevel(mm, soilType) {
    const target = soilTarget(soilType);
    const pct    = mm / target;
    const deficit = Math.max(0, target - mm);

    if (pct >= 1.0) return { color: '#00b894', advice: 'No watering needed', deficit: 0      };
    if (pct >= 0.7) return { color: '#74b9ff', advice: 'Light watering',      deficit        };
    if (pct >= 0.3) return { color: '#fdcb6e', advice: 'Moderate watering',   deficit        };
    return               { color: '#e17055', advice: 'Heavy watering',      deficit        };
  }

  function capitalise(s) {
    return s ? s.charAt(0).toUpperCase() + s.slice(1) : '';
  }

  function fmtDate(iso) {
    if (!iso) return '';
    return new Date(iso).toLocaleDateString(undefined, { month: 'long', day: 'numeric' });
  }

  function render(container, state, config) {
    const since    = config?.since     || 'sunday';
    const soilType = config?.soil_type || 'loam';

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
    const level = rainfallLevel(mm, soilType);
    const from  = fmtDate(entry.period_start);
    const deficitLine = level.deficit > 0
      ? `<div class="ra-deficit" style="color:${level.color}">${level.deficit.toFixed(1)}mm short</div>`
      : '';

    container.innerHTML = `
      <div class="widget-rain-accum">
        <div class="ra-amount" style="color:${level.color}">${mm.toFixed(1)}<span class="ra-unit">mm</span></div>
        <div class="ra-from">since ${from}</div>
        <div class="ra-advice" style="color:${level.color}">${level.advice}</div>
        ${deficitLine}
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
      {
        key:     'soil_type',
        label:   'Soil type',
        type:    'select',
        default: 'loam',
        options: SOIL_TYPES.map(s => ({ value: s.value, label: s.label })),
      },
    ],

    subscriptionParams(config) {
      return { since: config?.since || 'sunday' };
    },

    render(container, state, config)    { render(container, state, config); },
    onUpdate(container, data, config)   { render(container, data,  config); },
  });
})();
