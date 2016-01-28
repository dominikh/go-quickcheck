package main

import (
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
	rng         *rand.Rand
	state       string
	transitions map[string][]Transition
}

func NewFSM(seed int64) *FSM {
	return &FSM{
		rng:         rand.New(rand.NewSource(seed)),
		transitions: make(map[string][]Transition),
	}
}

func (fsm *FSM) Transition(from, to string, methods []string) {
	fsm.transitions[from] = append(fsm.transitions[from], Transition{to, methods})
}

func (fsm *FSM) Run(v interface{}) {
	rv := reflect.New(reflect.TypeOf(v))
	if init := rv.MethodByName("Init"); init != (reflect.Value{}) {
		init.Call([]reflect.Value{reflect.ValueOf(fsm.rng)})
	}

	for {
		if len(fsm.transitions[fsm.state]) == 0 {
			log.Println("dead end")
			return
		}

		idx := fsm.rng.Intn(len(fsm.transitions[fsm.state]))
		trans := fsm.transitions[fsm.state][idx]
		idx = fsm.rng.Intn(len(trans.methods))
		// TODO check that the name is valid
		call := rv.MethodByName(trans.methods[idx] + "Call")
		var args []interface{}
		for i, n := 0, reflect.TypeOf(call.Interface()).NumIn(); i < n; i++ {
			// TODO optimize reflection
			v, ok := quick.Value(reflect.TypeOf(call.Interface()).In(i), fsm.rng)
			if !ok {
				panic("cannot generate value") // XXX
			}
			args = append(args, v.Interface())
		}
		pre := rv.MethodByName(trans.methods[idx] + "Pre")
		if pre != (reflect.Value{}) {
			// TODO guard against code having wrong types/arity/...
			if !pre.Call([]reflect.Value{
				reflect.ValueOf(fsm.state),
				reflect.ValueOf(trans.to),
				reflect.ValueOf(args),
			})[0].Bool() {
				//log.Println("pre-condition failed for", trans.methods[idx])
				continue
			}
		}
		rargs := make([]reflect.Value, 0, len(args))
		for _, v := range args {
			rargs = append(rargs, reflect.ValueOf(v))
		}
		log.Printf("calling %s(%v)", trans.methods[idx]+"Call", args)
		ret := call.Call(rargs)
		iret := make([]interface{}, 0, len(ret))
		for _, v := range ret {
			iret = append(iret, v.Interface())
		}

		post := rv.MethodByName(trans.methods[idx] + "Post")
		if post != (reflect.Value{}) {

			if !post.Call([]reflect.Value{
				reflect.ValueOf(fsm.state),
				reflect.ValueOf(trans.to),
				reflect.ValueOf(args),
				reflect.ValueOf(iret),
			})[0].Bool() {
				log.Printf("postcondition failed for %s = (%v)", trans.methods[idx], iret)
				return
			}
		}

		next := rv.MethodByName(trans.methods[idx] + "Next")
		if next != (reflect.Value{}) {
			next.Call(
				[]reflect.Value{
					reflect.ValueOf(fsm.state),
					reflect.ValueOf(trans.to),
					reflect.ValueOf(args),
					reflect.ValueOf(iret),
				})
		}
	}
}

/*
$ go run quick.go
2016/01/28 11:57:20 calling SizeCall([])
2016/01/28 11:57:20 calling SizeCall([])
2016/01/28 11:57:20 calling SizeCall([])
2016/01/28 11:57:20 calling SizeCall([])
2016/01/28 11:57:20 calling AddCall([-2194441888669636008])
2016/01/28 11:57:20 calling GetCall([])
2016/01/28 11:57:20 calling AddCall([-4414100994337315531])
2016/01/28 11:57:20 calling GetCall([])
2016/01/28 11:57:20 calling SizeCall([])
2016/01/28 11:57:20 calling AddCall([4339827657622334614])
2016/01/28 11:57:20 calling SizeCall([])
2016/01/28 11:57:20 postcondition failed for Size = ([0])
*/

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

func (m *Model) Init(rng *rand.Rand) {
	// Init sets up the model at the start of a test run

	m.size = int(rng.Int31n(1 << 7))
	m.size = 1 // XXX, makes testing the prototype easier
	m.queue = &Queue{
		size:     m.size,
		elements: make([]int, m.size),
	}
}

// Preconditions determine whether a function is allowed to be called
// in the current state (as per the contract of the API)
//
// Calls are responsible for calling the function(s) under test
//
// Postconditions check if the function call's result matches the expected model
//
// Next updates the model

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

func main() {
	seed := time.Now().UnixNano()
	fsm := NewFSM(seed)
	// Our queue has a single state, in which all its methods can be
	// called repeatedly in any order
	fsm.Transition("state1", "state1", []string{"Add", "Get", "Size"})
	fsm.state = "state1" // XXX set the initial state. we'll want an API on FSM2 for that.
	fsm.Run(Model{})
}
