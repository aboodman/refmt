package objLegacy

import (
	. "github.com/polydawn/refmt/tok"
)

/*
	Allocates the machinery for treating an object like a `TokenSource`.
	This machinery will walk over structures in memory,
	emitting tokens representing values and fields as it visits them.

	Initialization must be finished by calling `Bind` to set the value to visit;
	after this, the `Step` function is ready to be pumped.
	Subsequent calls to `Bind` do a full reset, leaving `Step` ready to call
	again and making all of the machinery reusable without re-allocating.
*/
func NewMarshaller(s *Suite) *MarshalDriver {
	d := &MarshalDriver{
		marshalSlab: marshalSlab{
			suite: s,
			rows:  make([]marshalSlabRow, 0, 10),
		},
		stack: make([]MarshalMachine, 0, 10),
	}
	return d
}

func (d *MarshalDriver) Bind(v interface{}) {
	d.stack = d.stack[0:0]
	d.marshalSlab.rows = d.marshalSlab.rows[0:0]
	d.step = d.marshalSlab.mustPickMarshalMachine(v)
	d.step.Reset(&d.marshalSlab, v)
}

type MarshalDriver struct {
	marshalSlab marshalSlab
	stack       []MarshalMachine
	step        MarshalMachine
}

type MarshalMachine interface {
	Reset(*marshalSlab, interface{}) error
	Step(*MarshalDriver, *marshalSlab, *Token) (done bool, err error)
}

// for convenience in declaring fields of state machines with internal step funcs
type marshalMachineStep func(*MarshalDriver, *Token) (done bool, err error)

func (d *MarshalDriver) Step(tok *Token) (bool, error) {
	//	fmt.Printf("> next step is %#v\n", d.step)
	done, err := d.step.Step(d, &d.marshalSlab, tok)
	//	fmt.Printf(">> yield is %#v\n", TokenToString(*tok))
	// If the step errored: out, entirely.
	if err != nil {
		return true, err
	}
	// If the step wasn't done, return same status.
	if !done {
		return false, nil
	}
	// If it WAS done, pop next, or if stack empty, we're entirely done.
	nSteps := len(d.stack) - 1
	if nSteps == -1 {
		return true, nil // that's all folks
	}
	//	fmt.Printf(">> popping up from %#v\n", d.stack)
	d.step = d.stack[nSteps]
	d.stack = d.stack[0:nSteps]
	return false, nil
}

/*
	Starts the process of recursing marshalling over `target` value.

	Caller provides the machine to use (this is an optimization for maps and slices,
	which already know the machine and keep reusing it for all their entries).
	This method pushes the first step with `tok` (the upstream tends to have peeked at
	it in order to decide what to do, but if recursing, it belongs to the next obj),
	then saves this new machine onto the driver's stack: future calls to step
	the driver will then continuing stepping the new machine it returns a done status,
	at which point we'll finally "return" by popping back to the last machine on the stack
	(which is presumably the same one that just called this Recurse method).

	In other words, your MarshalMachine calls this when it wants to deal
	with an object, and by the time we call back to your machine again,
	that object will be traversed and the stream ready for you to continue.
*/
func (d *MarshalDriver) Recurse(tok *Token, target interface{}, nextMach MarshalMachine) (err error) {
	//	fmt.Printf(">>> pushing into recursion with %#v\n", nextMach)
	// Push the current machine onto the stack (we'll resume it when the new one is done),
	d.stack = append(d.stack, d.step)
	// Initialize the machine for this new target value.
	err = nextMach.Reset(&d.marshalSlab, target)
	if err != nil {
		return
	}
	d.step = nextMach
	// Immediately make a step (we're still the delegate in charge of someone else's step).
	_, err = d.Step(tok)
	return
}
