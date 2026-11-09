package sensor

import (
	"slices"
	"sync"
	"time"
)

// Key is one of the joystick's five inputs.
type Key string

// The joystick's keys: four directions and the centre press.
const (
	KeyUp     Key = "up"
	KeyDown   Key = "down"
	KeyLeft   Key = "left"
	KeyRight  Key = "right"
	KeyCenter Key = "center"
)

// AllKeys lists the keys in display order.
var AllKeys = []Key{KeyUp, KeyDown, KeyLeft, KeyRight, KeyCenter}

// recentPresses is how many presses the joystick state keeps for the page.
const recentPresses = 10

// KeyPress is one press of a key.
type KeyPress struct {
	Key Key       `json:"key"`
	At  time.Time `json:"at"`
}

// JoystickState is what the page and the API show: the keys held now, how
// often each was pressed since the daemon started, and the latest presses.
type JoystickState struct {
	Pressed []Key          `json:"pressed"`
	Counts  map[Key]uint64 `json:"counts"`
	Recent  []KeyPress     `json:"recent"`
}

// Presses turns polled key states into presses: a key counts once when it
// goes down, however long it is held. It is safe for concurrent use.
type Presses struct {
	mu     sync.Mutex
	down   map[Key]bool
	counts map[Key]uint64
	recent []KeyPress
}

// NewPresses returns an empty tracker.
func NewPresses() *Presses {
	return &Presses{down: map[Key]bool{}, counts: map[Key]uint64{}}
}

// Update records the keys held at now and returns the ones that were not held
// at the previous update.
func (p *Presses) Update(now time.Time, held []Key) []Key {
	p.mu.Lock()
	defer p.mu.Unlock()

	next := make(map[Key]bool, len(held))
	var pressed []Key

	for _, k := range held {
		next[k] = true

		if !p.down[k] {
			pressed = append(pressed, k)
			p.counts[k]++
			p.recent = append(p.recent, KeyPress{Key: k, At: now})
		}
	}

	if over := len(p.recent) - recentPresses; over > 0 {
		p.recent = append([]KeyPress(nil), p.recent[over:]...)
	}

	p.down = next

	return pressed
}

// State returns a copy of the current state, newest press first.
func (p *Presses) State() JoystickState {
	p.mu.Lock()
	defer p.mu.Unlock()

	st := JoystickState{
		Pressed: []Key{},
		Counts:  make(map[Key]uint64, len(AllKeys)),
		Recent:  make([]KeyPress, 0, len(p.recent)),
	}

	for _, k := range AllKeys {
		st.Counts[k] = p.counts[k]

		if p.down[k] {
			st.Pressed = append(st.Pressed, k)
		}
	}

	for _, kp := range slices.Backward(p.recent) {
		st.Recent = append(st.Recent, kp)
	}

	return st
}
