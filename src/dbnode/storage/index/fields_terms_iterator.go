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
	"github.com/m3db/m3/src/m3ninx/index/segment"
	xerrors "github.com/m3db/m3/src/x/errors"
)

// fieldsAndTermsIterator iterates over all known fields and terms for a segment.
type fieldsAndTermsIterator interface {
	// Next returns a bool indicating if there are any more elements.
	Next() bool

	// Current returns the current element.
	// NB: the element returned is only valid until the subsequent call to Next().
	Current() (field, term []byte)

	// Err returns any errors encountered during iteration.
	Err() error

	// Close releases any resources held by the iterator.
	Close() error

	// Reset resets the iterator to the start iterating the given segment.
	Reset(seg segment.Segment, opts fieldsAndTermsIteratorOpts) error
}

type fieldsAndTermsIteratorOpts struct {
	iterateTerms bool
	allowFn      allowFn
}

func (o fieldsAndTermsIteratorOpts) allow(f []byte) bool {
	if o.allowFn == nil {
		return true
	}
	return o.allowFn(f)
}

type allowFn func(field []byte) bool

type fieldsAndTermsIter struct {
	seg  segment.Segment
	opts fieldsAndTermsIteratorOpts

	done      bool
	err       error
	fieldIter segment.FieldsIterator
	termIter  segment.TermsIterator

	current struct {
		field []byte
		term  []byte
	}
}

var (
	fieldsAndTermsIterZeroed fieldsAndTermsIter
)

var _ fieldsAndTermsIterator = &fieldsAndTermsIter{}

func newFieldsAndTermsIterator(s segment.Segment, opts fieldsAndTermsIteratorOpts) (fieldsAndTermsIterator, error) {
	iter := &fieldsAndTermsIter{}
	err := iter.Reset(s, opts)
	if err != nil {
		return nil, err
	}
	return iter, nil
}

func (fti *fieldsAndTermsIter) Reset(s segment.Segment, opts fieldsAndTermsIteratorOpts) error {
	*fti = fieldsAndTermsIterZeroed
	fti.seg = s
	fti.opts = opts
	if s == nil {
		return nil
	}
	fiter, err := s.FieldsIterable().Fields()
	if err != nil {
		return err
	}
	fti.fieldIter = fiter
	return nil
}

func (fti *fieldsAndTermsIter) setNextField() bool {
	if fti.fieldIter == nil {
		return false
	}

	for fti.fieldIter.Next() {
		field := fti.fieldIter.Current()
		if !fti.opts.allow(field) {
			continue
		}
		fti.current.field = fti.fieldIter.Current()
		return true
	}

	fti.err = fti.fieldIter.Err()
	return false
}

func (fti *fieldsAndTermsIter) setNext() bool {
	// if only need to iterate fields
	if !fti.opts.iterateTerms {
		return fti.setNextField()
	}

	// if iterating both terms and fields, check if current field has another term
	if fti.termIter != nil {
		if fti.termIter.Next() {
			fti.current.term, _ = fti.termIter.Current()
			return true
		}
		if err := fti.termIter.Err(); err != nil {
			fti.err = err
			return false
		}
		if err := fti.termIter.Close(); err != nil {
			fti.err = err
			return false
		}
	}

	// i.e. need to switch to next field
	hasNext := fti.setNextField()
	if !hasNext {
		return false
	}

	// and get next term for the field
	termsIter, err := fti.seg.TermsIterable().Terms(fti.current.field)
	if err != nil {
		fti.err = err
		return false
	}
	fti.termIter = termsIter

	hasNext = fti.termIter.Next()
	if !hasNext {
		fti.err = fti.fieldIter.Err()
		return false
	}

	fti.current.term, _ = fti.termIter.Current()
	return true
}

func (fti *fieldsAndTermsIter) Next() bool {
	if fti.err != nil || fti.done {
		return false
	}
	return fti.setNext()
}

func (fti *fieldsAndTermsIter) Current() (field, term []byte) {
	return fti.current.field, fti.current.term
}

func (fti *fieldsAndTermsIter) Err() error {
	return fti.err
}

func (fti *fieldsAndTermsIter) Close() error {
	var multiErr xerrors.MultiError
	if fti.fieldIter != nil {
		multiErr = multiErr.Add(fti.fieldIter.Close())
	}
	if fti.termIter != nil {
		multiErr = multiErr.Add(fti.termIter.Close())
	}
	multiErr = multiErr.Add(fti.Reset(nil, fieldsAndTermsIteratorOpts{}))
	return multiErr.FinalError()
}
