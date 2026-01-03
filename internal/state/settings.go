package state

import "sync"

type Settings struct {
	Enabled        bool   `json:"enabled"`
	ServerAddress  string `json:"serverAddress"`
	ServerPort     int    `json:"serverPort"`
	ServerLogin    string `json:"serverLogin"`
	ServerPassword string `json:"serverPassword"`
}

type SettingsState struct {
	value *Settings
	mux   sync.Mutex

	subscribers []func(newValue *Settings)
}

func (s *SettingsState) Get() *Settings {
	s.mux.Lock()
	defer s.mux.Unlock()
	return s.value
}

func (s *SettingsState) Set(value *Settings) {
	s.mux.Lock()
	s.value = value
	s.mux.Unlock()
	s.notifySubscribers()
}

func (s *SettingsState) Update(fn func(value *Settings) *Settings) {
	s.mux.Lock()
	defer s.mux.Unlock()
	s.value = fn(s.value)
	s.notifySubscribers()
}

func (s *SettingsState) Subscribe(subscriber func(newValue *Settings)) {
	s.mux.Lock()
	defer s.mux.Unlock()
	s.subscribers = append(s.subscribers, subscriber)
}

func (s *SettingsState) notifySubscribers() {
	s.mux.Lock()
	subscribers := make([]func(newValue *Settings), len(s.subscribers))
	copy(subscribers, s.subscribers)
	value := s.value
	s.mux.Unlock()

	for _, subscriber := range subscribers {
		subscriber(value)
	}
}

func NewSettingsState(initialValue *Settings) *SettingsState {
	return &SettingsState{value: initialValue, subscribers: []func(newValue *Settings){}}
}
