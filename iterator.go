package iter

import (
	"errors"
)

var ErrStop = errors.New("iterator stopped")

// Cursor can be used to iterate API or database.  It drives iteration with
// functions provided via [Config].
type Cursor[Input, Result any] struct {
	result        Result
	input         Input
	next          bool
	hasNext       func(response Result) (Input, bool)
	fetchNext     func(request Input) (Result, error)
	getFirstInput func() Input
}

type Config[Input, Result any] struct {
	// HasNext checks if response indicates there is more Results
	// to fetch.
	HasNext func(response Result) (Input, bool)
	// FetchNext should fetch next Result.
	FetchNext func(request Input) (Result, error)
	// GetFirstInput must return initial input that can be used by
	// the cursor.
	GetFirstInput func() Input
}

// New creates a new instance of CursorIterator with the provided functions.
func New[Input, Result any](
	config Config[Input, Result],
) *Cursor[Input, Result] {
	return &Cursor[Input, Result]{
		next:          true,
		hasNext:       config.HasNext,
		fetchNext:     config.FetchNext,
		getFirstInput: config.GetFirstInput,
	}
}

// Next returns true if there are more elements to iterate, false otherwise.
func (d *Cursor[Input, Result]) Next() bool {
	return d.next
}

// Get returns the current element of the iterator and advances to the next element.
// An error is returned if called when there are no more elements.
func (d *Cursor[Input, Result]) Get() (Result, error) {
	if !d.next {
		return d.result, ErrStop
	}

	var err error

	d.result, err = d.fetchNext(d.input)
	if err != nil {
		return d.result, err
	}

	d.input, d.next = d.hasNext(d.result)
	return d.result, nil
}

// Iterate iterates over the elements using the provided callback function.
// It stops iterating if the callback function returns the ErrStop sentinel error.
// Any other error returned by the callback function will be propagated.
func (d *Cursor[Input, Result]) Iterate(callback func(response Result) error) error {
	for d.Next() {
		response, err := d.Get()
		if err != nil {
			if errors.Is(err, ErrStop) {
				return nil
			}
			return err
		}

		if err := callback(response); err != nil {
			if errors.Is(err, ErrStop) {
				return nil
			}
			return err
		}
	}

	return nil
}

// Reset reinitializes the iterator by resetting the request using firstFn.
func (d *Cursor[Input, Result]) Reset() {
	d.input = d.getFirstInput()
	d.next = true
}
