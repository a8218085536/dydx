package redisx

import (
	"github.com/garyburd/redigo/redis"
)

type Message struct {
	Id   string
	Data map[string]string
}

type Stream struct {
	Name  string
	Value []Message
}

func Streams(reply []interface{}, err error) ([]Stream, error) {
	if err != nil {
		return nil, err
	}

	var result = make([]Stream, 0)
	for streamIndex := 0; streamIndex < len(reply); streamIndex++ {
		var streamData, _ = redis.Values(reply[streamIndex], nil)
		var name, _ = redis.String(streamData[0], nil)
		var children, _ = redis.Values(streamData[1], nil)

		var messages = make([]Message, 0)
		for messageIndex := 0; messageIndex < len(children); messageIndex++ {
			var messageData, _ = redis.Values(children[messageIndex], nil)
			var id, _ = redis.String(messageData[0], nil)
			var msg, _ = redis.StringMap(messageData[1], nil)
			messages = append(messages, Message{Id: id, Data: msg})
		}

		result = append(result, Stream{Name: name, Value: messages})
	}

	return result, nil
}
