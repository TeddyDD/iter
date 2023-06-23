package iter

import (
	"errors"
)

var ErrStop = errors.New("iterator stopped")

// Cursor can be used to iterate API or database.  It drives iteration with
// functions provided via [Config].
type Cursor[Request, Response any] struct {
	response        Response
	request         Request
	next            bool
	hasNext         func(response Response) (Request, bool)
	fetchNext       func(request Request) (Response, error)
	getFirstRequest func() Request
}

type Config[Request, Response any] struct {
	// HasNext checks if response indicates there is more Responses
	// to fetch.
	HasNext func(response Response) (Request, bool)
	// FetchNext should fetch next Response.
	FetchNext func(request Request) (Response, error)
	// GetFirstRequest must return initial request that can be used by
	// the cursor.
	GetFirstRequest func() Request
}

// New creates a new instance of CursorIterator with the provided functions.
func New[Request, Response any](
	config Config[Request, Response],
) *Cursor[Request, Response] {
	return &Cursor[Request, Response]{
		next:            true,
		hasNext:         config.HasNext,
		fetchNext:       config.FetchNext,
		getFirstRequest: config.GetFirstRequest,
	}
}

// Next returns true if there are more elements to iterate, false otherwise.
func (d *Cursor[Request, Response]) Next() bool {
	return d.next
}

// Get returns the current element of the iterator and advances to the next element.
// An error is returned if called when there are no more elements.
func (d *Cursor[Request, Response]) Get() (Response, error) {
	if !d.next {
		return d.response, ErrStop
	}

	var err error

	d.response, err = d.fetchNext(d.request)
	if err != nil {
		return d.response, err
	}

	d.request, d.next = d.hasNext(d.response)
	return d.response, nil
}

// Iterate iterates over the elements using the provided callback function.
// It stops iterating if the callback function returns the ErrStop sentinel error.
// Any other error returned by the callback function will be propagated.
func (d *Cursor[Request, Response]) Iterate(callback func(response Response) error) error {
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
func (d *Cursor[Request, Response]) Reset() {
	d.request = d.getFirstRequest()
	d.next = true
}
