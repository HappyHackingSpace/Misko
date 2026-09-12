package domain

import (
	"errors"
	"fmt"
	"maps"
	"slices"
)

var ErrMissingParameter = errors.New("parameter is required")

// ResolveSetup validates the configuration of a physical apparatus and a
// session for one paradigm version. Apparatus parameters are measurements and
// must all be given; session parameters fall back to their defaults. Ranges
// and the version's rules across parameters apply to the combined values. It
// returns the session parameters with defaults filled in.
func ResolveSetup(key string, version int, apparatus, session map[string]float64) (map[string]float64, error) {
	d, err := lookup(key, version)
	if err != nil {
		return nil, err
	}
	p := d.paradigm
	isApparatus := func(k string) bool {
		return slices.ContainsFunc(p.ApparatusParameters, func(param Parameter) bool { return param.Key == k })
	}
	for k := range apparatus {
		if !isApparatus(k) {
			return nil, fmt.Errorf("%w: %s is not an apparatus parameter", ErrUnknownParameter, k)
		}
	}
	for k := range session {
		if isApparatus(k) {
			return nil, fmt.Errorf("%w: %s is not a session parameter", ErrUnknownParameter, k)
		}
	}
	for _, param := range p.ApparatusParameters {
		if _, ok := apparatus[param.Key]; !ok {
			return nil, fmt.Errorf("%w: %s", ErrMissingParameter, param.Key)
		}
	}
	values := make(map[string]float64, len(apparatus)+len(session))
	maps.Copy(values, apparatus)
	maps.Copy(values, session)
	resolved, err := resolveParameters(p, values)
	if err != nil {
		return nil, err
	}
	if d.validate != nil {
		if err := d.validate(resolved); err != nil {
			return nil, err
		}
	}
	out := make(map[string]float64, len(p.SessionParameters))
	for _, param := range p.SessionParameters {
		out[param.Key] = resolved[param.Key]
	}
	return out, nil
}
