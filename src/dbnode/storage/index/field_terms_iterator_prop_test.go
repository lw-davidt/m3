// +build big

// Copyright (c) 2019 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package index

import (
	"math/rand"
	"os"
	"testing"
	"time"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

func TestFieldsTermsIteratorPropertyTest(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	seed := time.Now().UnixNano()
	parameters.MinSuccessfulTests = 100
	parameters.MaxSize = 40
	parameters.Rng = rand.New(rand.NewSource(seed))
	properties := gopter.NewProperties(parameters)

	properties.Property("Fields Terms Iteration doesn't blow up", prop.ForAll(
		func(i fieldsTermsIteratorPropInput) (bool, error) {
			expected := i.expected()
			seg := i.setup.asSegment(t)
			iter, err := newFieldsAndTermsIterator(seg, fieldsAndTermsIteratorOpts{
				iterateTerms: i.iterateTerms,
				allowFn:      i.allowFn,
			})
			if err != nil {
				return false, err
			}
			observed := toSlice(t, iter)
			requireSlicesEqual(t, expected, observed)
			return true, nil
		},
		genFieldsTermsIteratorPropInput(),
	))

	reporter := gopter.NewFormatedReporter(true, 160, os.Stdout)
	if !properties.Run(reporter) {
		t.Errorf("failed with initial seed: %d", seed)
	}
}

type fieldsTermsIteratorPropInput struct {
	setup        fieldsTermsIterSetup
	iterateTerms bool
	allowFn      allowFn
}

func (i fieldsTermsIteratorPropInput) expected() []pair {
	fields := i.setup.fields
	expected := make([]pair, 0, len(fields))
	seen := make(map[string]bool, len(fields))
	for _, f := range fields {
		if !i.allowFn([]byte(f.Name)) {
			continue
		}
		if seen[f.Name] {
			continue
		}
		seen[f.Name] = true
		if !i.iterateTerms {
			f.Value = ""
		}
		expected = append(expected, f)
	}
	return expected
}

func genFieldsTermsIteratorPropInput() gopter.Gen {
	return genFieldsTermsIteratorSetup().
		Map(func(s fieldsTermsIterSetup, params *gopter.GenParameters) fieldsTermsIteratorPropInput {
			allowedFields := make(map[string]bool, len(s.fields))
			for _, f := range s.fields {
				if params.NextBool() {
					allowedFields[f.Name] = true
				}
			}
			return fieldsTermsIteratorPropInput{
				setup:        s,
				iterateTerms: params.NextBool(),
				allowFn: func(f []byte) bool {
					return allowedFields[string(f)]
				},
			}
		})
}

func genFieldsTermsIteratorSetup() gopter.Gen {
	return gen.SliceOf(
		gen.Identifier()).
		SuchThat(func(items []string) bool {
			return len(items)%2 == 0 && len(items) > 0
		}).
		Map(func(items []string) fieldsTermsIterSetup {
			pairs := make([]pair, 0, len(items)/2)
			for i := 0; i < len(items); i += 2 {
				name, value := items[i], items[i+1]
				pairs = append(pairs, pair{name, value})
			}
			return newFieldsTermsIterSetup(pairs...)
		})
}
