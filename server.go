package main

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net"
	"net/http"
	"strings"
)

// recovery wraps an http.Handler and recovers from any panics, returning 500.
func recovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				fmt.Printf("[panic] %v\n", rec)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// newECPServer sets up all Roku ECP routes and returns the ServeMux.
func newECPServer(s *State, events chan<- Event) http.Handler {
	mux := http.NewServeMux()

	// ── Commands ────────────────────────────────────────────────────────────
	// POST /keypress/{key}
	mux.HandleFunc("/keypress/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}
		key := strings.TrimPrefix(r.URL.Path, "/keypress/")
		if key == "" {
			http.Error(w, "Bad Request: missing key", http.StatusBadRequest)
			return
		}
		remote := remoteIP(r)
		s.Keypress(key, remote)
		events <- Event{Kind: EventKeypress, Detail: key, Remote: remote}
		w.WriteHeader(http.StatusOK)
	})

	// POST /keydown/{key}
	mux.HandleFunc("/keydown/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}
		key := strings.TrimPrefix(r.URL.Path, "/keydown/")
		remote := remoteIP(r)
		s.Keydown(key, remote)
		events <- Event{Kind: EventKeydown, Detail: key, Remote: remote}
		w.WriteHeader(http.StatusOK)
	})

	// POST /keyup/{key}
	mux.HandleFunc("/keyup/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}
		key := strings.TrimPrefix(r.URL.Path, "/keyup/")
		remote := remoteIP(r)
		s.Keyup(key, remote)
		events <- Event{Kind: EventKeyup, Detail: key, Remote: remote}
		w.WriteHeader(http.StatusOK)
	})

	// POST /launch/{appId}
	mux.HandleFunc("/launch/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}
		appID := strings.TrimPrefix(r.URL.Path, "/launch/")
		if appID == "" {
			http.Error(w, "Bad Request: missing app id", http.StatusBadRequest)
			return
		}
		remote := remoteIP(r)
		name := s.Launch(appID, remote)
		events <- Event{Kind: EventLaunch, Detail: name + " (id=" + appID + ")", Remote: remote}
		w.WriteHeader(http.StatusOK)
	})

	// ── Queries ─────────────────────────────────────────────────────────────
	// GET /query/apps
	mux.HandleFunc("/query/apps", func(w http.ResponseWriter, r *http.Request) {
		remote := remoteIP(r)
		s.LogQuery("apps", remote)
		events <- Event{Kind: EventQuery, Detail: "apps", Remote: remote}
		w.Header().Set("Content-Type", "text/xml; charset=utf-8")
		fmt.Fprint(w, buildAppsXML(s))
	})

	// GET /query/active-app
	mux.HandleFunc("/query/active-app", func(w http.ResponseWriter, r *http.Request) {
		remote := remoteIP(r)
		s.LogQuery("active-app", remote)
		events <- Event{Kind: EventQuery, Detail: "active-app", Remote: remote}
		w.Header().Set("Content-Type", "text/xml; charset=utf-8")
		fmt.Fprint(w, buildActiveAppXML(s))
	})

	// GET /query/device-info
	mux.HandleFunc("/query/device-info", func(w http.ResponseWriter, r *http.Request) {
		remote := remoteIP(r)
		s.LogQuery("device-info", remote)
		events <- Event{Kind: EventQuery, Detail: "device-info", Remote: remote}
		w.Header().Set("Content-Type", "text/xml; charset=utf-8")
		fmt.Fprint(w, buildDeviceInfoXML(s))
	})

	// GET /query/icon/{appId}
	mux.HandleFunc("/query/icon/", func(w http.ResponseWriter, r *http.Request) {
		appID := strings.TrimPrefix(r.URL.Path, "/query/icon/")
		remote := remoteIP(r)
		s.LogQuery("icon/"+appID, remote)
		events <- Event{Kind: EventQuery, Detail: "icon/" + appID, Remote: remote}

		data := iconCache.get(appID)
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
		w.Write(data) //nolint:errcheck
	})

	// Root — basic info / health-check
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		s.mu.RLock()
		name := s.DeviceName
		model := s.ModelName
		power := s.Power
		s.mu.RUnlock()

		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprintf(w, "NOW TV Simulator — %s (%s)  power=%s\n", name, model, power)
		fmt.Fprintln(w, "Endpoints: /keypress/:key  /keydown/:key  /keyup/:key  /launch/:id")
		fmt.Fprintln(w, "           /query/apps  /query/active-app  /query/device-info  /query/icon/:id")
	})

	return recovery(mux)
}

// ── XML builders ─────────────────────────────────────────────────────────────

func buildAppsXML(s *State) string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8" ?>` + "\n")
	b.WriteString("<apps>\n")
	for _, a := range s.Apps {
		b.WriteString(fmt.Sprintf(
			`  <app id=%q type=%q version=%q>%s</app>`+"\n",
			a.ID, a.Type, a.Version, xmlEscape(a.Name),
		))
	}
	b.WriteString("</apps>\n")
	return b.String()
}

func buildActiveAppXML(s *State) string {
	s.mu.RLock()
	app := s.ActiveApp
	power := s.Power
	s.mu.RUnlock()

	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8" ?>` + "\n")
	b.WriteString("<active-app>\n")

	if power == PowerStandby || app.ID == "" {
		b.WriteString("  <app>Roku</app>\n")
	} else {
		b.WriteString(fmt.Sprintf(
			`  <app id=%q type=%q version=%q>%s</app>`+"\n",
			app.ID, app.Type, app.Version, xmlEscape(app.Name),
		))
	}
	b.WriteString("</active-app>\n")
	return b.String()
}

func buildDeviceInfoXML(s *State) string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" ?>
<device-info>
  <udn>%s</udn>
  <serial-number>%s</serial-number>
  <device-id>%s</device-id>
  <vendor-name>Roku</vendor-name>
  <model-number>%s</model-number>
  <model-name>%s</model-name>
  <model-region>GB</model-region>
  <is-tv>false</is-tv>
  <is-stick>false</is-stick>
  <supports-ethernet>true</supports-ethernet>
  <wifi-mac>b0:a7:37:96:4d:fb</wifi-mac>
  <ethernet-mac>b0:a7:37:96:4d:fa</ethernet-mac>
  <network-type>wifi</network-type>
  <user-device-name>%s</user-device-name>
  <software-version>%s</software-version>
  <software-build>09021</software-build>
  <secure-device>true</secure-device>
  <language>%s</language>
  <country>%s</country>
  <locale>%s_%s</locale>
  <time-zone>Europe/London</time-zone>
  <time-zone-offset>0</time-zone-offset>
  <power-mode>%s</power-mode>
  <supports-suspend>false</supports-suspend>
  <supports-find-remote>false</supports-find-remote>
  <supports-audio-guide>false</supports-audio-guide>
  <developer-enabled>true</developer-enabled>
  <search-enabled>true</search-enabled>
  <voice-search-enabled>false</voice-search-enabled>
  <notifications-enabled>true</notifications-enabled>
  <notifications-first-use>false</notifications-first-use>
  <supports-private-listening>false</supports-private-listening>
  <headphones-connected>false</headphones-connected>
</device-info>
`,
		s.UDN, s.SerialNumber, s.SerialNumber,
		s.ModelNumber, s.ModelName,
		xmlEscape(s.DeviceName),
		s.SoftwareVersion,
		s.Language, s.Country,
		s.Language, s.Country,
		s.Power,
	)
}

// ── Icon cache + generator ────────────────────────────────────────────────────

// iconStore is a simple in-memory cache so icons are only generated once.
type iconStore struct {
	cache map[string][]byte
}

func newIconStore() *iconStore { return &iconStore{cache: make(map[string][]byte)} }

func (ic *iconStore) get(appID string) []byte {
	if data, ok := ic.cache[appID]; ok {
		return data
	}
	data := generateIcon(appID)
	ic.cache[appID] = data
	return data
}

// iconCache is the process-wide icon cache (icons never change at runtime).
var iconCache = newIconStore()

// appColors maps well-known channel IDs to a recognisable brand colour.
var appColors = map[string]color.RGBA{
	"837":    {R: 0, G: 50, B: 160, A: 255},    // NOW — dark blue
	"12":     {R: 229, G: 9, B: 20, A: 255},    // Netflix red
	"13":     {R: 0, G: 168, B: 225, A: 255},   // Prime blue
	"2285":   {R: 17, G: 60, B: 166, A: 255},   // Disney+ blue
	"195316": {R: 255, G: 0, B: 0, A: 255},     // YouTube red
	"3423":   {R: 255, G: 255, B: 255, A: 255}, // BBC white
	"13871":  {R: 0, G: 98, B: 198, A: 255},    // ITVX blue
	"41468":  {R: 60, G: 120, B: 180, A: 255},  // My5
	"34376":  {R: 30, G: 215, B: 96, A: 255},   // Spotify green
	"12943":  {R: 40, G: 40, B: 40, A: 255},    // Apple TV dark
	"2594":   {R: 229, G: 160, B: 13, A: 255},  // Plex yellow
}

// generateIcon creates a 290×218 placeholder PNG icon for the given app ID.
// Each app gets its brand colour; unknown apps get a deterministic colour
// derived from the ID string.  Results are cached by the caller.
func generateIcon(appID string) []byte {
	const w, h = 290, 218

	col, ok := appColors[appID]
	if !ok {
		var hash uint32
		for _, c := range appID {
			hash = hash*31 + uint32(c)
		}
		col = color.RGBA{
			R: uint8(80 + (hash>>16)&0x7f),
			G: uint8(80 + (hash>>8)&0x7f),
			B: uint8(80 + hash&0x7f),
			A: 255,
		}
	}

	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, col)
		}
	}

	// Draw a simple play-triangle in white.
	cx, cy := w/2, h/2
	triSize := h / 4
	triColor := color.RGBA{R: 255, G: 255, B: 255, A: 180}
	for dy := -triSize; dy <= triSize; dy++ {
		maxDx := triSize - abs(dy)
		for dx := -triSize / 2; dx <= maxDx; dx++ {
			px, py := cx+dx, cy+dy
			if px >= 0 && px < w && py >= 0 && py < h {
				img.SetRGBA(px, py, triColor)
			}
		}
	}

	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// remoteIP extracts the client IP from the request, handling IPv6 and proxies.
func remoteIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return strings.TrimSpace(strings.Split(xff, ",")[0])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// xmlEscape escapes the five XML special characters.
func xmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, `"`, "&quot;")
	s = strings.ReplaceAll(s, "'", "&apos;")
	return s
}
