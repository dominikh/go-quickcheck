package quickcheck

import (
	"log"
	"math/rand"
	"reflect"
	"testing/quick"
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

func NewFSM(seed int64) *FSM {
	return &FSM{
		seed:        seed,
		state:       "state0", // FIXME do not hardcode name of initial state
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
	// FIXME Right now, Run will only return once it has encountered a
	// failure
	//
	// FIXME Run only makes one (infinitely long) attempt at
	// triggering a failure.
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

func minimizeFunc(fsm *FSM, model interface{}) func(d []Step) result {
	return func(d []Step) result {
		valid, fail := fsm.Replay(d, model)
		if !valid {
			return ddUnresolved
		}
		if fail {
			return ddFail
		}
		return ddPass
	}
}

func (fsm *FSM) Minimize(steps []Step, model interface{}) []Step {
	return minimize(steps, minimizeFunc(fsm, model))
}
