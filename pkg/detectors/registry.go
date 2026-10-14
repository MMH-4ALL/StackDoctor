// Package detectors contains StackDoctor's built-in project detectors.
package detectors

import "github.com/MMH-4ALL/StackDoctor/pkg/diagnostic"

func Builtins() *diagnostic.Registry {
	r := &diagnostic.Registry{}
	for _, d := range []diagnostic.Detector{Node{}, Python{}, Go{}, Rust{}, Env{}, Ports{}} {
		if err := r.Register(d); err != nil {
			panic(err)
		} // Static programming error, never project input.
	}
	return r
}
