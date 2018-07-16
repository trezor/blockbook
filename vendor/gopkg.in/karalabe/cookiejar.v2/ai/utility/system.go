// CookieJar - A contestant's algorithm toolbox
// Copyright (c) 2014 Peter Szilagyi. All rights reserved.
//
// CookieJar is dual licensed: use of this source code is governed by a BSD
// license that can be found in the LICENSE file. Alternatively, the CookieJar
// toolbox may be used in accordance with the terms and conditions contained
// in a signed written agreement between you and the author(s).

// Package utility implements a reasoner AI based on utility theory.
package utility

import "fmt"

// Utility theory based AI system configuration.
type Config struct {
	Input []InputConf
	Combo []ComboConf
}

// Configuration for input based utility curve(s).
type InputConf struct {
	Id      int     // A referable identifier for the utility
	Min     float64 // Interval start for normalization
	Max     float64 // Interval end for normalization
	Set     bool    // Flag whether the config defines a set of utilities
	NonZero bool    // Flag whether the curve is allowed absolute zero output
	Curve   Curve   // Function mapping the data to a curve
}

// Configuration for combination based utility curve(s).
type ComboConf struct {
	Id   int        // A referable identifier for the utility
	SrcA int        // First input source of the combinator
	SrcB int        // Second input source of the combinator
	Set  bool       // Flag whether the config defines a set of utilities
	Comb Combinator // Function combining the input sources
}

// Utility theory based decision making system.
type System struct {
	utils map[int]utility
}

// Creates a utility theory AI system.
func New(config *Config) *System {
	sys := &System{
		utils: make(map[int]utility),
	}
	for _, input := range config.Input {
		sys.addInput(&input)
	}
	for _, combo := range config.Combo {
		sys.addCombo(&combo)
	}
	return sys
}

// Injects a new input based utility curve (set) into the system.
func (s *System) addInput(config *InputConf) {
	if config.Set {
		// A set of utilities is needed
		utils := newInputSetUtility(config.Curve, config.NonZero)
		utils.Limit(config.Min, config.Max)
		s.utils[config.Id] = utils
	} else {
		// Singleton input utility, insert as is
		util := newInputUtility(config.Curve, config.NonZero)
		util.Limit(config.Min, config.Max)
		s.utils[config.Id] = util
	}
}

// Injects a new combinatorial utility curve set into the system.
func (s *System) addCombo(config *ComboConf) {
	if config.Set {
		// A set of utilities is needed
		srcA := s.utils[config.SrcA]
		srcB := s.utils[config.SrcB]
		s.utils[config.Id] = newComboSetUtility(config.Comb, srcA, srcB)
	} else {
		// Singleton combo utility, insert as is
		srcA := s.utils[config.SrcA]
		srcB := s.utils[config.SrcB]

		util := newComboUtility(config.Comb)
		util.Init(srcA, srcB)

		s.utils[config.Id] = util
	}
}

// Sets the normalization limits for data a utility.
func (s *System) Limit(id int, min, max float64) {
	switch util := s.utils[id].(type) {
	case *inputUtility:
		util.Limit(min, max)
	case *inputSetUtility:
		util.Limit(min, max)
	default:
		panic(fmt.Sprintf("Unknown utility type: %+v", util))
	}
}

// Updates the input of a data utility.
func (s *System) Update(id int, input float64) {
	s.utils[id].(*inputUtility).Update(input)
}

// Updates the input of a member of a data utility set.
func (s *System) UpdateOne(id, index int, input float64) {
	s.utils[id].(*inputSetUtility).Update(index, input)
}

// Updates the input of all the members of a data utility set.
func (s *System) UpdateAll(id int, inputs []float64) {
	util := s.utils[id].(*inputSetUtility)
	for i, input := range inputs {
		util.Update(i, input)
	}
}

// Evaluates a singleton utility.
func (s *System) Evaluate(id int) float64 {
	return s.utils[id].(singleUtility).Evaluate()
}

// Evaluates a member of a utility set.
func (s *System) EvaluateOne(id, index int) float64 {
	return s.utils[id].(multiUtility).Evaluate(index)
}
