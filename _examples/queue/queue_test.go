package queue

import (
	"fmt"
	"log"
	"testing"
	"time"

	"honnef.co/go/quickcheck"
)

// The model describing how a queue should behave

type Model struct {
	size     int
	elements []int
	queue    *Queue
}

// Preconditions determine whether a function is allowed to be called
// in the current state (as per the contract of the API)
//
// Calls are responsible for calling the function(s) under test
//
// Postconditions check if the function call's result matches the expected model
//
// Next updates the model

func (m *Model) InitCall(size uint8) {
	// Init sets up the model at the start of a test run

	m.size = int(size % 16)
	m.size = 1 // XXX, makes testing the prototype easier
	m.queue = New(m.size)
}

func (m *Model) AddPre(from, to string, args []interface{}) bool { return m.size > len(m.elements) }
func (m *Model) AddCall(v int)                                   { m.queue.Add(v) }
func (m *Model) AddNext(from, to string, args []interface{}, ret []interface{}) {
	m.elements = append(m.elements, args[0].(int))
}

func (m *Model) GetPre(from, to string, args []interface{}) bool { return len(m.elements) > 0 }
func (m *Model) GetCall() int                                    { return m.queue.Get() }
func (m *Model) GetPost(from, to string, args []interface{}, ret []interface{}) bool {
	return ret[0].(int) == m.elements[0]
}
func (m *Model) GetNext(from, to string, args []interface{}, ret []interface{}) {
	// XXX this leaks memory, fix later
	m.elements = m.elements[1:]
}

func (m *Model) SizeCall() int { return m.queue.Size() }
func (m *Model) SizePost(from, to string, args []interface{}, ret []interface{}) bool {
	return ret[0].(int) == len(m.elements)
}

func logSteps(t *testing.T, ss []quickcheck.Step) {
	// TODO this functionality should be in the quickcheck package
	for _, s := range ss {
		t.Log("\t" + formatStep(s))
	}
}

func formatStep(s quickcheck.Step) string {
	// TODO this functionality should be in the quickcheck package
	return fmt.Sprintf("%s(%v)", s.Method, s.Args)
}

func TestQueue(t *testing.T) {
	seed := time.Now().UnixNano()
	log.Println("seed:", seed)
	fsm := quickcheck.NewFSM(seed)

	fsm.Transition("state0", "state1", []string{"Init"})
	fsm.Transition("state1", "state1", []string{"Add", "Get", "Size"})

	steps := fsm.Run(Model{})
	// TODO quickcheck should minimize for us
	// TODO quickcheck should give up if it cannot find a bug
	t.Log("Call chain:")
	logSteps(t, steps)
	steps = fsm.Minimize(steps, Model{})
	t.Log("Minimized call chain:")
	logSteps(t, steps)
	t.Fail()
}
