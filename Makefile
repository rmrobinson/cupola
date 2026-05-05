FRONTEND_VENDOR := cmd/cupola/frontend/js/vendor
LEAFLET_VERSION := 1.9.4
PROTOMAPS_LEAFLET_VERSION := 5.1.0

.PHONY: build test lint clean vendor-frontend

build: $(FRONTEND_VENDOR)/.stamp
	go build ./cmd/cupola

# vendor-frontend: always re-downloads; build uses stamp so it's skipped when up to date
vendor-frontend:
	mkdir -p $(FRONTEND_VENDOR)
	curl -sLf "https://unpkg.com/leaflet@$(LEAFLET_VERSION)/dist/leaflet.js" \
	     -o $(FRONTEND_VENDOR)/leaflet.js
	curl -sLf "https://unpkg.com/leaflet@$(LEAFLET_VERSION)/dist/leaflet.css" \
	     -o $(FRONTEND_VENDOR)/leaflet.css
	curl -sLf "https://unpkg.com/protomaps-leaflet@$(PROTOMAPS_LEAFLET_VERSION)/dist/protomaps-leaflet.js" \
	     -o $(FRONTEND_VENDOR)/protomaps-leaflet.js
	touch $(FRONTEND_VENDOR)/.stamp

$(FRONTEND_VENDOR)/.stamp:
	$(MAKE) vendor-frontend

test:
	go test ./...

lint:
	golangci-lint run ./...

clean:
	rm -rf $(FRONTEND_VENDOR)
	rm -f cupola
