/**
 * Subscriptions — register and deregister widget data subscriptions.
 *
 * Tracks every active subscription locally so that on SSE reconnect all
 * subscriptions can be re-registered with the backend (the backend drops them
 * when the session's SSE connection closes).
 */
const Subscriptions = (() => {
  const active = {};  // widgetId → { widgetId, domain, params }

  // Re-register all tracked subscriptions whenever a new SSE connection opens.
  Stream.onConnect(() => {
    Object.values(active).forEach(({ widgetId, domain, params }) => {
      doPost(widgetId, domain, params);
    });
  });

  function doPost(widgetId, domain, params) {
    return fetch('/api/v1/subscriptions', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        widget_id: widgetId,
        session_id: Stream.SESSION_ID,
        domain,
        params: params || null,
      }),
    }).then(r => {
      if (!r.ok) throw new Error(`HTTP ${r.status}`);
    }).catch(err => {
      AppUI.reportError(`Subscription failed: ${domain}`, err);
    });
  }

  function create(widgetId, domain, params) {
    active[widgetId] = { widgetId, domain, params };
    return doPost(widgetId, domain, params);
  }

  function remove(widgetId) {
    delete active[widgetId];
    return fetch(`/api/v1/subscriptions/${widgetId}`, {
      method: 'DELETE',
    }).then(r => {
      if (!r.ok) throw new Error(`HTTP ${r.status}`);
    }).catch(err => {
      AppUI.reportError('Subscription cleanup failed', err);
    });
  }

  return { create, remove };
})();
