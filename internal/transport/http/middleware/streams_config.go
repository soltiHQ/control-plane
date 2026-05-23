package middleware

// StreamsConfig parametrises streaming-endpoint protections (SSE log tail,
// future watches). Kept separate from the failure-based RateLimit because
// streams hold open connections — error-rate isn't a good proxy.
type StreamsConfig struct {
	// MaxPerIP caps concurrent streams from a single source IP.
	// 0 or negative disables the cap.
	MaxPerIP int `yaml:"max_per_ip" envconfig:"MAX_PER_IP"`

	// SubscriberBuffer is the per-subscriber channel buffer in the
	// log-fanout hub. Larger = more tolerance for slow readers before a
	// LaggedProto is emitted. 0 uses the package default (64).
	SubscriberBuffer int `yaml:"subscriber_buffer" envconfig:"SUBSCRIBER_BUFFER"`
}
