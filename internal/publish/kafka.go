package publish

import (
	"context"
	"strings"
	"time"

	"leads-system/internal/config"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riferrei/srclient"
	"go.uber.org/zap"
)

type Publisher struct {
	log    *zap.Logger
	cfg    *config.Config
	prod   *kafka.Producer
	enc    Encoder
	prefix string
	pool   *pgxpool.Pool
}

func NewPublisher(ctx context.Context, cfg *config.Config, log *zap.Logger, pool *pgxpool.Pool) (*Publisher, error) {
	if !cfg.Kafka.Enable {
		return &Publisher{log: log, cfg: cfg, enc: JSONEncoder{}, prefix: cfg.Kafka.TopicPrefix, pool: pool}, nil
	}
	conf := &kafka.ConfigMap{"bootstrap.servers": strings.Join(cfg.Kafka.Brokers, ","), "compression.type": cfg.Kafka.Compression, "acks": cfg.Kafka.Acks, "security.protocol": cfg.Kafka.SecurityProtocol}
	if cfg.Kafka.SASLMechanism != "" {
		conf.SetKey("sasl.mechanisms", cfg.Kafka.SASLMechanism)
		conf.SetKey("sasl.username", cfg.Kafka.SASLUsername)
		conf.SetKey("sasl.password", cfg.Kafka.SASLPassword)
	}
	prod, err := kafka.NewProducer(conf)
	if err != nil {
		return nil, err
	}

	var enc Encoder = JSONEncoder{}
	if strings.ToLower(cfg.SchemaRegistry.Format) == "avro" {
		cli := srclient.CreateSchemaRegistryClient(cfg.SchemaRegistry.URL)
		if cfg.SchemaRegistry.Username != "" {
			cli.SetCredentials(cfg.SchemaRegistry.Username, cfg.SchemaRegistry.Password)
		}
		enc = AvroEncoder{Client: cli}
	}
	p := &Publisher{log: log, cfg: cfg, prod: prod, enc: enc, prefix: cfg.Kafka.TopicPrefix, pool: pool}
	go func() {
		for e := range prod.Events() {
			if m, ok := e.(*kafka.Message); ok && m.TopicPartition.Error != nil {
				log.Error("kafka delivery failed", zap.Error(m.TopicPartition.Error))
			}
		}
	}()
	return p, nil
}
func (p *Publisher) Close() {
	if p.prod != nil {
		p.prod.Flush(2000)
		p.prod.Close()
	}
}
func (p *Publisher) PublishOutbox(eventType string, payload map[string]any) error {
	topic := p.prefix + strings.ToLower(strings.ReplaceAll(eventType, "_", "-"))
	schema := DefaultSchema(eventType)
	bytes, err := p.enc.Encode(topic+"-value", schema, payload)
	if err != nil {
		return err
	}
	if p.prod == nil {
		p.log.Info("DRY-PUBLISH", zap.String("topic", topic), zap.Int("bytes", len(bytes)))
		return nil
	}
	msg := &kafka.Message{TopicPartition: kafka.TopicPartition{Topic: &topic, Partition: kafka.PartitionAny}, Value: bytes, Timestamp: time.Now()}
	return p.prod.Produce(msg, nil)
}
