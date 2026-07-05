package models

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
)

type JSONMap map[string]any
type JSONList []any

func (j JSONMap) Value() (driver.Value, error) {
	if j == nil {
		return "{}", nil
	}

	value, err := json.Marshal(j)
	if err != nil {
		return nil, err
	}

	return string(value), nil
}

func (j *JSONMap) Scan(value any) error {
	if value == nil {
		*j = JSONMap{}
		return nil
	}

	var bytes []byte
	switch typed := value.(type) {
	case []byte:
		bytes = typed
	case string:
		bytes = []byte(typed)
	default:
		return fmt.Errorf("unsupported JSONMap type %T", value)
	}

	if len(bytes) == 0 {
		*j = JSONMap{}
		return nil
	}

	return json.Unmarshal(bytes, j)
}

func (j JSONList) Value() (driver.Value, error) {
	if j == nil {
		return "[]", nil
	}

	value, err := json.Marshal(j)
	if err != nil {
		return nil, err
	}

	return string(value), nil
}

func (j *JSONList) Scan(value any) error {
	if value == nil {
		*j = JSONList{}
		return nil
	}

	var bytes []byte
	switch typed := value.(type) {
	case []byte:
		bytes = typed
	case string:
		bytes = []byte(typed)
	default:
		return fmt.Errorf("unsupported JSONList type %T", value)
	}

	if len(bytes) == 0 {
		*j = JSONList{}
		return nil
	}

	return json.Unmarshal(bytes, j)
}
