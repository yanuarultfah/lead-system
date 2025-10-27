package config

import (
	"os"
	"regexp"
	"time"

	"gopkg.in/yaml.v3"
)

type AppCfg struct {
	Name         string        `yaml:"name"`
	Env          string        `yaml:"env"`
	ListenAddr   string        `yaml:"listen_addr"`
	ReadTimeout  time.Duration `yaml:"read_timeout"`
	WriteTimeout time.Duration `yaml:"write_timeout"`
	IdleTimeout  time.Duration `yaml:"idle_timeout"`
	MaxBodyBytes string        `yaml:"max_body_bytes"`
}
type LogCfg struct {
	Level   string `yaml:"level"`
	PIIMask bool   `yaml:"pii_mask"`
}
type DBCfg struct {
	URI                string        `yaml:"uri"`
	MaxConns           int32         `yaml:"max_conns"`
	MinConns           int32         `yaml:"min_conns"`
	MaxConnLifetime    time.Duration `yaml:"max_conn_lifetime"`
	MaxConnIdleTime    time.Duration `yaml:"max_conn_idle_time"`
	HealthzQuery       string        `yaml:"healthz_query"`
	StatementTimeoutMs int           `yaml:"statement_timeout_ms"`
}
type OTELCfg struct {
	Enable   bool   `yaml:"enable"`
	Endpoint string `yaml:"endpoint"`
}
type TrafficCfg struct {
	InjectGlitch   bool `yaml:"inject_glitch"`
	JitterPct      int  `yaml:"jitter_pct"`
	GlitchErrorPct int  `yaml:"glitch_error_pct"`
}
type APICfg struct {
	AllowOrigins []string `yaml:"allow_origins"`
}
type KafkaCfg struct {
	Enable           bool     `yaml:"enable"`
	Brokers          []string `yaml:"brokers"`
	Acks             string   `yaml:"acks"`
	Compression      string   `yaml:"compression"`
	TopicPrefix      string   `yaml:"topic_prefix"`
	SASLMechanism    string   `yaml:"sasl_mechanism"`
	SASLUsername     string   `yaml:"sasl_username"`
	SASLPassword     string   `yaml:"sasl_password"`
	SecurityProtocol string   `yaml:"security_protocol"`
}
type SRCfg struct {
	URL      string `yaml:"url"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
	Format   string `yaml:"format"` // json|avro
}
type Config struct {
	App            AppCfg     `yaml:"app"`
	Log            LogCfg     `yaml:"log"`
	DB             DBCfg      `yaml:"db"`
	OTEL           OTELCfg    `yaml:"otel"`
	Traffic        TrafficCfg `yaml:"traffic"`
	API            APICfg     `yaml:"api"`
	Kafka          KafkaCfg   `yaml:"kafka"`
	SchemaRegistry SRCfg      `yaml:"schema_registry"`
}

func Load() (*Config, error) {
	f := os.Getenv("CONFIG_FILE")
	if f == "" {
		f = "configs/config.yaml"
	}
	b, err := os.ReadFile(f)
	if err != nil {
		return nil, err
	}
	// Expand ${VAR} and ${VAR:-default} style variables in the file before YAML unmarshalling
	content := string(b)
	// pattern captures ${VAR} or ${VAR:-default}
	re := regexp.MustCompile(`\$\{([A-Za-z0-9_]+)(:-([^}]*))?}`)
	content = re.ReplaceAllStringFunc(content, func(s string) string {
		sub := re.FindStringSubmatch(s)
		if len(sub) < 2 {
			return s
		}
		name := sub[1]
		def := ""
		if len(sub) >= 4 {
			def = sub[3]
		}
		if v, ok := os.LookupEnv(name); ok && v != "" {
			return v
		}
		return def
	})
	b = []byte(content)
	var c Config
	if err := yaml.Unmarshal(b, &c); err != nil {
		return nil, err
	}
	if v := os.Getenv("PG_URI"); v != "" {
		c.DB.URI = v
	}
	if v := os.Getenv("LOG_LEVEL"); v != "" {
		c.Log.Level = v
	}
	return &c, nil
}
