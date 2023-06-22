package iter

import (
	"errors"
)

var ErrStop = errors.New("iterator stopped")

type CursorIterator[Request, Response any] struct {
	response Response
	request  Request
	next     bool
	hasNext  func(response Response) (Request, bool)
	getNext  func(request Request) (Response, error)
	firstFn  func() Request
}

type Config[Request, Response any] struct {
	HasNext  func(response Response) (Request, bool)
	GetNext  func(request Request) (Response, error)
	GetFirst func() Request
}

// New creates a new instance of CursorIterator with the provided functions.
func New[Request, Response any](
	config Config[Request, Response],
) *CursorIterator[Request, Response] {
	return &CursorIterator[Request, Response]{
		next:    true,
		hasNext: config.HasNext,
		getNext: config.GetNext,
		firstFn: config.GetFirst,
	}
}

// Next returns true if there are more elements to iterate, false otherwise.
func (d *CursorIterator[Request, Response]) Next() bool {
	return d.next
}

// Get returns the current element of the iterator and advances to the next element.
// An error is returned if called when there are no more elements.
func (d *CursorIterator[Request, Response]) Get() (Response, error) {
	if !d.next {
		return d.response, ErrStop
	}

	var err error

	d.response, err = d.getNext(d.request)
	if err != nil {
		return d.response, err
	}

	d.request, d.next = d.hasNext(d.response)
	return d.response, nil
}

// Iterate iterates over the elements using the provided callback function.
// It stops iterating if the callback function returns the ErrStop sentinel error.
// Any other error returned by the callback function will be propagated.
func (d *CursorIterator[Request, Response]) Iterate(callback func(response Response) error) error {
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
func (d *CursorIterator[Request, Response]) Reset() {
	d.request = d.firstFn()
	d.next = true
}
