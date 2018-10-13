package gokv

import (
	"encoding/json"
)

// Store is an abstraction for different key-value store implementations.
// A store must be able to store and retrieve key-value pairs,
// with the key being a string and the value being any Go interface{}.
type Store interface {
	// Set stores the given value for the given key.
	// The implementation automatically marshalls the value if required.
	// The marshalling target depends on the implementation. It can be JSON, gob etc.
	// Implementations should offer a configuration for this.
	Set(string, interface{}) error
	// Get retrieves the value for the given key.
	// The implementation automatically unmarshalls the value if required.
	// The unmarshalling source depends on the implementation. It can be JSON, gob etc.
	// The automatic unmarshalling requires a pointer to a proper type being passed as parameter.
	// The Get method will populate the fields of the object that the passed pointer
	// points to with the values of the retrieved object's values.
	// If no object is found it returns (false, nil).
	Get(string, interface{}) (bool, error)
}

func toJSON(v interface{}) ([]byte, error) {
	return json.Marshal(v)
}

func fromJSON(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}
