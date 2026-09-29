package orders

import (
	"ethgo/model"
	"strconv"
	"strings"
)

type HandlerFunc func(*Message) error

type Message struct {
	streamName string
	groupName  string
	messageId  string
	data       map[string]string
}

func (t *Message) ID() string {
	if id, ok := t.data["id"]; ok {
		return id
	}
	panic("id field must not be empty")
}

func (t *Message) Hash() string {
	if hash, ok := t.data["hash"]; ok {
		return hash
	}
	panic("hash field must not be empty")
}

func (t *Message) SetNonce(value string) {
	t.data["nonce"] = value
}

func (t *Message) Nonce() uint64 {
	if val, ok := t.data["nonce"]; ok {
		nonce, err := strconv.ParseUint(val, 10, 64)
		if err != nil {
			panic(err)
		}
		return nonce
	}

	panic("nonce field must not be empty")
}

func (t *Message) CreatedAt() int64 {
	values := strings.SplitN(t.messageId, "-", 2)
	createdAt, err := strconv.ParseInt(values[0], 10, 64)
	if err != nil {
		panic(err)
	}
	return createdAt
}

func (t *Message) Exists(key string) bool {
	_, ok := t.data[key]
	return ok
}

func (t *Message) Bytes(key string) []byte {
	return []byte(t.data[key])
}

func (t *Message) Set(key string, value string) {
	t.data[key] = value
}

func (t *Message) String(key string) string {
	return t.data[key]
}

func (t *Message) Int64(key string) (int64, error) {
	return strconv.ParseInt(t.String(key), 10, 64)
}

func (t *Message) Uint64(key string) (uint64, error) {
	return strconv.ParseUint(t.String(key), 10, 64)
}

func (t *Message) Source() string {
	return t.streamName
}

func (t *Message) Ack() error {
	var red = model.RedisPool.Get()
	defer red.Close()

	red.Send("XACK", t.streamName, t.groupName, t.messageId)
	// red.Send("XDEL", t.streamName, t.messageId)
	return red.Flush()
}
