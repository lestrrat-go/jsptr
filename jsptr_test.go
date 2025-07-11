package jsptr_test

import (
	"fmt"
	"testing"

	"github.com/lestrrat-go/blackmagic"
	"github.com/lestrrat-go/jsptr"
	"github.com/stretchr/testify/require"
)

func TestPointerNew(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		pattern string
	}{
		{
			name:    "empty pointer",
			input:   "",
			wantErr: false,
			pattern: "",
		},
		{
			name:    "root pointer",
			input:   "/",
			wantErr: false,
			pattern: "/",
		},
		{
			name:    "simple path",
			input:   "/foo",
			wantErr: false,
			pattern: "/foo",
		},
		{
			name:    "nested path",
			input:   "/foo/bar",
			wantErr: false,
			pattern: "/foo/bar",
		},
		{
			name:    "invalid - no leading slash",
			input:   "foo",
			wantErr: true,
		},
		{
			name:    "escaped characters",
			input:   "/foo~1bar/baz~0qux",
			wantErr: false,
			pattern: "/foo~1bar/baz~0qux",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ptr, err := jsptr.New(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.pattern, ptr.Pattern())
		})
	}
}

func TestPointerRetrieveFromJSON(t *testing.T) {
	jsonData := `{
		"foo": "bar",
		"array": [1, 2, 3],
		"nested": {
			"key": "value",
			"num": 42
		}
	}`

	tests := []struct {
		name     string
		pointer  string
		expected any
		wantErr  bool
	}{
		{
			name:     "root",
			pointer:  "",
			expected: map[string]any{"foo": "bar", "array": []any{1.0, 2.0, 3.0}, "nested": map[string]any{"key": "value", "num": 42.0}},
			wantErr:  false,
		},
		{
			name:     "simple property",
			pointer:  "/foo",
			expected: "bar",
			wantErr:  false,
		},
		{
			name:     "array element",
			pointer:  "/array/0",
			expected: 1.0,
			wantErr:  false,
		},
		{
			name:     "nested property",
			pointer:  "/nested/key",
			expected: "value",
			wantErr:  false,
		},
		{
			name:     "nested number",
			pointer:  "/nested/num",
			expected: 42.0,
			wantErr:  false,
		},
		{
			name:    "non-existent property",
			pointer: "/nonexistent",
			wantErr: true,
		},
		{
			name:    "invalid array index",
			pointer: "/array/foo",
			wantErr: true,
		},
		{
			name:    "array index out of bounds",
			pointer: "/array/10",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ptr, err := jsptr.New(tt.pointer)
			require.NoError(t, err)

			var result any
			err = ptr.Retrieve(&result, []byte(jsonData))
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestPointerRetrieveFromMap(t *testing.T) {
	data := map[string]any{
		"foo": "bar",
		"array": []any{1, 2, 3},
		"nested": map[string]any{
			"key": "value",
			"num": 42,
		},
	}

	tests := []struct {
		name     string
		pointer  string
		expected any
		wantErr  bool
	}{
		{
			name:     "root",
			pointer:  "",
			expected: data,
			wantErr:  false,
		},
		{
			name:     "simple property",
			pointer:  "/foo",
			expected: "bar",
			wantErr:  false,
		},
		{
			name:     "array element",
			pointer:  "/array/1",
			expected: 2,
			wantErr:  false,
		},
		{
			name:     "nested property",
			pointer:  "/nested/key",
			expected: "value",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ptr, err := jsptr.New(tt.pointer)
			require.NoError(t, err)

			var result any
			err = ptr.Retrieve(&result, data)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestPointerRetrieveFromStruct(t *testing.T) {
	type Inner struct {
		Baz string `json:"baz"`
	}

	type Foo struct {
		Bar Inner  `json:"barbarbar"`
		Num int    `json:"num"`
		Str string // no json tag, should use field name
	}

	data := Foo{
		Bar: Inner{Baz: "test value"},
		Num: 42,
		Str: "default",
	}

	tests := []struct {
		name     string
		pointer  string
		expected any
		wantErr  bool
	}{
		{
			name:     "root",
			pointer:  "",
			expected: data,
			wantErr:  false,
		},
		{
			name:     "nested with json tag",
			pointer:  "/barbarbar/baz",
			expected: "test value",
			wantErr:  false,
		},
		{
			name:     "field with json tag",
			pointer:  "/num",
			expected: 42,
			wantErr:  false,
		},
		{
			name:     "field without json tag",
			pointer:  "/Str",
			expected: "default",
			wantErr:  false,
		},
		{
			name:    "non-existent field",
			pointer: "/nonexistent",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ptr, err := jsptr.New(tt.pointer)
			require.NoError(t, err)

			var result any
			err = ptr.Retrieve(&result, data)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestPointerRetrieveTypedResults(t *testing.T) {
	jsonData := `{
		"str": "hello",
		"num": 42,
		"bool": true,
		"array": [1, 2, 3]
	}`

	t.Run("string result", func(t *testing.T) {
		ptr, err := jsptr.New("/str")
		require.NoError(t, err)

		var result string
		err = ptr.Retrieve(&result, []byte(jsonData))
		require.NoError(t, err)
		require.Equal(t, "hello", result)
	})

	t.Run("number result", func(t *testing.T) {
		ptr, err := jsptr.New("/num")
		require.NoError(t, err)

		var result float64
		err = ptr.Retrieve(&result, []byte(jsonData))
		require.NoError(t, err)
		require.Equal(t, 42.0, result)
	})

	t.Run("boolean result", func(t *testing.T) {
		ptr, err := jsptr.New("/bool")
		require.NoError(t, err)

		var result bool
		err = ptr.Retrieve(&result, []byte(jsonData))
		require.NoError(t, err)
		require.Equal(t, true, result)
	})

	t.Run("array result", func(t *testing.T) {
		ptr, err := jsptr.New("/array")
		require.NoError(t, err)

		var result []any
		err = ptr.Retrieve(&result, []byte(jsonData))
		require.NoError(t, err)
		require.Equal(t, []any{1.0, 2.0, 3.0}, result)
	})
}

func TestPointerEscaping(t *testing.T) {
	jsonData := `{
		"foo/bar": "value1",
		"foo~bar": "value2",
		"foo~1bar": "value3"
	}`

	tests := []struct {
		name     string
		pointer  string
		expected string
	}{
		{
			name:     "escaped slash",
			pointer:  "/foo~1bar",
			expected: "value1",
		},
		{
			name:     "escaped tilde",
			pointer:  "/foo~0bar",
			expected: "value2",
		},
		{
			name:     "literal tilde and one",
			pointer:  "/foo~01bar",
			expected: "value3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ptr, err := jsptr.New(tt.pointer)
			require.NoError(t, err)

			var result string
			err = ptr.Retrieve(&result, []byte(jsonData))
			require.NoError(t, err)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestPointerComplexExample(t *testing.T) {
	// This is the example from the spec
	type Inner struct {
		Baz string `json:"baz"`
	}

	type Foo struct {
		Bar Inner `json:"barbarbar"`
	}

	data := Foo{
		Bar: Inner{Baz: "test value"},
	}

	ptr, err := jsptr.New("/barbarbar/baz")
	require.NoError(t, err)

	var result string
	err = ptr.Retrieve(&result, data)
	require.NoError(t, err)
	require.Equal(t, "test value", result)
}

// CustomSource implements Source interface for testing
type CustomSource struct {
	data map[string]any
}

func (c *CustomSource) RetrieveJSONPointer(dst any, ptrspec string) error {
	// Custom implementation that prefixes all keys with "custom_"
	ptr, err := jsptr.New(ptrspec)
	if err != nil {
		return err
	}

	if ptrspec == "" {
		return blackmagic.AssignIfCompatible(dst, c.data)
	}

	// For this test, we'll look for keys with "custom_" prefix
	if len(ptr.Pattern()) > 0 && ptr.Pattern()[0] == '/' {
		key := "custom_" + ptr.Pattern()[1:]
		if value, exists := c.data[key]; exists {
			return blackmagic.AssignIfCompatible(dst, value)
		}
	}
	
	return fmt.Errorf("key not found")
}

func TestPointerWithCustomSource(t *testing.T) {
	customSource := &CustomSource{
		data: map[string]any{
			"custom_foo": "custom value",
			"custom_bar": 42,
		},
	}

	ptr, err := jsptr.New("/foo")
	require.NoError(t, err)

	var result string
	err = ptr.Retrieve(&result, customSource)
	require.NoError(t, err)
	require.Equal(t, "custom value", result)
}

func TestPointerWithDifferentSliceTypes(t *testing.T) {
	tests := []struct {
		name     string
		data     any
		pointer  string
		expected any
		wantErr  bool
	}{
		{
			name:     "[]int slice",
			data:     []int{1, 2, 3},
			pointer:  "/1",
			expected: 2,
			wantErr:  false,
		},
		{
			name:     "[]string slice",
			data:     []string{"a", "b", "c"},
			pointer:  "/0",
			expected: "a",
			wantErr:  false,
		},
		{
			name:     "[3]int array",
			data:     [3]int{10, 20, 30},
			pointer:  "/2",
			expected: 30,
			wantErr:  false,
		},
		{
			name:     "nested [][]int",
			data:     [][]int{{1, 2}, {3, 4}},
			pointer:  "/0/1",
			expected: 2,
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ptr, err := jsptr.New(tt.pointer)
			require.NoError(t, err)

			var result any
			err = ptr.Retrieve(&result, tt.data)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestPointerWithScalarTypes(t *testing.T) {
	tests := []struct {
		name     string
		data     any
		pointer  string
		expected any
		wantErr  bool
	}{
		{
			name:     "int scalar - root",
			data:     42,
			pointer:  "",
			expected: 42,
			wantErr:  false,
		},
		{
			name:    "int scalar - invalid pointer",
			data:    42,
			pointer: "/foo",
			wantErr: true,
		},
		{
			name:     "bool scalar - root",
			data:     true,
			pointer:  "",
			expected: true,
			wantErr:  false,
		},
		{
			name:    "bool scalar - invalid pointer",
			data:    true,
			pointer: "/0",
			wantErr: true,
		},
		{
			name:     "float64 scalar - root",
			data:     3.14,
			pointer:  "",
			expected: 3.14,
			wantErr:  false,
		},
		{
			name:    "nil scalar - invalid pointer",
			data:    nil,
			pointer: "/foo",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ptr, err := jsptr.New(tt.pointer)
			require.NoError(t, err)

			var result any
			err = ptr.Retrieve(&result, tt.data)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestPointerWithDifferentMapTypes(t *testing.T) {
	tests := []struct {
		name     string
		data     any
		pointer  string
		expected any
		wantErr  bool
	}{
		{
			name:     "map[string]int",
			data:     map[string]int{"foo": 42, "bar": 24},
			pointer:  "/foo",
			expected: 42,
			wantErr:  false,
		},
		{
			name:     "map[string]string",
			data:     map[string]string{"key": "value"},
			pointer:  "/key",
			expected: "value",
			wantErr:  false,
		},
		{
			name:    "map[int]string - should be scalar",
			data:    map[int]string{1: "one", 2: "two"},
			pointer: "/1",
			wantErr: true, // Should be treated as scalar, not indexable
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ptr, err := jsptr.New(tt.pointer)
			require.NoError(t, err)

			var result any
			err = ptr.Retrieve(&result, tt.data)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.expected, result)
		})
	}
}
func TestPointerWithInvalidJSON(t *testing.T) {
	// Test that invalid JSON is properly handled during source creation
	invalidJSON := `{"foo": "bar", "invalid": }`
	
	ptr, err := jsptr.New("/foo")
	require.NoError(t, err)
	
	var result string
	err = ptr.Retrieve(&result, []byte(invalidJSON))
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to parse JSON")
}
