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

type Result struct {
	Step     Step
	Invalid  bool
	PreFail  bool
	PostFail bool
	Ret      []interface{}
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
	// defer func() {
	// 	panicked = recover()
	// }()
	ret := m.Call(args)
	return ret, nil
}

//func (fsm *FSM) step(s Step, rv reflect.Value) (valid bool, prefail bool, callfail bool) {
func (fsm *FSM) step(s Step, rv reflect.Value) Result {
	// TODO don't verify step if we just generated it
	if s.State != fsm.state {
		return Result{Step: s, Invalid: true}
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
		return Result{Step: s, Invalid: true}
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
			return Result{Step: s, PreFail: true}
		}
	}
	rargs := make([]reflect.Value, 0, len(s.Args))
	for _, v := range s.Args {
		rargs = append(rargs, reflect.ValueOf(v))
	}
	//log.Printf("calling %s(%v)", s.Method+"Call", s.Args)
	ret, panicked := funcall(call, rargs)
	iret := make([]interface{}, 0, len(ret))
	for _, v := range ret {
		iret = append(iret, v.Interface())
	}
	if panicked != nil {
		log.Printf("postcondition failed for %s = (%v): got panic %v", s.Method, iret, panicked)
		return Result{Step: s, PostFail: true, Ret: iret}
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
			return Result{Step: s, PostFail: true, Ret: iret}
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
	return Result{Step: s, Ret: iret}
}

func (fsm *FSM) Replay(ss []Step, model interface{}) (results []Result, valid, fail bool) {
	rv := reflect.New(reflect.TypeOf(model))
	fsm.init(rv)
	for _, s := range ss {
		res := fsm.step(s, rv)
		results = append(results, res)
		if res.Invalid {
			return results, false, false
		}
		if res.PreFail {
			return results, false, false
		}
		if res.PostFail {
			return results, true, true
		}
	}
	return results, true, false
}

func (fsm *FSM) init(rv reflect.Value) {
	fsm.state = "state0"                         // XXX
	fsm.rng = rand.New(rand.NewSource(fsm.seed)) // XXX we shouldn't need the rng in Replay
}

func (fsm *FSM) Run(v interface{}) []Result {
	// FIXME Right now, Run will only return once it has encountered a
	// failure
	//
	// FIXME Run only makes one (infinitely long) attempt at
	// triggering a failure.
	var out []Result

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

		res := fsm.step(step, rv)
		if res.PreFail {
			continue
		}
		out = append(out, res)
		if res.PostFail {
			return out
		}
	}
}

func minimizeFunc(fsm *FSM, model interface{}) func(d []Step) ([]Result, result) {
	return func(d []Step) ([]Result, result) {
		res, valid, fail := fsm.Replay(d, model)
		if !valid {
			return res, ddUnresolved
		}
		if fail {
			log.Println("fail")
			return res, ddFail
		}
		return res, ddPass
	}
}

func (fsm *FSM) Minimize(res []Result, model interface{}) []Result {
	steps := make([]Step, len(res))
	for i, v := range res {
		steps[i] = v.Step
	}
	return minimize(steps, minimizeFunc(fsm, model))
}
