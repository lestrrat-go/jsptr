package jsptr

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync"

	"github.com/lestrrat-go/blackmagic"
	"github.com/valyala/fastjson"
)

// Source is an interface for abstracting different data sources
type Source interface {
	RetrieveJSONPointer(dst any, ptrspec string) error
}

// Pointer represents a compiled JSON pointer
type Pointer struct {
	pattern string
	tokens  []string
}

// New creates a new JSON pointer from a path specification
func New(pathspec string) (*Pointer, error) {
	if pathspec == "" {
		return &Pointer{pattern: "", tokens: nil}, nil
	}

	if !strings.HasPrefix(pathspec, "/") {
		return nil, fmt.Errorf("JSON pointer must start with '/'")
	}

	// Split the path into tokens, skipping the empty first element
	parts := strings.Split(pathspec, "/")[1:]
	
	// Unescape each token
	tokens := make([]string, len(parts))
	for i, part := range parts {
		tokens[i] = unescapeToken(part)
	}

	return &Pointer{
		pattern: pathspec,
		tokens:  tokens,
	}, nil
}

// Pattern returns the original path specification
func (p *Pointer) Pattern() string {
	return p.pattern
}

// Retrieve retrieves the value at the JSON pointer location
func (p *Pointer) Retrieve(dst any, target any) error {
	// Create appropriate source based on target type
	source, err := createSource(target)
	if err != nil {
		return err
	}
	return source.RetrieveJSONPointer(dst, p.pattern)
}

// unescapeToken unescapes JSON pointer tokens
func unescapeToken(token string) string {
	// JSON pointer escaping: ~1 -> /, ~0 -> ~
	token = strings.ReplaceAll(token, "~1", "/")
	token = strings.ReplaceAll(token, "~0", "~")
	return token
}

// createSource creates an appropriate source for the given target
func createSource(target any) (Source, error) {
	// First check if target already implements Source interface
	if source, ok := target.(Source); ok {
		return source, nil
	}

	// Use reflection to properly detect types
	rv := reflect.ValueOf(target)
	if !rv.IsValid() {
		return scalarSource{data: target}, nil
	}

	// Handle specific types first
	switch v := target.(type) {
	case []byte:
		return createJSONSource(v)
	case string:
		return createJSONSource([]byte(v))
	case map[string]any:
		return mapSource{data: v}, nil
	}

	// Use reflection for more general type checking
	switch rv.Kind() {
	case reflect.Slice, reflect.Array:
		// Convert to []any for uniform handling
		length := rv.Len()
		slice := make([]any, length)
		for i := range length {
			slice[i] = rv.Index(i).Interface()
		}
		return sliceSource{data: slice}, nil
	case reflect.Map:
		// Only handle string-keyed maps
		if rv.Type().Key().Kind() == reflect.String {
			// Convert to map[string]any for uniform handling
			result := make(map[string]any)
			for _, key := range rv.MapKeys() {
				keyStr := key.String()
				value := rv.MapIndex(key).Interface()
				result[keyStr] = value
			}
			return mapSource{data: result}, nil
		}
		// Non-string-keyed maps cannot be accessed with JSON pointer
		return nil, fmt.Errorf("cannot use JSON pointer with non-string-keyed map type %s", rv.Type())
	case reflect.Struct:
		return structSource{data: target}, nil
	case reflect.Ptr:
		// For pointers, recurse with the pointed-to value
		if rv.IsNil() {
			return scalarSource{data: target}, nil
		}
		return createSource(rv.Elem().Interface())
	default:
		// Scalars (int, bool, float64, etc.)
		return scalarSource{data: target}, nil
	}
}

// createJSONSource creates a jsonSource with pre-parsed JSON data
func createJSONSource(data []byte) (Source, error) {
	var p fastjson.Parser
	parsed, err := p.ParseBytes(data)
	if err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}
	return jsonSource{data: data, parsed: parsed}, nil
}

// scalarSource handles scalar values (int, bool, float64, etc.)
type scalarSource struct {
	data any
}

func (s scalarSource) RetrieveJSONPointer(dst any, ptrspec string) error {
	// Scalars can only be retrieved with empty pointer
	if ptrspec != "" {
		return fmt.Errorf("cannot index into scalar value %T with pointer '%s'", s.data, ptrspec)
	}
	return blackmagic.AssignIfCompatible(dst, s.data)
}

// jsonSource handles JSON byte data
type jsonSource struct {
	data   []byte
	parsed *fastjson.Value
}

func (s jsonSource) RetrieveJSONPointer(dst any, ptrspec string) error {
	// Use cached parsed JSON data
	v := s.parsed

	// Handle empty pointer - return the parsed data directly
	if ptrspec == "" {
		return s.assignFromValue(dst, v)
	}

	// Parse the pointer and navigate to the value
	ptr, err := New(ptrspec)
	if err != nil {
		return err
	}

	// Navigate through the JSON using the pointer tokens
	current := v
	for _, token := range ptr.tokens {
		switch current.Type() {
		case fastjson.TypeObject:
			current = current.Get(token)
			if current == nil {
				return fmt.Errorf("property '%s' not found", token)
			}
		case fastjson.TypeArray:
			index, err := strconv.Atoi(token)
			if err != nil {
				return fmt.Errorf("invalid array index '%s'", token)
			}
			arr, err := current.Array()
			if err != nil {
				return fmt.Errorf("failed to get array: %w", err)
			}
			if index < 0 || index >= len(arr) {
				return fmt.Errorf("array index %d out of bounds", index)
			}
			current = arr[index]
		default:
			return fmt.Errorf("cannot index into %s with '%s'", current.Type(), token)
		}
	}

	return s.assignFromValue(dst, current)
}

// assignFromValue converts a fastjson.Value to a Go value and assigns it to dst
func (s jsonSource) assignFromValue(dst any, v *fastjson.Value) error {
	if v == nil {
		return blackmagic.AssignIfCompatible(dst, nil)
	}

	switch v.Type() {
	case fastjson.TypeNull:
		return blackmagic.AssignIfCompatible(dst, nil)
	case fastjson.TypeString:
		str, err := v.StringBytes()
		if err != nil {
			return fmt.Errorf("failed to get string value: %w", err)
		}
		return blackmagic.AssignIfCompatible(dst, string(str))
	case fastjson.TypeNumber:
		return blackmagic.AssignIfCompatible(dst, v.GetFloat64())
	case fastjson.TypeTrue:
		return blackmagic.AssignIfCompatible(dst, true)
	case fastjson.TypeFalse:
		return blackmagic.AssignIfCompatible(dst, false)
	case fastjson.TypeArray:
		arr, err := v.Array()
		if err != nil {
			return fmt.Errorf("failed to get array: %w", err)
		}
		result := make([]any, len(arr))
		for i, item := range arr {
			var temp any
			if err := s.assignFromValue(&temp, item); err != nil {
				return fmt.Errorf("failed to convert array item %d: %w", i, err)
			}
			result[i] = temp
		}
		return blackmagic.AssignIfCompatible(dst, result)
	case fastjson.TypeObject:
		obj, err := v.Object()
		if err != nil {
			return fmt.Errorf("failed to get object: %w", err)
		}
		result := make(map[string]any)
		obj.Visit(func(key []byte, val *fastjson.Value) {
			var temp any
			if err := s.assignFromValue(&temp, val); err == nil {
				result[string(key)] = temp
			}
		})
		return blackmagic.AssignIfCompatible(dst, result)
	default:
		return fmt.Errorf("unsupported JSON type: %s", v.Type())
	}
}

// mapSource handles map[string]any data
type mapSource struct {
	data map[string]any
}

func (s mapSource) RetrieveJSONPointer(dst any, ptrspec string) error {
	// Handle empty pointer - return the data directly
	if ptrspec == "" {
		return blackmagic.AssignIfCompatible(dst, s.data)
	}

	ptr, err := New(ptrspec)
	if err != nil {
		return err
	}

	current := any(s.data)
	
	for _, token := range ptr.tokens {
		switch curr := current.(type) {
		case map[string]any:
			val, exists := curr[token]
			if !exists {
				return fmt.Errorf("property '%s' not found", token)
			}
			current = val
		case []any:
			index, err := strconv.Atoi(token)
			if err != nil {
				return fmt.Errorf("invalid array index '%s'", token)
			}
			if index < 0 || index >= len(curr) {
				return fmt.Errorf("array index %d out of bounds", index)
			}
			current = curr[index]
		default:
			return fmt.Errorf("cannot index into %T with '%s'", current, token)
		}
	}

	return blackmagic.AssignIfCompatible(dst, current)
}

// sliceSource handles []any data
type sliceSource struct {
	data []any
}

func (s sliceSource) RetrieveJSONPointer(dst any, ptrspec string) error {
	// Handle empty pointer - return the data directly
	if ptrspec == "" {
		return blackmagic.AssignIfCompatible(dst, s.data)
	}

	ptr, err := New(ptrspec)
	if err != nil {
		return err
	}

	// First token must be an array index
	index, err := strconv.Atoi(ptr.tokens[0])
	if err != nil {
		return fmt.Errorf("invalid array index '%s'", ptr.tokens[0])
	}
	if index < 0 || index >= len(s.data) {
		return fmt.Errorf("array index %d out of bounds", index)
	}

	// If only one token, return the element
	if len(ptr.tokens) == 1 {
		return blackmagic.AssignIfCompatible(dst, s.data[index])
	}

	// Create new pointer for remaining tokens
	remainingPath := "/" + strings.Join(ptr.tokens[1:], "/")
	source, err := createSource(s.data[index])
	if err != nil {
		return err
	}
	return source.RetrieveJSONPointer(dst, remainingPath)
}

// structSource handles struct data with JSON tag caching
type structSource struct {
	data any
}

// Cache for struct field information
var (
	structCache = make(map[reflect.Type]*structInfo)
	cacheMutex  sync.RWMutex
)

type structInfo struct {
	fields map[string]*fieldInfo
}

type fieldInfo struct {
	index    []int
	jsonName string
}

func (s structSource) RetrieveJSONPointer(dst any, ptrspec string) error {
	// Handle empty pointer - return the data directly
	if ptrspec == "" {
		return blackmagic.AssignIfCompatible(dst, s.data)
	}

	ptr, err := New(ptrspec)
	if err != nil {
		return err
	}

	current := s.data
	
	for _, token := range ptr.tokens {
		current, err = s.getField(current, token)
		if err != nil {
			return err
		}
	}

	return blackmagic.AssignIfCompatible(dst, current)
}

func (s structSource) getField(obj any, fieldName string) (any, error) {
	val := reflect.ValueOf(obj)
	
	// Handle pointers
	for val.Kind() == reflect.Ptr {
		if val.IsNil() {
			return nil, fmt.Errorf("cannot access field of nil pointer")
		}
		val = val.Elem()
	}

	if val.Kind() != reflect.Struct {
		return nil, fmt.Errorf("cannot access field '%s' of non-struct type %T", fieldName, obj)
	}

	info := getStructInfo(val.Type())
	fieldInfo, exists := info.fields[fieldName]
	if !exists {
		return nil, fmt.Errorf("field '%s' not found in struct %T", fieldName, obj)
	}

	fieldVal := val.FieldByIndex(fieldInfo.index)
	return fieldVal.Interface(), nil
}

func getStructInfo(t reflect.Type) *structInfo {
	cacheMutex.RLock()
	if info, exists := structCache[t]; exists {
		cacheMutex.RUnlock()
		return info
	}
	cacheMutex.RUnlock()

	cacheMutex.Lock()
	defer cacheMutex.Unlock()

	// Double-check after acquiring write lock
	if info, exists := structCache[t]; exists {
		return info
	}

	info := &structInfo{
		fields: make(map[string]*fieldInfo),
	}

	// Process all fields, including embedded ones
	processFields(t, nil, info)

	structCache[t] = info
	return info
}

func processFields(t reflect.Type, index []int, info *structInfo) {
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		fieldIndex := append(index, i)

		// Handle embedded fields
		if field.Anonymous {
			fieldType := field.Type
			if fieldType.Kind() == reflect.Ptr {
				fieldType = fieldType.Elem()
			}
			if fieldType.Kind() == reflect.Struct {
				processFields(fieldType, fieldIndex, info)
			}
			continue
		}

		// Skip unexported fields
		if !field.IsExported() {
			continue
		}

		// Get JSON tag
		jsonTag := field.Tag.Get("json")
		if jsonTag == "-" {
			continue
		}

		// Parse JSON tag
		jsonName := field.Name
		if jsonTag != "" {
			parts := strings.Split(jsonTag, ",")
			if parts[0] != "" {
				jsonName = parts[0]
			}
		}

		info.fields[jsonName] = &fieldInfo{
			index:    fieldIndex,
			jsonName: jsonName,
		}
	}
}