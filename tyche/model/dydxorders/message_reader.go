package dydxorders

import (
	"ethgo/model"
	"ethgo/util/redisx"

	"github.com/garyburd/redigo/redis"
)

type Message struct {
	streamName string
	groupName  string
	messageID  string
	Data       map[string]string
}

func (m *Message) Ack() error {
	var red = model.RedisPool.Get()
	defer red.Close()

	red.Send("XACK", m.streamName, m.groupName, m.messageID)
	return red.Flush()
}

type MessageReader struct {
	streamName string
	groupName  string
	readerName string
}

func NewPendingReader(groupName, readerName string) (*MessageReader, error) {
	return newMessageReader(keys.Pending(), groupName, readerName)
}

func newMessageReader(streamName, groupName, readerName string) (*MessageReader, error) {
	var red = model.RedisPool.Get()
	defer red.Close()

	_, err := red.Do("XGROUP", "CREATE", streamName, groupName, "0-0", "MKSTREAM")
	if err != nil {
		if err.Error() != "BUSYGROUP Consumer Group name already exists" {
			return nil, err
		}
	}

	return &MessageReader{
		streamName: streamName,
		groupName:  groupName,
		readerName: readerName,
	}, nil
}

func (reader *MessageReader) ReadLatest(count int, timeoutMS int64) ([]*Message, error) {
	var red = model.RedisPool.Get()
	defer red.Close()

	args := redis.Args{}
	args = args.Add("GROUP", reader.groupName, reader.readerName)
	args = args.Add("BLOCK", timeoutMS)
	args = args.Add("COUNT", count)
	args = args.Add("STREAMS", reader.streamName, ">")

	streams, err := redisx.Streams(redis.Values(red.Do("XREADGROUP", args...)))
	if err != nil {
		return nil, err
	}
	if len(streams) == 0 {
		return nil, nil
	}

	messages := make([]*Message, 0, len(streams[0].Value))
	for _, msg := range streams[0].Value {
		messages = append(messages, &Message{
			streamName: reader.streamName,
			groupName:  reader.groupName,
			messageID:  msg.Id,
			Data:       msg.Data,
		})
	}
	return messages, nil
}
