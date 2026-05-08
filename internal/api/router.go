package api

import (
	"io/fs"
	"net/http"
	"net/url"
	"strings"

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
	agencies     *gtfsrt.AgencyManager
	homeLat      float64
	homeLon      float64
	countryCode  string
	cspImgSrc    []string
}

func NewHandler(
	registry *collector.Registry,
	stateStore *store.StateStore,
	db *store.SQLiteStore,
	subs *store.SubscriptionManager,
	notesRefresh func() error,
	tileHandler *tiles.Handler,
	frontend fs.FS,
	agencies *gtfsrt.AgencyManager,
	homeLat, homeLon float64,
	countryCode string,
	cspImgSrc []string,
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
		countryCode:  countryCode,
		cspImgSrc:    cspImgSrc,
	}
}

// Router builds and returns the chi router with all routes mounted.
func (h *Handler) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(corsMiddleware)
	r.Use(h.securityHeadersMiddleware)

	r.Get("/api/v1/config", h.getConfig)
	r.Get("/api/v1/domains", h.getDomains)
	r.Get("/api/v1/state/{domain}", h.getState)
	r.Get("/api/v1/details/{domain}", h.getDetail)
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
	r.Get("/api/v1/transit/agencies/{agencyID}/routes/{routeID}/shape", h.getTransitRouteShape)
	r.Get("/api/v1/transit/agency-configs", h.listTransitAgencyConfigs)
	r.Post("/api/v1/transit/agency-configs", h.createTransitAgencyConfig)
	r.Get("/api/v1/transit/agency-configs/{agencyID}", h.getTransitAgencyConfig)
	r.Patch("/api/v1/transit/agency-configs/{agencyID}", h.updateTransitAgencyConfig)
	r.Delete("/api/v1/transit/agency-configs/{agencyID}", h.deleteTransitAgencyConfig)

	r.Get("/admin", h.getAdminPage)
	r.Get("/admin/", h.getAdminPage)
	r.Route("/api/v1/admin", func(r chi.Router) {
		// Keep admin API routes grouped so future authentication middleware can
		// wrap this boundary without changing endpoint paths.
		r.Get("/collectors", h.getAdminCollectors)
	})

	r.Get("/tiles/local.pmtiles", h.getTileFile)
	r.Get("/tiles/{z}/{x}/{y}", h.getTile)

	if h.frontend != nil {
		r.Handle("/*", http.FileServer(http.FS(h.frontend)))
	}

	return r
}

func (h *Handler) securityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy",
			"default-src 'self'; "+
				"script-src 'self'; "+
				"style-src 'self' 'unsafe-inline'; "+
				"img-src "+h.cspImgSrcDirective()+"; "+
				"connect-src 'self'; "+
				"worker-src 'self'; "+
				"manifest-src 'self'; "+
				"base-uri 'self'; "+
				"object-src 'none'; "+
				"frame-ancestors 'self'")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "same-origin")
		next.ServeHTTP(w, r)
	})
}

func (h *Handler) cspImgSrcDirective() string {
	sources := []string{"'self'", "data:", "blob:"}
	seen := map[string]bool{
		"'self'": true,
		"data:":  true,
		"blob:":  true,
	}
	for _, raw := range h.cspImgSrc {
		src := normalizeCSPSource(raw)
		if src == "" || seen[src] {
			continue
		}
		sources = append(sources, src)
		seen[src] = true
	}
	return strings.Join(sources, " ")
}

func normalizeCSPSource(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.ContainsAny(raw, " \t\r\n;") {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return ""
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return ""
	}
	return u.Scheme + "://" + u.Host
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
