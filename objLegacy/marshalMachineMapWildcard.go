package objLegacy

import (
	"fmt"
	"reflect"
	"sort"

	. "github.com/polydawn/refmt/tok"
)

type MarshalMachineMapWildcard struct {
	target_rv reflect.Value
	valueMach MarshalMachine
	keys      []wildcardMapStringyKey
	index     int
	value     bool
}

func (m *MarshalMachineMapWildcard) Reset(s *marshalSlab, valp interface{}) error {
	m.target_rv = reflect.ValueOf(valp).Elem()

	// Pick machinery for handling the value types.
	m.valueMach = s.mustPickMarshalMachineByType(m.target_rv.Type().Elem())

	// Enumerate all the keys (must do this up front, one way or another),
	// flip them into strings,
	// and sort them (optional, arguably, but right now you're getting it).
	key_rt := m.target_rv.Type().Key()
	switch key_rt.Kind() {
	case reflect.String:
		// continue.
		// note: stdlib json.marshal supports all the int types here as well, and will
		//  tostring them.  but this is not supported symmetrically; so we simply... don't.
	default:
		return fmt.Errorf("unsupported map key type %q", key_rt.Name())
	}
	keys_rv := m.target_rv.MapKeys()
	m.keys = make([]wildcardMapStringyKey, len(keys_rv))
	for i, v := range keys_rv {
		m.keys[i].rv = v
		m.keys[i].s = v.String()
	}
	sort.Sort(wildcardMapStringyKey_byString(m.keys))

	m.index = -1
	return nil
}

func (m *MarshalMachineMapWildcard) Step(d *MarshalDriver, s *marshalSlab, tok *Token) (done bool, err error) {
	if m.index < 0 {
		if m.target_rv.IsNil() {
			tok.Type = TNull
			m.index++
			return true, nil
		}
		tok.Type = TMapOpen
		tok.Length = m.target_rv.Len()
		m.index++
		return false, nil
	}
	if m.index == len(m.keys) {
		tok.Type = TMapClose
		m.index++
		s.release()
		return true, nil
	}
	if m.index > len(m.keys) {
		return true, fmt.Errorf("invalid state: value already consumed")
	}
	if m.value {
		val_rv := m.target_rv.MapIndex(m.keys[m.index].rv)
		new_vprv := reflect.New(val_rv.Type())
		new_vprv.Elem().Set(val_rv)
		valp := new_vprv.Interface()
		m.value = false
		m.index++
		return false, d.Recurse(tok, valp, m.valueMach)
	}
	tok.Type = TString
	tok.Str = m.keys[m.index].s
	m.value = true
	return false, nil
}

// Holder for the reflect.Value and string form of a key.
// We need the reflect.Value for looking up the map value;
// and we need the string for sorting.
type wildcardMapStringyKey struct {
	rv reflect.Value
	s  string
}

type wildcardMapStringyKey_byString []wildcardMapStringyKey

func (x wildcardMapStringyKey_byString) Len() int           { return len(x) }
func (x wildcardMapStringyKey_byString) Swap(i, j int)      { x[i], x[j] = x[j], x[i] }
func (x wildcardMapStringyKey_byString) Less(i, j int) bool { return x[i].s < x[j].s }
