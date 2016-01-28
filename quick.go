package main

import (
	"fmt"
	"log"
	"math/rand"
	"reflect"
	"testing/quick"
	"time"
)

type Transition struct {
	to      string
	methods []string
}

type FSM struct {
	seed        int64
	rng         *rand.Rand
	state       string
	transitions map[string][]Transition
}

type Step struct {
	State    string
	NewState string
	Method   string
	Args     []interface{}
}

func formatStep(s Step) string {
	return fmt.Sprintf("%s(%v)", s.Method, s.Args)
}

func NewFSM(seed int64) *FSM {
	return &FSM{
		seed:        seed,
		transitions: make(map[string][]Transition),
	}
}

func (fsm *FSM) Transition(from, to string, methods []string) {
	fsm.transitions[from] = append(fsm.transitions[from], Transition{to, methods})
}

func funcall(m reflect.Value, args []reflect.Value) (v []reflect.Value, panicked interface{}) {
	defer func() {
		panicked = recover()
	}()
	ret := m.Call(args)
	return ret, nil
}

func (fsm *FSM) step(s Step, rv reflect.Value) (valid bool, prefail bool, callfail bool) {
	// TODO don't verify step if we just generated it
	if s.State != fsm.state {
		return false, false, false
	}
	found := false
outer:
	for _, t := range fsm.transitions[s.State] {
		if t.to != s.NewState {
			continue
		}
		for _, m := range t.methods {
			if m == s.Method {
				found = true
				break outer
			}
		}
	}
	if !found {
		return false, false, false
	}

	call := rv.MethodByName(s.Method + "Call")
	pre := rv.MethodByName(s.Method + "Pre")
	if pre != (reflect.Value{}) {
		// TODO guard against code having wrong types/arity/...
		if !pre.Call([]reflect.Value{
			reflect.ValueOf(fsm.state),
			reflect.ValueOf(s.NewState),
			reflect.ValueOf(s.Args),
		})[0].Bool() {
			//log.Println("pre-condition failed for", s.Method)
			return false, true, false
		}
	}
	rargs := make([]reflect.Value, 0, len(s.Args))
	for _, v := range s.Args {
		rargs = append(rargs, reflect.ValueOf(v))
	}
	//log.Printf("calling %s(%v)", s.Method+"Call", s.Args)
	defer func() {
		if e := recover(); e != nil {
			log.Println("panic", e)
		}
	}()
	ret, panicked := funcall(call, rargs)
	iret := make([]interface{}, 0, len(ret))
	for _, v := range ret {
		iret = append(iret, v.Interface())
	}
	if panicked != nil {
		log.Printf("postcondition failed for %s = (%v): got panic %v", s.Method, iret, panicked)
		return true, false, true
	}

	post := rv.MethodByName(s.Method + "Post")
	if post != (reflect.Value{}) {

		if !post.Call([]reflect.Value{
			reflect.ValueOf(fsm.state),
			reflect.ValueOf(s.NewState),
			reflect.ValueOf(s.Args),
			reflect.ValueOf(iret),
		})[0].Bool() {
			//log.Printf("postcondition failed for %s = (%v)", s.Method, iret)
			return true, false, true
		}
	}

	next := rv.MethodByName(s.Method + "Next")
	if next != (reflect.Value{}) {
		next.Call(
			[]reflect.Value{
				reflect.ValueOf(fsm.state),
				reflect.ValueOf(s.NewState),
				reflect.ValueOf(s.Args),
				reflect.ValueOf(iret),
			})
	}

	fsm.state = s.NewState
	return true, false, false
}

func (fsm *FSM) Replay(ss []Step, model interface{}) (valid, fail bool) {
	rv := reflect.New(reflect.TypeOf(model))
	fsm.init(rv)
	for _, s := range ss {
		valid, prefail, callfail := fsm.step(s, rv)
		if !valid {
			return false, false
		}
		if prefail {
			// FIXME is this code dead?
			log.Println("precondition failed unexpectedly")
			return false, false
		}
		if callfail {
			return true, true
		}
	}
	return true, false
}

func (fsm *FSM) init(rv reflect.Value) {
	fsm.state = "state0"                         // XXX
	fsm.rng = rand.New(rand.NewSource(fsm.seed)) // XXX we shouldn't need the rng in Replay
}

func (fsm *FSM) Run(v interface{}) []Step {
	var out []Step

	rv := reflect.New(reflect.TypeOf(v))
	fsm.init(rv)

	for {
		if len(fsm.transitions[fsm.state]) == 0 {
			log.Println("dead end")
			return out
		}

		idx := fsm.rng.Intn(len(fsm.transitions[fsm.state]))
		trans := fsm.transitions[fsm.state][idx]
		idx = fsm.rng.Intn(len(trans.methods))
		// TODO check that the name is valid

		step := Step{
			State:    fsm.state,
			NewState: trans.to,
			Method:   trans.methods[idx],
		}

		call := rv.MethodByName(step.Method + "Call")
		for i, n := 0, reflect.TypeOf(call.Interface()).NumIn(); i < n; i++ {
			// TODO optimize reflection
			v, ok := quick.Value(reflect.TypeOf(call.Interface()).In(i), fsm.rng)
			if !ok {
				panic("cannot generate value") // XXX
			}
			step.Args = append(step.Args, v.Interface())
		}

		_, prefail, callfail := fsm.step(step, rv)
		if prefail {
			continue
		}
		out = append(out, step)
		if callfail {
			return out
		}
	}
}

// The implementation of our buggy Queue that we want to test

type Queue struct {
	r, w     int
	size     int
	elements []int
}

func (q *Queue) Add(v int) {
	q.elements[q.w] = v
	q.w = (q.w + 1) % q.size
}

func (q *Queue) Get() int {
	v := q.elements[q.r]
	q.r = (q.r + 1) % q.size
	return v
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func (q *Queue) Size() int {
	// this implementation is purposefully buggy.
	return abs(q.w-q.r) % q.size
}

// The model describing how a queue should work

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

	m.size = int(size)
	// m.size = 1 // XXX, makes testing the prototype easier
	m.queue = &Queue{
		size:     m.size + 1,
		elements: make([]int, m.size+1),
	}
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

func logSteps(ss []Step) {
	for _, s := range ss {
		log.Println("\t" + formatStep(s))
	}
}

func main() {
	seed := time.Now().UnixNano()
	// seed = 1453987917457171993
	// seed = 1454013752742354501
	log.Println("seed:", seed)
	fsm := NewFSM(seed)
	// Our queue has a single state, in which all its methods can be
	// called repeatedly in any order
	fsm.Transition("state0", "state1", []string{"Init"})
	fsm.Transition("state1", "state1", []string{"Add", "Get", "Size"})
	fsm.state = "state0" // XXX set the initial state. we'll want an API on FSM2 for that.

	steps := fsm.Run(Model{})

	log.Println("Failure:")
	logSteps(steps)

	f := func(d []Step) Result {
		valid, fail := fsm.Replay(d, Model{})
		if !valid {
			return Unresolved
		}
		if fail {
			return Fail
		}
		return Pass
	}
	minimized := Minimize(steps, f)
	log.Println("Minimized to:")
	logSteps(minimized)
}
