package main

import (
	"sync"
	"time"
)

// PowerMode mirrors the Roku power-mode field in /query/device-info.
type PowerMode string

const (
	PowerOn      PowerMode = "PowerOn"
	PowerStandby PowerMode = "Standby"
)

// App represents an installed channel / app on the simulated device.
type App struct {
	ID      string
	Name    string
	Type    string // "appl" | "tvin"
	Version string
}

// EventKind labels the type of ECP interaction logged in the event ring.
type EventKind string

const (
	EventKeypress EventKind = "KEYPRESS"
	EventKeydown  EventKind = "KEYDOWN "
	EventKeyup    EventKind = "KEYUP   "
	EventLaunch   EventKind = "LAUNCH  "
	EventQuery    EventKind = "QUERY   "
)

// Event is one item in the simulator's event log.
type Event struct {
	Time   time.Time
	Kind   EventKind
	Detail string
	Remote string // client IP:port
}

const maxEvents = 50

// State holds all mutable simulator state and is safe for concurrent access.
type State struct {
	mu sync.RWMutex

	// Power
	Power PowerMode

	// Active foreground app; ID == "" means the Roku home screen.
	ActiveApp App

	// Installed apps
	Apps []App

	// Ring buffer of recent events (capped at maxEvents)
	Events []Event

	// Device identity (populated from CLI flags / defaults)
	DeviceName      string
	ModelName       string
	ModelNumber     string
	SerialNumber    string
	UDN             string
	SoftwareVersion string
	Language        string
	Country         string
}

// defaultApps is a plausible NOW TV / Sky Stream UK channel list.
var defaultApps = []App{
	{ID: "837", Name: "NOW", Type: "appl", Version: "10.1.0"},
	{ID: "12", Name: "Netflix", Type: "appl", Version: "4.1.218"},
	{ID: "13", Name: "Prime Video", Type: "appl", Version: "6.3.2"},
	{ID: "2285", Name: "Disney+", Type: "appl", Version: "1.2.0"},
	{ID: "195316", Name: "YouTube", Type: "appl", Version: "2.0.8"},
	{ID: "3423", Name: "BBC iPlayer", Type: "appl", Version: "8.7.3"},
	{ID: "13871", Name: "ITVX", Type: "appl", Version: "3.2.1"},
	{ID: "41468", Name: "My5", Type: "appl", Version: "4.1.0"},
	{ID: "34376", Name: "Spotify", Type: "appl", Version: "1.3.2"},
	{ID: "12943", Name: "Apple TV", Type: "appl", Version: "1.0.5"},
	{ID: "2594", Name: "Plex", Type: "appl", Version: "7.34.2"},
	{ID: "tvinput.hdmi1", Name: "HDMI 1", Type: "tvin", Version: "1.0.0"},
	{ID: "tvinput.hdmi2", Name: "HDMI 2", Type: "tvin", Version: "1.0.0"},
}

// newState builds an initial State with sensible defaults.
func newState(deviceName, modelName, modelNumber, serial, udn, swVersion, lang, country string) *State {
	return &State{
		Power:           PowerOn,
		ActiveApp:       App{ID: "", Name: "Roku", Type: "appl", Version: ""},
		Apps:            defaultApps,
		DeviceName:      deviceName,
		ModelName:       modelName,
		ModelNumber:     modelNumber,
		SerialNumber:    serial,
		UDN:             udn,
		SoftwareVersion: swVersion,
		Language:        lang,
		Country:         country,
	}
}

// AddEvent appends an event, dropping the oldest when the ring is full.
func (s *State) AddEvent(e Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Events = append(s.Events, e)
	if len(s.Events) > maxEvents {
		s.Events = s.Events[len(s.Events)-maxEvents:]
	}
}

// Keypress processes a key press, updating power state where relevant.
// Returns true if the event changes displayable state.
func (s *State) Keypress(key, remote string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	changed := false
	switch key {
	case "PowerOff":
		if s.Power != PowerStandby {
			s.Power = PowerStandby
			changed = true
		}
	case "PowerOn":
		if s.Power != PowerOn {
			s.Power = PowerOn
			changed = true
		}
	case "Power":
		if s.Power == PowerOn {
			s.Power = PowerStandby
		} else {
			s.Power = PowerOn
		}
		changed = true
	case "Home":
		if s.ActiveApp.ID != "" {
			s.ActiveApp = App{ID: "", Name: "Roku", Type: "appl", Version: ""}
			changed = true
		}
	}

	s.Events = appendEvent(s.Events, Event{
		Time:   time.Now(),
		Kind:   EventKeypress,
		Detail: key,
		Remote: remote,
	})
	return changed
}

// Keydown logs a key-down event without changing state.
func (s *State) Keydown(key, remote string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Events = appendEvent(s.Events, Event{
		Time:   time.Now(),
		Kind:   EventKeydown,
		Detail: key,
		Remote: remote,
	})
}

// Keyup logs a key-up event without changing state.
func (s *State) Keyup(key, remote string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Events = appendEvent(s.Events, Event{
		Time:   time.Now(),
		Kind:   EventKeyup,
		Detail: key,
		Remote: remote,
	})
}

// Launch switches the active app. Returns the app name (or "unknown").
func (s *State) Launch(appID, remote string) string {
	s.mu.Lock()
	defer s.mu.Unlock()

	app := App{ID: appID, Name: "unknown", Type: "appl", Version: "1.0.0"}
	for _, a := range s.Apps {
		if a.ID == appID {
			app = a
			break
		}
	}
	s.ActiveApp = app

	s.Events = appendEvent(s.Events, Event{
		Time:   time.Now(),
		Kind:   EventLaunch,
		Detail: app.Name + " (id=" + appID + ")",
		Remote: remote,
	})
	return app.Name
}

// LogQuery records a query endpoint hit.
func (s *State) LogQuery(endpoint, remote string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Events = appendEvent(s.Events, Event{
		Time:   time.Now(),
		Kind:   EventQuery,
		Detail: endpoint,
		Remote: remote,
	})
}

// Snapshot returns an immutable copy of the state fields needed for display.
type Snapshot struct {
	Power     PowerMode
	ActiveApp App
	Events    []Event
}

func (s *State) Snapshot() Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	evCopy := make([]Event, len(s.Events))
	copy(evCopy, s.Events)
	return Snapshot{
		Power:     s.Power,
		ActiveApp: s.ActiveApp,
		Events:    evCopy,
	}
}

// FindApp returns the App with the given ID, or a zero App if not found.
func (s *State) FindApp(id string) (App, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, a := range s.Apps {
		if a.ID == id {
			return a, true
		}
	}
	return App{}, false
}

func appendEvent(events []Event, e Event) []Event {
	events = append(events, e)
	if len(events) > maxEvents {
		events = events[len(events)-maxEvents:]
	}
	return events
}
