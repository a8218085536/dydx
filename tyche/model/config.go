package model

type Config struct {
	// Maximum number of idle connections in the pool.
	MaxIdle int `toml:"maxIdle" json:"maxIdle"`

	// Maximum number of connections allocated by the pool at a given time.
	// When zero, there is no limit on the number of connections in the pool.
	MaxActive int `toml:"maxActive" json:"maxActive"`

	// Close connections after remaining idle for this duration. If the value
	// is zero, then idle connections are not closed. Applications should set
	// the timeout to a value less than the server's timeout.
	IdleTimeout int `toml:"idleTimeout" json:"idleTimeout"`

	// Prefix of all keys
	Domain    string `toml:"domain" json:"domain"`
	Namespace string `toml:"-" json:"-"`

	// URI scheme. URLs should follow the draft IANA specification for the
	// scheme (https://www.iana.org/assignments/uri-schemes/prov/redis).
	URI string `toml:"uri" json:"uri"`
}
