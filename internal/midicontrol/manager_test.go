package midicontrol

import (
	"io"
	"sync"
	"testing"
	"time"
)

type fakeBackend struct {
	mu        sync.Mutex
	endpoints []Endpoint
	listeners map[string]func([]byte)
	outputs   []*fakeOutput
}

func (f *fakeBackend) List() ([]Endpoint, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Endpoint(nil), f.endpoints...), nil
}

func (f *fakeBackend) Listen(endpoint Endpoint, receive func([]byte)) (io.Closer, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.listeners == nil {
		f.listeners = map[string]func([]byte){}
	}
	f.listeners[endpoint.ID] = receive
	return closeFunc(func() error {
		f.mu.Lock()
		delete(f.listeners, endpoint.ID)
		f.mu.Unlock()
		return nil
	}), nil
}

func (f *fakeBackend) OpenOutput(endpoint Endpoint) (Output, error) {
	output := &fakeOutput{}
	f.mu.Lock()
	f.outputs = append(f.outputs, output)
	f.mu.Unlock()
	return output, nil
}

func (f *fakeBackend) Close() error { return nil }

func (f *fakeBackend) emit(id string, message []byte) {
	f.mu.Lock()
	listener := f.listeners[id]
	f.mu.Unlock()
	if listener != nil {
		listener(message)
	}
}

type fakeOutput struct {
	mu       sync.Mutex
	messages [][]byte
}

func (f *fakeOutput) Send(message []byte) error {
	f.mu.Lock()
	f.messages = append(f.messages, append([]byte(nil), message...))
	f.mu.Unlock()
	return nil
}
func (f *fakeOutput) Close() error { return nil }

type closeFunc func() error

func (f closeFunc) Close() error { return f() }

func TestManagerHotplugInitializesRoutesAndSafelyDisconnects(t *testing.T) {
	backend := &fakeBackend{endpoints: []Endpoint{
		{ID: "keys-in", Name: "Launchkey Mini MK4 25 MIDI Out", Input: true},
		{ID: "keys-mirror", Name: "Launchkey Mini MK4 25 MIDI Out Mirror", Input: true},
		{ID: "daw-in", Name: "Launchkey Mini MK4 25 DAW Out", Input: true},
		{ID: "keys-out", Name: "Launchkey Mini MK4 25 MIDI In", Output: true},
		{ID: "daw-out", Name: "Launchkey Mini MK4 25 DAW In", Output: true},
	}}
	commands := make(chan Command, 16)
	manager := NewManager(backend, LaunchkeyMini25MK4{}, func(command Command) { commands <- command })
	manager.interval = 5 * time.Millisecond
	if err := manager.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := manager.Close(); err != nil {
			t.Errorf("close manager: %v", err)
		}
	}()
	waitCommand := func(kind CommandKind) Command {
		t.Helper()
		deadline := time.After(time.Second)
		for {
			select {
			case command := <-commands:
				if command.Kind == kind {
					return command
				}
			case <-deadline:
				t.Fatalf("timed out waiting for command %d", kind)
			}
		}
	}
	if connected := waitCommand(CommandConnected); connected.Name != "Launchkey Mini MK4 25" || connected.Detail != "" {
		t.Fatalf("connected command = %#v", connected)
	}
	backend.mu.Lock()
	if len(backend.outputs) != 1 || len(backend.outputs[0].messages) == 0 {
		backend.mu.Unlock()
		t.Fatal("hotplug did not initialize the Launchkey output")
	}
	if len(backend.listeners) != 2 || backend.listeners["keys-mirror"] != nil {
		backend.mu.Unlock()
		t.Fatalf("opened MIDI listeners = %#v, want one keys and one DAW input", backend.listeners)
	}
	backend.mu.Unlock()
	backend.emit("keys-in", []byte{0x90, 64, 100})
	if note := waitCommand(CommandPerformance); note.Performance.Note != 64 {
		t.Fatalf("routed note = %#v", note)
	}
	backend.mu.Lock()
	backend.endpoints = nil
	backend.mu.Unlock()
	allOff := waitCommand(CommandPerformance)
	if allOff.Performance.Kind != 3 {
		t.Fatalf("disconnect performance command = %#v", allOff)
	}
	waitCommand(CommandDisconnected)
}

func TestManagerReportsDegradedConnectionInsteadOfSilentlyFailing(t *testing.T) {
	backend := &fakeBackend{endpoints: []Endpoint{
		{ID: "keys-in", Name: "Launchkey Mini MK4 25 MIDI Out", Input: true},
	}}
	commands := make(chan Command, 16)
	manager := NewManager(backend, LaunchkeyMini25MK4{}, func(command Command) { commands <- command })
	manager.interval = time.Hour
	if err := manager.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = manager.Close() }()
	deadline := time.After(time.Second)
	for {
		select {
		case command := <-commands:
			if command.Kind == CommandProblem {
				if command.Detail == "" {
					t.Fatal("problem command omitted detail")
				}
				return
			}
		case <-deadline:
			t.Fatal("timed out waiting for degraded-connection detail")
		}
	}
}
