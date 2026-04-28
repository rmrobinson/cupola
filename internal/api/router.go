package api

import (
	"io/fs"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/rmrobinson/cupola/internal/collector"
	"github.com/rmrobinson/cupola/internal/collector/gtfsrt"
	"github.com/rmrobinson/cupola/internal/store"
	"github.com/rmrobinson/cupola/internal/tiles"
)

// Handler holds shared dependencies for all API handlers.
type Handler struct {
	registry     *collector.Registry
	store        *store.StateStore
	db           *store.SQLiteStore
	subs         *store.SubscriptionManager
	notesRefresh func() error   // called after every notes mutation to push SSE
	tileHandler  *tiles.Handler // nil until tiles are ready
	frontend     fs.FS          // embedded static files; nil disables file serving
	agencies     []*gtfsrt.Agency
	homeLat      float64
	homeLon      float64
}

func NewHandler(
	registry *collector.Registry,
	stateStore *store.StateStore,
	db *store.SQLiteStore,
	subs *store.SubscriptionManager,
	notesRefresh func() error,
	tileHandler *tiles.Handler,
	frontend fs.FS,
	agencies []*gtfsrt.Agency,
	homeLat, homeLon float64,
) *Handler {
	return &Handler{
		registry:     registry,
		store:        stateStore,
		db:           db,
		subs:         subs,
		notesRefresh: notesRefresh,
		tileHandler:  tileHandler,
		frontend:     frontend,
		agencies:     agencies,
		homeLat:      homeLat,
		homeLon:      homeLon,
	}
}

// Router builds and returns the chi router with all routes mounted.
func (h *Handler) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(corsMiddleware)

	r.Get("/api/v1/domains", h.getDomains)
	r.Get("/api/v1/state/{domain}", h.getState)
	r.Get("/api/v1/stream", h.getStream)

	r.Post("/api/v1/subscriptions", h.createSubscription)
	r.Delete("/api/v1/subscriptions/{widgetID}", h.deleteSubscription)

	r.Get("/api/v1/profiles", h.listProfiles)
	r.Post("/api/v1/profiles", h.createProfile)
	r.Get("/api/v1/profiles/{id}", h.getProfile)
	r.Delete("/api/v1/profiles/{id}", h.deleteProfile)

	r.Get("/api/v1/notes", h.listNotes)
	r.Post("/api/v1/notes", h.createNote)
	r.Patch("/api/v1/notes/{id}", h.updateNote)
	r.Delete("/api/v1/notes/{id}", h.deleteNote)

	r.Get("/api/v1/transit/agencies", h.getTransitAgencies)
	r.Get("/api/v1/transit/agencies/{agencyID}/routes", h.getTransitRoutes)
	r.Get("/api/v1/transit/agencies/{agencyID}/routes/{routeID}/stops", h.getTransitStops)

	r.Get("/tiles/{z}/{x}/{y}", h.getTile)

	if h.frontend != nil {
		r.Handle("/*", http.FileServer(http.FS(h.frontend)))
	}

	return r
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
