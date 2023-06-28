# iter

iter is a Go package that provides a cursor iterator for iterating over
paginated results from APIs or databases. It allows you to drive the iteration
manually, giving you control over fetching the next result and advancing to
the next element. Features

- Manually drive the iteration using the Next and Get methods.
- Iterate over the elements using the Iterate method, which accepts a callback function.
- Stop the iteration by returning the ErrStop sentinel error from the callback function.
- Reset the iterator to its initial state using the Reset method.

## Installation

Use the go get command to install the iter package:

```
go get go.teddydd.me/iter
```

## Usage

Import the iter package into your Go program:

```go
import "go.teddydd.me/iter"
```

Define the necessary functions for your cursor:

```go

// Define the input and result types for the cursor.
type MyInput struct {
	// Define your input structure here.
}

type MyResult struct {
	// Define your result structure here.
}

// Define the configuration for the cursor.
config := iter.Config[MyInput, MyResult]{
	HasNext: func(result MyResult) (MyInput, bool) {
		// Implement the logic to check if there are more results to fetch.
		// Return the next input and a boolean indicating whether there are more results.
	},

	FetchNext: func(input MyInput) (MyResult, error) {
		// Implement the logic to fetch the next result based on the given input.
		// Return the fetched result or an error, if any.
	},

	GetFirstInput: func() MyInput {
		// Implement the logic to get the initial input for the cursor.
		// Return the initial input.
	},
}

// Create a new cursor using the configuration.
cursor := iter.New[MyInput, MyResult](config)
```

Use the cursor to iterate over the results:

```go

// Manually iterate using Next and Get.
for cursor.Next() {
	result, err := cursor.Get()
	if err != nil {
		// Handle the error or stop the iteration.
		break
	}

	// Process the result.
	// ...

	// Example: Stop the iteration if a specific condition is met.
	if shouldStop(result) {
		break
	}
}

// Iterate using the Iterate method and a callback function.
err := cursor.Iterate(func(result MyResult) error {
	// Process the result.
	// ...

	// Make sure to handle empty MyResult if API might return one.

	// Example: Stop the iteration if a specific condition is met.
	if shouldStop(result) {
		return iter.ErrStop
	}

	return nil
})

if err != nil {
	// Handle the error.
	// ErrStop is is not returned from Iterate
}
```

Reset the cursor to its initial state, if needed:

```go
cursor.Reset()
```

## Examples

For more usage examples, please refer to the iterator tests in the
repository. It contains simple iterator for HTTP API.

## Contributing

Contributions to the iter package are welcome! If you find any issues or have
suggestions for improvements, please feel free to write to my public inbox:
https://lists.sr.ht/~teddy/public-inbox You can also send patches.

## Source code

Source code is avaliable [here](https://git.sr.ht/~teddy/iter/). Read-only
[mirror](https://github.com/TeddyDD/iter) is provided for convenience.

## License

The iter package is open-source and released under the BSD Zero Clause License.
