package orders

import (
	"errors"
	"fmt"
	"strconv"
)

var ErrNil = errors.New("modifier: nil returned")
var ErrNegativeInt = errors.New("modifier: unexpected value for Uint64")

type Modifier struct {
	id     string
	hash   string
	nonce  uint64
	fields map[string]interface{}
}

func NewModifier(id, hash string, nonce uint64) *Modifier {
	var value = &Modifier{
		id:     id,
		nonce:  nonce,
		hash:   hash,
		fields: map[string]interface{}{},
	}

	value.Set("id", id)
	value.Set("hash", hash)
	value.Set("nonce", nonce)
	return value
}

func (m *Modifier) ID() string {
	return m.id
}

func (m *Modifier) Hash() string {
	return m.hash
}

func (m *Modifier) Nonce() uint64 {
	return m.nonce
}

func (m *Modifier) Exists(key string) bool {
	_, ok := m.fields[key]
	return ok
}

func (m *Modifier) Fields() map[string]interface{} {
	return m.fields
}

func (m *Modifier) Values() []interface{} {
	var args []interface{}
	for key, val := range m.fields {
		args = append(args, key, val)
	}
	return args
}

func (m *Modifier) Get(key string) interface{} {
	return m.fields[key]
}

func (m *Modifier) Set(key string, val interface{}) {
	m.fields[key] = val
}

func (m *Modifier) Bool(key string) (bool, error) {
	switch val := m.Get(key).(type) {
	case int64:
		return val != 0, nil
	case []byte:
		return strconv.ParseBool(string(val))
	case nil:
		return false, ErrNil
	default:
		return false, fmt.Errorf("unexpected type for Bool, got type %T", val)
	}
}

func (m *Modifier) Bytes(key string) ([]byte, error) {
	switch val := m.Get(key).(type) {
	case []byte:
		return val, nil
	case string:
		return []byte(val), nil
	case nil:
		return nil, ErrNil
	default:
		return nil, fmt.Errorf("unexpected type for Bytes, got type %T", val)
	}
}

func (m *Modifier) Int(key string) (int, error) {
	switch val := m.Get(key).(type) {
	case int64:
		x := int(val)
		if int64(x) != val {
			return 0, strconv.ErrRange
		}
		return x, nil
	case []byte:
		n, err := strconv.ParseInt(string(val), 10, 0)
		return int(n), err
	case string:
		n, err := strconv.ParseInt(val, 10, 0)
		return int(n), err
	case nil:
		return 0, ErrNil
	default:
		return 0, fmt.Errorf("unexpected type for Int, got type %T", val)
	}
}

func (m *Modifier) Int64(key string) (int64, error) {
	switch val := m.Get(key).(type) {
	case int64:
		return val, nil
	case []byte:
		n, err := strconv.ParseInt(string(val), 10, 64)
		return n, err
	case string:
		n, err := strconv.ParseInt(val, 10, 64)
		return n, err
	case nil:
		return 0, ErrNil
	default:
		return 0, fmt.Errorf("unexpected type for Int64, got type %T", val)
	}
}

func (m *Modifier) String(key string) (string, error) {
	switch val := m.Get(key).(type) {
	case []byte:
		return string(val), nil
	case string:
		return val, nil
	case nil:
		return "", ErrNil
	default:
		return "", fmt.Errorf("unexpected type for String, got type %T", val)
	}
}

func (m *Modifier) Uint64(key string) (uint64, error) {
	switch val := m.Get(key).(type) {
	case int64:
		if val < 0 {
			return 0, ErrNegativeInt
		}
		return uint64(val), nil
	case []byte:
		n, err := strconv.ParseUint(string(val), 10, 64)
		return n, err
	case string:
		n, err := strconv.ParseUint(val, 10, 64)
		return n, err
	case nil:
		return 0, ErrNil
	default:
		return 0, fmt.Errorf("unexpected type for Uint64, got type %T", val)
	}
}
