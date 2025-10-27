package publish

import (
	"encoding/json"

	"github.com/linkedin/goavro/v2"
	"github.com/riferrei/srclient"
)

type Encoder interface {
	Encode(subject string, schema string, data map[string]any) ([]byte, error)
}
type JSONEncoder struct{}

func (JSONEncoder) Encode(subject, schema string, data map[string]any) ([]byte, error) {
	return json.Marshal(data)
}

type AvroEncoder struct {
	Client *srclient.SchemaRegistryClient
}

func (a AvroEncoder) Encode(subject, schema string, data map[string]any) ([]byte, error) {
	sch, err := a.Client.GetLatestSchema(subject)
	if err != nil {
		sch, err = a.Client.CreateSchema(subject, schema, srclient.Avro)
		if err != nil {
			return nil, err
		}
	}
	codec, err := goavro.NewCodec(sch.Schema())
	if err != nil {
		return nil, err
	}
	return codec.BinaryFromNative(nil, data)
}
