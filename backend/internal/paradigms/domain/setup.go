package domain

import (
	"errors"
	"fmt"
	"maps"
	"slices"
)

var ErrMissingParameter = errors.New("parameter is required")

// ResolveSetup validates the configuration of a physical apparatus and a
// session for one paradigm version. The environment holds apparatus values,
// which are measurements and must all be given, and optional analysis values;
// the session holds session values. Missing analysis and session values fall
// back to their defaults. Ranges and the version's rules across parameters
// apply to the combined values. It returns the session values with defaults
// filled in.
func ResolveSetup(key string, version int, environment, session map[string]float64) (map[string]float64, error) {
	d, err := lookup(key, version)
	if err != nil {
		return nil, err
	}
	p := d.paradigm
	scope := func(k string) Scope {
		i := slices.IndexFunc(p.Parameters, func(param Parameter) bool { return param.Key == k })
		if i < 0 {
			return ""
		}
		return p.Parameters[i].Scope
	}
	for k := range environment {
		if s := scope(k); s != ApparatusScope && s != AnalysisScope {
			return nil, fmt.Errorf("%w: %s is not an apparatus or analysis parameter", ErrUnknownParameter, k)
		}
	}
	for k := range session {
		if scope(k) != SessionScope {
			return nil, fmt.Errorf("%w: %s is not a session parameter", ErrUnknownParameter, k)
		}
	}
	for _, param := range p.In(ApparatusScope) {
		if _, ok := environment[param.Key]; !ok {
			return nil, fmt.Errorf("%w: %s", ErrMissingParameter, param.Key)
		}
	}
	values := make(map[string]float64, len(environment)+len(session))
	maps.Copy(values, environment)
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
	sessionParams := p.In(SessionScope)
	out := make(map[string]float64, len(sessionParams))
	for _, param := range sessionParams {
		out[param.Key] = resolved[param.Key]
	}
	return out, nil
}
