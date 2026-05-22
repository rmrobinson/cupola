FRONTEND_VENDOR := cmd/cupola/frontend/js/vendor
LEAFLET_VERSION := 1.9.4
PROTOMAPS_LEAFLET_VERSION := 5.1.0
SCREENSHOT_BASE_URL ?= http://localhost:8181
SCREENSHOT_DIR := docs/screenshots

.PHONY: build test lint clean vendor-frontend screenshots

build: $(FRONTEND_VENDOR)/.stamp
	go build -o cupolad ./cmd/cupola

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

screenshots:
	mkdir -p $(SCREENSHOT_DIR)
	curl -fsS -X POST -H "Content-Type: application/json" \
	     --data-binary @docs/examples/readme-basic-profile.json \
	     "$(SCREENSHOT_BASE_URL)/api/v1/profiles"
	curl -fsS -X POST -H "Content-Type: application/json" \
	     --data-binary @docs/examples/readme-operations-profile.json \
	     "$(SCREENSHOT_BASE_URL)/api/v1/profiles"
	npx --yes playwright@latest install chromium
	npx --yes playwright@latest screenshot --browser=chromium --viewport-size=1440,900 \
	     --wait-for-timeout=3000 "$(SCREENSHOT_BASE_URL)/?profile=readme-basic&kiosk=1" \
	     "$(SCREENSHOT_DIR)/basic-dashboard.png"
	npx --yes playwright@latest screenshot --browser=chromium --viewport-size=1440,900 \
	     --wait-for-timeout=3000 "$(SCREENSHOT_BASE_URL)/?profile=readme-operations&kiosk=1" \
	     "$(SCREENSHOT_DIR)/operations-dashboard.png"

clean:
	rm -rf $(FRONTEND_VENDOR)
	rm -f cupola
