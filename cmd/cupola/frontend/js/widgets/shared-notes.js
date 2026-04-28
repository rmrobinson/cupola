/**
 * shared-notes widget — read/write shared notes with live SSE updates.
 *
 * Widget contract:
 *   render(container, state, config)   — called on initial load
 *   onUpdate(container, data, config)  — called on SSE domain update
 *
 * Both state (from GET /api/v1/state/notes) and data (from SSE) have shape:
 *   { updated_at: string, notes: Note[] }
 */
(function () {
  window.CupolaWidgets = window.CupolaWidgets || [];

  function esc(s) {
    if (s == null) return '';
    return String(s)
      .replace(/&/g, '&amp;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;');
  }

  function renderList(container, notes) {
    const list = container.querySelector('.notes-list');
    if (!list) return;

    if (!notes || notes.length === 0) {
      list.innerHTML = '<p class="notes-empty">No notes yet.</p>';
      return;
    }

    list.innerHTML = notes.map(n => `
      <div class="note-card${n.pinned ? ' note-pinned' : ''}" data-id="${esc(n.id)}">
        <div class="note-card-top">
          <span class="note-title">${esc(n.title) || '<em>Untitled</em>'}</span>
          <span class="note-meta">${n.pinned ? '&#9733; ' : ''}${esc(n.author)}</span>
        </div>
        ${n.body ? `<p class="note-body">${esc(n.body)}</p>` : ''}
        <div class="note-card-actions">
          <button class="btn-note-edit"   data-id="${esc(n.id)}">Edit</button>
          <button class="btn-note-delete" data-id="${esc(n.id)}">Delete</button>
        </div>
      </div>
    `).join('');

    list.querySelectorAll('.btn-note-delete').forEach(btn => {
      btn.addEventListener('click', () =>
        fetch(`/api/v1/notes/${btn.dataset.id}`, { method: 'DELETE' })
      );
    });

    list.querySelectorAll('.btn-note-edit').forEach(btn => {
      btn.addEventListener('click', () => {
        const note = notes.find(n => n.id === btn.dataset.id);
        showEditor(container, note || null);
      });
    });
  }

  function showEditor(container, note) {
    const editor = container.querySelector('.notes-editor');
    if (!editor) return;
    const id = note ? note.id : null;

    editor.innerHTML = `
      <form class="note-form">
        <input  class="note-f-title"  type="text"  placeholder="Title"  value="${esc(note?.title || '')}">
        <textarea class="note-f-body" placeholder="Body">${esc(note?.body || '')}</textarea>
        <label class="note-f-pinned-label">
          <input type="checkbox" class="note-f-pinned" ${note?.pinned ? 'checked' : ''}> Pinned
        </label>
        <div class="note-form-btns">
          <button type="submit"  class="btn-primary btn-small">Save</button>
          <button type="button" class="btn-secondary btn-small btn-cancel-note">Cancel</button>
        </div>
      </form>
    `;
    editor.classList.remove('hidden');

    editor.querySelector('.btn-cancel-note').addEventListener('click', () =>
      editor.classList.add('hidden')
    );

    editor.querySelector('.note-form').addEventListener('submit', async e => {
      e.preventDefault();
      const title  = editor.querySelector('.note-f-title').value.trim();
      const body   = editor.querySelector('.note-f-body').value;
      const pinned = editor.querySelector('.note-f-pinned').checked;
      const url    = id ? `/api/v1/notes/${id}` : '/api/v1/notes';
      await fetch(url, {
        method:  id ? 'PATCH' : 'POST',
        headers: { 'Content-Type': 'application/json' },
        body:    JSON.stringify({ title, body, pinned }),
      });
      editor.classList.add('hidden');
    });
  }

  window.CupolaWidgets.push({
    type:   'shared-notes',
    domain: 'notes',
    defaultSize: { w: 4, h: 8 },
    subscriptionParams: () => null,

    render(container, state, _config) {
      container.innerHTML = `
        <div class="widget-notes">
          <div class="notes-toolbar">
            <span class="widget-title">Notes</span>
            <button class="btn-new-note btn-small">+ New</button>
          </div>
          <div class="notes-editor hidden"></div>
          <div class="notes-list"></div>
        </div>
      `;
      renderList(container, state?.notes || []);
      container.querySelector('.btn-new-note').addEventListener('click', () =>
        showEditor(container, null)
      );
    },

    onUpdate(container, data, _config) {
      renderList(container, data?.notes || []);
    },
  });
})();
