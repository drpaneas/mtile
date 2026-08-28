package midicontrol

import (
	"errors"
	"io"
	"sort"
	"sync"
	"time"

	"github.com/drpaneas/mtile/internal/performance"
)

const defaultScanInterval = time.Second

// Manager owns hot-plug discovery and controller feedback. Performance
// commands are delivered directly from the MIDI callback; UI commands are
// also safe to queue because the consumer decides how to dispatch each kind.
type Manager struct {
	backend Backend
	profile Profile
	handle  func(Command)

	mu       sync.Mutex
	state    ControllerState
	output   Output
	inputs   []io.Closer
	portKey  string
	name     string
	running  bool
	stop     chan struct{}
	done     chan struct{}
	interval time.Duration
	origin   time.Time
}

func NewManager(backend Backend, profile Profile, handle func(Command)) *Manager {
	return &Manager{backend: backend, profile: profile, handle: handle, interval: defaultScanInterval, origin: time.Now()}
}

func (m *Manager) Start() error {
	if m == nil || m.backend == nil || m.profile == nil {
		return errors.New("MIDI manager has no backend or controller profile")
	}
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return nil
	}
	m.running, m.stop, m.done = true, make(chan struct{}), make(chan struct{})
	m.mu.Unlock()
	go m.run()
	return nil
}

func (m *Manager) run() {
	defer close(m.done)
	m.scan()
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			m.scan()
		case <-m.stop:
			m.disconnect(true)
			return
		}
	}
}

func (m *Manager) scan() {
	ports, err := m.backend.List()
	if err != nil {
		return
	}
	matched := ports[:0]
	for _, port := range ports {
		if m.profile.Matches(port.Name) {
			matched = append(matched, port)
		}
	}
	sort.Slice(matched, func(i, j int) bool { return matched[i].ID < matched[j].ID })
	key := ""
	for _, port := range matched {
		key += port.ID + "\x00"
	}
	m.mu.Lock()
	same := key == m.portKey
	m.mu.Unlock()
	if same {
		return
	}
	m.disconnect(false)
	if len(matched) == 0 {
		return
	}
	var inputs []io.Closer
	var output Output
	name := matched[0].Name
	for _, port := range matched {
		role := m.profile.Role(port.Name)
		if port.Input {
			input, listenErr := m.backend.Listen(port, func(data []byte) {
				m.dispatch(m.profile.Decode(role, data, time.Since(m.origin)))
			})
			if listenErr == nil {
				inputs = append(inputs, input)
			}
		}
		if port.Output && output == nil && role == PortDAW {
			output, _ = m.backend.OpenOutput(port)
		}
	}
	if output == nil {
		for _, port := range matched {
			if port.Output {
				output, _ = m.backend.OpenOutput(port)
				break
			}
		}
	}
	if len(inputs) == 0 {
		if output != nil {
			_ = output.Close()
		}
		return
	}
	m.mu.Lock()
	m.inputs, m.output, m.portKey, m.name = inputs, output, key, name
	state := m.state
	m.mu.Unlock()
	m.send(m.profile.Initialize())
	m.send(m.profile.Feedback(state))
	m.emit(Command{Kind: CommandConnected, Name: name})
}

func (m *Manager) dispatch(batch CommandBatch) {
	for _, command := range batch.Slice() {
		m.emit(command)
	}
}

func (m *Manager) emit(command Command) {
	if m.handle != nil {
		m.handle(command)
	}
}

func (m *Manager) SetState(state ControllerState) {
	if m == nil {
		return
	}
	m.mu.Lock()
	if m.state == state {
		m.mu.Unlock()
		return
	}
	m.state = state
	connected := m.portKey != ""
	m.mu.Unlock()
	if connected {
		m.send(m.profile.Feedback(state))
	}
}

func (m *Manager) Connected() (bool, string) {
	if m == nil {
		return false, ""
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.portKey != "", m.name
}

func (m *Manager) send(messages [][]byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.output == nil {
		return
	}
	for _, message := range messages {
		if err := m.output.Send(message); err != nil {
			return
		}
	}
}

func (m *Manager) disconnect(shutdown bool) {
	m.mu.Lock()
	wasConnected := m.portKey != ""
	output := m.output
	inputs := m.inputs
	name := m.name
	m.output, m.inputs, m.portKey, m.name = nil, nil, "", ""
	m.mu.Unlock()
	if output != nil {
		if shutdown {
			for _, message := range m.profile.Shutdown() {
				_ = output.Send(message)
			}
		}
		_ = output.Close()
	}
	for _, input := range inputs {
		_ = input.Close()
	}
	if wasConnected {
		m.emit(Command{Kind: CommandPerformance, Performance: performanceAllNotesOff()})
		m.emit(Command{Kind: CommandDisconnected, Name: name})
	}
}

func performanceAllNotesOff() performance.PerformanceEvent {
	return performance.PerformanceEvent{Kind: performance.AllNotesOff, Source: performance.SourceController}
}

func (m *Manager) Close() error {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	if !m.running {
		m.mu.Unlock()
		return m.backend.Close()
	}
	stop, done := m.stop, m.done
	m.running = false
	m.mu.Unlock()
	close(stop)
	<-done
	return m.backend.Close()
}
