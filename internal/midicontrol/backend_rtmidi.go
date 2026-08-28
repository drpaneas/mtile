//go:build darwin && cgo

package midicontrol

import (
	"fmt"
	"io"
	"sync"

	"gitlab.com/gomidi/midi/v2/drivers"
	"gitlab.com/gomidi/midi/v2/drivers/rtmididrv"
)

type systemBackend struct {
	driver  *rtmididrv.Driver
	mu      sync.Mutex
	inputs  map[string]drivers.In
	outputs map[string]drivers.Out
}

func NewSystemBackend() (Backend, error) {
	driver, err := rtmididrv.New()
	if err != nil {
		return nil, err
	}
	return &systemBackend{driver: driver, inputs: map[string]drivers.In{}, outputs: map[string]drivers.Out{}}, nil
}

func (b *systemBackend) List() ([]Endpoint, error) {
	inputs, err := b.driver.Ins()
	if err != nil {
		return nil, err
	}
	outputs, err := b.driver.Outs()
	if err != nil {
		return nil, err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.inputs, b.outputs = map[string]drivers.In{}, map[string]drivers.Out{}
	endpoints := make([]Endpoint, 0, len(inputs)+len(outputs))
	for _, input := range inputs {
		id := fmt.Sprintf("in:%d:%s", input.Number(), input.String())
		b.inputs[id] = input
		endpoints = append(endpoints, Endpoint{ID: id, Name: input.String(), Input: true})
	}
	for _, output := range outputs {
		id := fmt.Sprintf("out:%d:%s", output.Number(), output.String())
		b.outputs[id] = output
		endpoints = append(endpoints, Endpoint{ID: id, Name: output.String(), Output: true})
	}
	return endpoints, nil
}

func (b *systemBackend) Listen(endpoint Endpoint, receive func([]byte)) (io.Closer, error) {
	b.mu.Lock()
	input := b.inputs[endpoint.ID]
	b.mu.Unlock()
	if input == nil {
		return nil, fmt.Errorf("MIDI input %q disappeared", endpoint.Name)
	}
	if err := input.Open(); err != nil {
		return nil, err
	}
	stop, err := input.Listen(func(message []byte, _ int32) {
		if receive != nil {
			receive(message)
		}
	}, drivers.ListenConfig{SysEx: true, SysExBufferSize: 2048})
	if err != nil {
		_ = input.Close()
		return nil, err
	}
	return &systemInput{port: input, stop: stop}, nil
}

func (b *systemBackend) OpenOutput(endpoint Endpoint) (Output, error) {
	b.mu.Lock()
	output := b.outputs[endpoint.ID]
	b.mu.Unlock()
	if output == nil {
		return nil, fmt.Errorf("MIDI output %q disappeared", endpoint.Name)
	}
	if err := output.Open(); err != nil {
		return nil, err
	}
	return &systemOutput{port: output}, nil
}

func (b *systemBackend) Close() error { return b.driver.Close() }

type systemInput struct {
	port drivers.In
	stop func()
}

func (i *systemInput) Close() error {
	if i.stop != nil {
		i.stop()
		i.stop = nil
	}
	if i.port != nil {
		return i.port.Close()
	}
	return nil
}

type systemOutput struct{ port drivers.Out }

func (o *systemOutput) Send(message []byte) error { return o.port.Send(message) }
func (o *systemOutput) Close() error              { return o.port.Close() }
