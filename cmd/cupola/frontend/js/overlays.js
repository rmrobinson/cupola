/**
 * CupolaOverlays — shared registry for widget-provided map overlays.
 *
 * Provider widgets (e.g. transit) call register/unregister.
 * Consumer widgets (e.g. radar-map) call subscribeMap/unsubscribeMap.
 *
 * The registry cross-notifies both sides:
 *   - Map consumers are notified whenever the overlay set changes.
 *   - Overlay providers are notified whenever map availability changes,
 *     so they can enable or disable their "show on map" controls.
 */
window.CupolaOverlays = (() => {
  'use strict';

  const _overlays  = new Map(); // widgetId → overlay object
  const _mapCbs    = new Set(); // callbacks registered by map widgets
  const _availCbs  = new Set(); // callbacks registered by provider widgets

  function _notifyMap() {
    const all = [..._overlays.values()];
    // Snapshot the Set before iterating so callbacks that call
    // subscribeMap/unsubscribeMap mid-loop don't corrupt iteration.
    for (const cb of [..._mapCbs]) cb(all);
  }

  function _notifyAvail() {
    const has = _mapCbs.size > 0;
    for (const cb of [..._availCbs]) cb(has);
  }

  return {
    // ── Provider API ────────────────────────────────────────────────────────
    // overlay shape: { type: 'polyline', color: '#RRGGBB', coordinates: [[[lon,lat],...], ...] }

    register(id, overlay) {
      _overlays.set(id, overlay);
      _notifyMap();
    },

    unregister(id) {
      _overlays.delete(id);
      _notifyMap();
    },

    hasMap() {
      return _mapCbs.size > 0;
    },

    // cb(hasMap: boolean) — called immediately with current state on registration,
    // then again on every change. Mirrors the immediate-fire behaviour of subscribeMap.
    onMapAvail(cb) {
      _availCbs.add(cb);
      cb(_mapCbs.size > 0);
    },

    offMapAvail(cb) {
      _availCbs.delete(cb);
    },

    // ── Consumer API ────────────────────────────────────────────────────────

    // cb(overlays: overlay[]) — called immediately with current overlays, then on every change
    subscribeMap(cb) {
      _mapCbs.add(cb);
      _notifyAvail();
      cb([..._overlays.values()]);
    },

    unsubscribeMap(cb) {
      _mapCbs.delete(cb);
      _notifyAvail();
    },
  };
})();
