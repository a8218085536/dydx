package dydxtrading

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Shopify/sarama"
)

func TestKafkaIndexerBlockMessageSourceFetchesMessagesFromCursor(t *testing.T) {
	consumer := &fakeKafkaConsumer{
		partition: &fakeKafkaPartitionConsumer{
			messages: channelWithMessages(
				&sarama.ConsumerMessage{Offset: 5, Value: []byte("block-5")},
				&sarama.ConsumerMessage{Offset: 6, Value: []byte("block-6")},
			),
			errors: make(chan *sarama.ConsumerError),
		},
	}
	source, err := NewKafkaIndexerBlockMessageSource(KafkaIndexerBlockMessageSourceConfig{
		Consumer:    consumer,
		Topic:       "to-ender",
		Partition:   2,
		MaxMessages: 2,
		ReadTimeout: time.Second,
	})
	if err != nil {
		t.Fatalf("NewKafkaIndexerBlockMessageSource() error = %v", err)
	}

	messages, nextCursor, err := source.FetchMessages(context.Background(), "5")

	if err != nil {
		t.Fatalf("FetchMessages() error = %v", err)
	}
	if consumer.topic != "to-ender" || consumer.partitionID != 2 || consumer.offset != 5 {
		t.Fatalf("consumer called topic=%q partition=%d offset=%d", consumer.topic, consumer.partitionID, consumer.offset)
	}
	if len(messages) != 2 {
		t.Fatalf("messages len = %d, want 2", len(messages))
	}
	if string(messages[0].Value) != "block-5" || messages[0].Offset != "6" {
		t.Fatalf("first message = %+v", messages[0])
	}
	if string(messages[1].Value) != "block-6" || messages[1].Offset != "7" {
		t.Fatalf("second message = %+v", messages[1])
	}
	if nextCursor != "7" {
		t.Fatalf("next cursor = %q, want 7", nextCursor)
	}
	if !consumer.partition.closed {
		t.Fatal("partition consumer was not closed")
	}
}

func TestKafkaIndexerBlockMessageSourceRejectsInvalidCursor(t *testing.T) {
	source, err := NewKafkaIndexerBlockMessageSource(KafkaIndexerBlockMessageSourceConfig{
		Consumer: &fakeKafkaConsumer{},
		Topic:    "to-ender",
	})
	if err != nil {
		t.Fatalf("NewKafkaIndexerBlockMessageSource() error = %v", err)
	}

	_, _, err = source.FetchMessages(context.Background(), "not-a-number")

	if err == nil {
		t.Fatal("FetchMessages() error = nil, want invalid cursor error")
	}
}

func TestKafkaIndexerBlockMessageSourceReturnsConsumerErrors(t *testing.T) {
	want := errors.New("consumer failed")
	source, err := NewKafkaIndexerBlockMessageSource(KafkaIndexerBlockMessageSourceConfig{
		Consumer: &fakeKafkaConsumer{
			partition: &fakeKafkaPartitionConsumer{
				messages: make(chan *sarama.ConsumerMessage),
				errors:   channelWithErrors(&sarama.ConsumerError{Err: want}),
			},
		},
		Topic:       "to-ender",
		ReadTimeout: time.Second,
	})
	if err != nil {
		t.Fatalf("NewKafkaIndexerBlockMessageSource() error = %v", err)
	}

	_, _, err = source.FetchMessages(context.Background(), "")

	if !errors.Is(err, want) {
		t.Fatalf("FetchMessages() error = %v, want %v", err, want)
	}
}

type fakeKafkaConsumer struct {
	topic       string
	partitionID int32
	offset      int64
	partition   *fakeKafkaPartitionConsumer
}

func (c *fakeKafkaConsumer) ConsumePartition(topic string, partition int32, offset int64) (kafkaPartitionConsumer, error) {
	c.topic = topic
	c.partitionID = partition
	c.offset = offset
	if c.partition == nil {
		c.partition = &fakeKafkaPartitionConsumer{
			messages: make(chan *sarama.ConsumerMessage),
			errors:   make(chan *sarama.ConsumerError),
		}
		close(c.partition.messages)
	}
	return c.partition, nil
}

func (c *fakeKafkaConsumer) Close() error {
	return nil
}

type fakeKafkaPartitionConsumer struct {
	messages chan *sarama.ConsumerMessage
	errors   chan *sarama.ConsumerError
	closed   bool
}

func (c *fakeKafkaPartitionConsumer) Messages() <-chan *sarama.ConsumerMessage {
	return c.messages
}

func (c *fakeKafkaPartitionConsumer) Errors() <-chan *sarama.ConsumerError {
	return c.errors
}

func (c *fakeKafkaPartitionConsumer) Close() error {
	c.closed = true
	return nil
}

func channelWithMessages(messages ...*sarama.ConsumerMessage) chan *sarama.ConsumerMessage {
	ch := make(chan *sarama.ConsumerMessage, len(messages))
	for _, message := range messages {
		ch <- message
	}
	close(ch)
	return ch
}

func channelWithErrors(errors ...*sarama.ConsumerError) chan *sarama.ConsumerError {
	ch := make(chan *sarama.ConsumerError, len(errors))
	for _, err := range errors {
		ch <- err
	}
	close(ch)
	return ch
}
