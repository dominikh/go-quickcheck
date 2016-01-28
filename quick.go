package main

import (
	"log"
	"math/rand"
	"reflect"
	"runtime"
	"testing/quick"
	"time"
)

// Add:pre -> len(s.elements) < s.size
// Add:post -> true
// Add:transform -> s.elements << x

// Get:pre -> len(s.elements) > 0
// Get:post -> res == s.elements[0]
// Get:transform -> s.elements = s.elements[1:]

// Size:pre -> true
// Size:post -> res == len(s.elements)
// Size:transform -> nil

type Precondition func() bool
type Postcondition func(input interface{}) bool

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
	return abs(q.w-q.r) % q.size
}

// func (q *Queue) Size() int {
// 	return (q.w - q.r + q.size) % q.size
// }

func (*Queue) Free() {}

type State struct {
	name string
	// state -> list of fn
	transitions map[string][]interface{}

	Data interface{}
}

type Function struct {
	Fn   interface{}
	Pre  func(from, to string, data interface{}, args []interface{}) bool
	Args func(from, to string, data interface{}) []interface{}
	Next func(from, to string, data interface{}, args []interface{}, ret []interface{}) interface{}
	Post func(from, to string, data interface{}, args []interface{}, ret []interface{}) bool
}

type state struct {
	size     int
	elements []int
	queue    *Queue
}

type transition struct {
	to    string
	funcs []Function
}

type FSM struct {
	states []string // TODO probably not needed
	// from+to -> funcs
	transitions map[string][]transition
	functions   []Function

	state string
	data  interface{}
}

func (fsm *FSM) State(state string) {
	fsm.states = append(fsm.states, state)
}

func (fsm *FSM) InitialState(state string) {
	fsm.state = state
}

func (fsm *FSM) InitialData(data interface{}) {
	fsm.data = data
}

func (fsm *FSM) Transition(from, to string, funcs []Function) {
	if fsm.transitions == nil {
		fsm.transitions = make(map[string][]transition)
	}
	fsm.transitions[from] = append(fsm.transitions[from], transition{to, funcs})
}

func (fsm *FSM) Function(fn Function) {
	fsm.functions = append(fsm.functions, fn)
}

func funcName(fn interface{}) string {
	if fn == nil {
		return "nil"
	}
	return runtime.FuncForPC(reflect.ValueOf(fn).Pointer()).Name()
}

func (fsm *FSM) Run() {
	seed := time.Now().UnixNano()
	//seed := int64(1453934094266043870)
	//seed := int64(1453934777130601499)
	rng := rand.New(rand.NewSource(seed))
	log.Println("rand seed:", seed)
	n := 0
	for {
		n++
		if n > 10000 {
			log.Println("giving up")
			return
		}
		if len(fsm.transitions[fsm.state]) == 0 {
			log.Println("dead end")
			return
		}
		idx := rng.Intn(len(fsm.transitions[fsm.state]))
		trans := fsm.transitions[fsm.state][idx]
		idx = rng.Intn(len(trans.funcs))
		fn := trans.funcs[idx]
		// FIXME this is buggy. a function pointer might not identify a function uniquely

		// TODO use Args
		var args []reflect.Value
		var argsi []interface{}
		if fn.Fn != nil {
			typ := reflect.TypeOf(fn.Fn)
			num := typ.NumIn()
			for i := 0; i < num; i++ {
				v, ok := quick.Value(typ.In(i), rng)
				if !ok {
					panic("cannot generate value")
				}
				args = append(args, v)
			}
			argsi = make([]interface{}, 0, len(args))
			for _, v := range args {
				argsi = append(argsi, v.Interface())
			}
		}
		if fn.Pre != nil {
			if !fn.Pre(fsm.state, trans.to, fsm.data, argsi) {
				// FIXME
				log.Println("skipping", funcName(fn.Fn))
				continue
			}
		}

		log.Println("calling", funcName(fn.Fn), argsi)
		var ret []reflect.Value
		var reti []interface{}
		if fn.Fn != nil {
			ret = reflect.ValueOf(fn.Fn).Call(args)
			reti = make([]interface{}, 0, len(ret))
		}
		for _, v := range ret {
			reti = append(reti, v.Interface())
		}
		if fn.Post != nil {
			if !fn.Post(fsm.state, trans.to, fsm.data, []interface{}{0}, reti) {
				log.Fatal("post condition failed")
			}
		}
		if fn.Next != nil {
			fsm.data = fn.Next(fsm.state, trans.to, fsm.data, argsi, reti)
		}
		fsm.state = trans.to
	}
}

func main() {
	// TODO this should reuse the same rng as the rest
	rand.Seed(time.Now().Unix())
	var n uint8
	for n == 0 {
		n = uint8(rand.Int31())
	}
	log.Println("n:", n)
	q := &Queue{size: int(n) + 1, elements: make([]int, int(n)+1)}

	fsm := FSM{}
	//fsm.State("new")
	fsm.State("initialised")
	//fsm.State("destroyed")
	//fsm.InitialState("new")
	fsm.InitialState("initialised")
	fsm.InitialData(state{size: int(n)})

	add := Function{
		q.Add,
		func(from, to string, data interface{}, args []interface{}) bool {
			return data.(state).size > len(data.(state).elements)
		},
		nil,
		func(from, to string, data interface{}, args []interface{}, ret []interface{}) interface{} {
			s := data.(state)
			els := make([]int, len(s.elements))
			copy(els, s.elements)
			els = append(els, args[0].(int))
			s.elements = els
			return s
		},
		nil,
	}
	fsm.Function(add)

	get := Function{
		q.Get,
		func(from, to string, data interface{}, args []interface{}) bool {
			return len(data.(state).elements) > 0
		},
		nil,
		func(from, to string, data interface{}, args []interface{}, ret []interface{}) interface{} {
			s := data.(state)
			s.elements = s.elements[1:]
			return s
		},
		func(from, to string, data interface{}, args []interface{}, ret []interface{}) bool {
			v := ret[0].(int) == data.(state).elements[0]
			if !v {
				log.Printf("got element %d, want %d", ret[0].(int), data.(state).elements[0])
			}
			return v
		},
	}
	fsm.Function(get)

	size := Function{
		q.Size,
		nil,
		nil,
		nil,
		func(from, to string, data interface{}, args []interface{}, ret []interface{}) bool {
			// TODO testing-like interface
			v := ret[0].(int) == len(data.(state).elements)
			if !v {
				log.Printf("got size %d, want %d", ret[0].(int), len(data.(state).elements))
			}
			return v
		},
	}
	fsm.Function(size)

	free := Function{
		q.Free,
		nil,
		nil,
		nil,
		nil,
	}
	fsm.Function(free)

	// init := Function{
	// 	nil, nil, nil,
	// 	func(from, to string, data interface{}, args []interface{}, ret []interface{}) interface{} {
	// 		v := data.(state)
	// 		// TODO this should reuse the same rng as the rest
	// 		n := uint8(rand.Int31())
	// 		v.queue = &Queue{size: int(n), elements: make([]int, int(n))}
	// 		v.size = int(n)
	// 		return v
	// 	},
	// 	nil,
	// }

	//fsm.Transition("new", "initialised", []Function{init})
	fsm.Transition("initialised", "initialised", []Function{add, get, size})
	//fsm.Transition("initialised", "destroyed", []Function{free})

	fsm.Run()
}
