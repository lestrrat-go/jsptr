package jsptr_test

import (
	"fmt"

	"github.com/lestrrat-go/blackmagic"
	"github.com/lestrrat-go/jsptr"
)

type Root struct {
	Foo Foo `json:"foo"`
}

type Foo struct {
	Bar Bar `json:"bar"`
}

type Bar struct {
	Baz string `json:"baz"`
}

type Custom struct{}

func (c *Custom) RetrieveJSONPointer(dst any, ptr string) error {
	if ptr == "/foo/bar/baz" {
		return blackmagic.AssignIfCompatible(dst, "hello world")
	}
	return fmt.Errorf("not found")
}

func Example() {
	const message = "hello world"

	// Retrieve from a map: Useful if you unmarshal a JSON into a map[string]any
	m := map[string]any{
		"foo": map[string]any{
			"bar": map[string]any{
				"baz": message,
			},
		},
	}

	// Retrieve from a struct: Useful if you unmarshal a JSON into a struct
	s := &Root{
		Foo: Foo{
			Bar: Bar{
				Baz: message,
			},
		},
	}

	// You could even use a custom target that implements the RetrieveJSONPointer method
	custom := &Custom{}

	// Or slices
	slice := []string{"foo", "bar", "baz", message}

	testcases := []struct {
		Ptr    string
		Target any
	}{
		{
			Target: m,
			Ptr:    "/foo/bar/baz",
		},
		{
			Target: s,
			Ptr:    "/foo/bar/baz",
		},
		{
			Target: custom,
			Ptr:    "/foo/bar/baz",
		},
		{
			Target: slice,
			Ptr:    "/3",
		},
	}
	for _, tc := range testcases {
		// Obviously, in real likfe you could (and should) reuse the same pointer if you are
		// going to be evaluating the same pointer multiple times.
		p, err := jsptr.New(tc.Ptr)
		if err != nil {
			fmt.Printf("Error creating pointer: %v\n", err)
			return
		}

		var dst string
		if err := p.Retrieve(&dst, tc.Target); err != nil {
			fmt.Printf("Error retrieving value: %v\n", err)
			return
		}
		if dst != message {
			fmt.Printf("Expected 'hello world', got '%s'\n", dst)
			return
		}
	}

	// OUTPUT:
}
