package dydxtrading

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/Shopify/sarama"
)

const DefaultIndexerKafkaTopic = "to-ender"

type kafkaConsumer interface {
	ConsumePartition(topic string, partition int32, offset int64) (kafkaPartitionConsumer, error)
	Close() error
}

type kafkaPartitionConsumer interface {
	Messages() <-chan *sarama.ConsumerMessage
	Errors() <-chan *sarama.ConsumerError
	Close() error
}

type KafkaIndexerBlockMessageSourceConfig struct {
	Brokers     []string
	Topic       string
	Partition   int32
	MaxMessages int
	ReadTimeout time.Duration
	Consumer    kafkaConsumer
}

type KafkaIndexerBlockMessageSource struct {
	consumer    kafkaConsumer
	topic       string
	partition   int32
	maxMessages int
	readTimeout time.Duration
}

func NewKafkaIndexerBlockMessageSource(cfg KafkaIndexerBlockMessageSourceConfig) (*KafkaIndexerBlockMessageSource, error) {
	consumer := cfg.Consumer
	if consumer == nil {
		if len(cfg.Brokers) == 0 {
			return nil, fmt.Errorf("missing Kafka brokers")
		}
		created, err := sarama.NewConsumer(cfg.Brokers, nil)
		if err != nil {
			return nil, err
		}
		consumer = saramaKafkaConsumer{consumer: created}
	}
	topic := cfg.Topic
	if topic == "" {
		topic = DefaultIndexerKafkaTopic
	}
	maxMessages := cfg.MaxMessages
	if maxMessages <= 0 {
		maxMessages = 100
	}
	readTimeout := cfg.ReadTimeout
	if readTimeout <= 0 {
		readTimeout = time.Second
	}
	return &KafkaIndexerBlockMessageSource{
		consumer:    consumer,
		topic:       topic,
		partition:   cfg.Partition,
		maxMessages: maxMessages,
		readTimeout: readTimeout,
	}, nil
}

type saramaKafkaConsumer struct {
	consumer sarama.Consumer
}

func (c saramaKafkaConsumer) ConsumePartition(topic string, partition int32, offset int64) (kafkaPartitionConsumer, error) {
	return c.consumer.ConsumePartition(topic, partition, offset)
}

func (c saramaKafkaConsumer) Close() error {
	return c.consumer.Close()
}

func (s *KafkaIndexerBlockMessageSource) FetchMessages(ctx context.Context, cursor string) ([]IndexerBlockMessage, string, error) {
	offset := sarama.OffsetOldest
	if cursor != "" {
		parsed, err := strconv.ParseInt(cursor, 10, 64)
		if err != nil {
			return nil, "", fmt.Errorf("invalid Kafka cursor %q: %w", cursor, err)
		}
		offset = parsed
	}
	partitionConsumer, err := s.consumer.ConsumePartition(s.topic, s.partition, offset)
	if err != nil {
		return nil, "", err
	}
	defer partitionConsumer.Close()

	timer := time.NewTimer(s.readTimeout)
	defer timer.Stop()

	var messages []IndexerBlockMessage
	nextCursor := cursor
	for len(messages) < s.maxMessages {
		select {
		case <-ctx.Done():
			return messages, nextCursor, ctx.Err()
		case message, ok := <-partitionConsumer.Messages():
			if !ok {
				return messages, nextCursor, nil
			}
			nextOffset := message.Offset + 1
			nextCursor = strconv.FormatInt(nextOffset, 10)
			messages = append(messages, IndexerBlockMessage{
				Offset: nextCursor,
				Value:  message.Value,
			})
		case consumerErr, ok := <-partitionConsumer.Errors():
			if !ok {
				continue
			}
			if consumerErr == nil || consumerErr.Err == nil {
				return messages, nextCursor, fmt.Errorf("Kafka consumer error")
			}
			return messages, nextCursor, consumerErr.Err
		case <-timer.C:
			return messages, nextCursor, nil
		}
	}
	return messages, nextCursor, nil
}
