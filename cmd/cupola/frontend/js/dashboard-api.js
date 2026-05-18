const DashboardAPI = (() => {
  async function listProfiles() {
    const res = await fetch('/api/v1/profiles');
    if (!res.ok) throw new Error(`HTTP ${res.status}`);
    return res.json();
  }

  async function getProfile(profileID) {
    const res = await fetch(`/api/v1/profiles/${encodeURIComponent(profileID)}`);
    if (!res.ok) throw new Error(`HTTP ${res.status}`);
    return res.json();
  }

  async function saveProfile(profile) {
    const res = await fetch('/api/v1/profiles', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(profile),
    });
    if (!res.ok) throw new Error(`HTTP ${res.status}`);
  }

  async function deleteProfile(profileID) {
    const res = await fetch(`/api/v1/profiles/${encodeURIComponent(profileID)}`, { method: 'DELETE' });
    if (!res.ok) throw new Error(await responseError(res));
  }

  async function exportProfile(profileID) {
    const res = await fetch(`/api/v1/profiles/${encodeURIComponent(profileID)}/export`);
    if (!res.ok) throw new Error(`HTTP ${res.status}`);
    return {
      blob: await res.blob(),
      filename: filenameFromContentDisposition(res.headers.get('Content-Disposition')) ||
        `cupola-dashboard-${profileID}.json`,
    };
  }

  async function validateImport(exportData) {
    const res = await fetch('/api/v1/profiles/import/validate', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ export: exportData }),
    });
    if (!res.ok) throw new Error(`HTTP ${res.status}`);
    return res.json();
  }

  async function importProfile({ exportData, name, widgetIDs }) {
    const res = await fetch('/api/v1/profiles/import', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ export: exportData, name, widget_ids: widgetIDs }),
    });
    if (!res.ok) throw new Error(`HTTP ${res.status}`);
    return res.json();
  }

  function filenameFromContentDisposition(value) {
    const match = (value || '').match(/filename="([^"]+)"/);
    return match?.[1] || '';
  }

  async function responseError(res) {
    const text = await res.text();
    return text.trim() || `HTTP ${res.status}`;
  }

  return {
    listProfiles,
    getProfile,
    saveProfile,
    deleteProfile,
    exportProfile,
    validateImport,
    importProfile,
    filenameFromContentDisposition,
  };
})();

if (typeof module !== 'undefined') {
  module.exports = DashboardAPI;
}
