package api

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/rmrobinson/cupola/internal/domain"
	"github.com/rmrobinson/cupola/internal/store"
)

type noteCreateRequest struct {
	Title  string `json:"title"`
	Body   string `json:"body"`
	Author string `json:"author"`
	Pinned bool   `json:"pinned"`
}

type notePatchRequest struct {
	Title  *string `json:"title"`
	Body   *string `json:"body"`
	Author *string `json:"author"`
	Pinned *bool   `json:"pinned"`
}

func (h *Handler) listNotes(w http.ResponseWriter, r *http.Request) {
	notes, err := h.db.ListNotes()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(notes)
}

func (h *Handler) createNote(w http.ResponseWriter, r *http.Request) {
	var req noteCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	now := time.Now().UTC()
	n := domain.Note{
		ID:        newNoteID(),
		Title:     req.Title,
		Body:      req.Body,
		Author:    req.Author,
		Pinned:    req.Pinned,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := h.db.CreateNote(n); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if err := h.notesRefresh(); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(n)
}

func (h *Handler) updateNote(w http.ResponseWriter, r *http.Request) {
	var req notePatchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	updated, err := h.db.UpdateNote(chi.URLParam(r, "id"), store.NoteUpdate{
		Title:  req.Title,
		Body:   req.Body,
		Author: req.Author,
		Pinned: req.Pinned,
	})
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if updated == nil {
		http.NotFound(w, r)
		return
	}
	if err := h.notesRefresh(); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(updated)
}

func (h *Handler) deleteNote(w http.ResponseWriter, r *http.Request) {
	if err := h.db.DeleteNote(chi.URLParam(r, "id")); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if err := h.notesRefresh(); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func newNoteID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return fmt.Sprintf("%x", b)
}
