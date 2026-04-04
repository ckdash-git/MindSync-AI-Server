package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Config holds all configuration for the application.
type Config struct {
	Server     ServerConfig
	Database   DatabaseConfig
	JWT        JWTConfig
	Encryption EncryptionConfig
	OpenRouter OpenRouterConfig
	Privacy    PrivacyConfig
	RateLimit  RateLimitConfig
	Timeout    TimeoutConfig
	CORS       CORSConfig
	Log        LogConfig
}

type ServerConfig struct {
	Host            string        `mapstructure:"SERVER_HOST"`
	Port            int           `mapstructure:"SERVER_PORT"`
	Env             string        `mapstructure:"SERVER_ENV"`
	ReadTimeout     time.Duration `mapstructure:"SERVER_READ_TIMEOUT"`
	WriteTimeout    time.Duration `mapstructure:"SERVER_WRITE_TIMEOUT"`
	ShutdownTimeout time.Duration `mapstructure:"SERVER_SHUTDOWN_TIMEOUT"`
}

type DatabaseConfig struct {
	Host            string        `mapstructure:"DB_HOST"`
	Port            int           `mapstructure:"DB_PORT"`
	User            string        `mapstructure:"DB_USER"`
	Password        string        `mapstructure:"DB_PASSWORD"`
	Name            string        `mapstructure:"DB_NAME"`
	SSLMode         string        `mapstructure:"DB_SSLMODE"`
	MaxOpenConns    int           `mapstructure:"DB_MAX_OPEN_CONNS"`
	MaxIdleConns    int           `mapstructure:"DB_MAX_IDLE_CONNS"`
	ConnMaxLifetime time.Duration `mapstructure:"DB_CONN_MAX_LIFETIME"`
}

// DSN returns the PostgreSQL connection string.
func (d DatabaseConfig) DSN() string {
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=%s",
		d.User, d.Password, d.Host, d.Port, d.Name, d.SSLMode,
	)
}

type JWTConfig struct {
	Secret        string        `mapstructure:"JWT_SECRET"`
	AccessExpiry  time.Duration `mapstructure:"JWT_ACCESS_EXPIRY"`
	RefreshExpiry time.Duration `mapstructure:"JWT_REFRESH_EXPIRY"`
}

type EncryptionConfig struct {
	Keys       map[string]string // version -> hex-encoded key
	CurrentVer string            `mapstructure:"ENCRYPTION_KEY_CURRENT"`
}

type OpenRouterConfig struct {
	BaseURL      string        `mapstructure:"OPENROUTER_BASE_URL"`
	DefaultModel string        `mapstructure:"OPENROUTER_DEFAULT_MODEL"`
	Timeout      time.Duration `mapstructure:"OPENROUTER_TIMEOUT"`
	MaxRetries   int           `mapstructure:"OPENROUTER_MAX_RETRIES"`
}

type PrivacyConfig struct {
	Mode string `mapstructure:"PRIVACY_MODE"`
}

type RateLimitConfig struct {
	AuthRPM   int `mapstructure:"RATE_LIMIT_AUTH_RPM"`
	ChatRPM   int `mapstructure:"RATE_LIMIT_CHAT_RPM"`
	StreamRPM int `mapstructure:"RATE_LIMIT_STREAM_RPM"`
}

type TimeoutConfig struct {
	Default time.Duration `mapstructure:"REQUEST_TIMEOUT_DEFAULT"`
	Stream  time.Duration `mapstructure:"REQUEST_TIMEOUT_STREAM"`
}

type CORSConfig struct {
	AllowedOrigins []string
	AllowedMethods []string
	AllowedHeaders []string
}

type LogConfig struct {
	Level string `mapstructure:"LOG_LEVEL"`
}

// Load reads configuration from .env file and environment variables.
// Environment variables take precedence over .env file values.
func Load() (*Config, error) {
	v := viper.New()

	// Read from .env file if it exists
	v.SetConfigFile(".env")
	v.SetConfigType("env")
	_ = v.ReadInConfig() // Ignore error; env vars may suffice

	// Environment variables override file values
	v.AutomaticEnv()

	// Set defaults
	setDefaults(v)

	cfg := &Config{}

	// Server
	cfg.Server = ServerConfig{
		Host:            v.GetString("SERVER_HOST"),
		Port:            v.GetInt("SERVER_PORT"),
		Env:             v.GetString("SERVER_ENV"),
		ReadTimeout:     v.GetDuration("SERVER_READ_TIMEOUT"),
		WriteTimeout:    v.GetDuration("SERVER_WRITE_TIMEOUT"),
		ShutdownTimeout: v.GetDuration("SERVER_SHUTDOWN_TIMEOUT"),
	}

	// Database
	cfg.Database = DatabaseConfig{
		Host:            v.GetString("DB_HOST"),
		Port:            v.GetInt("DB_PORT"),
		User:            v.GetString("DB_USER"),
		Password:        v.GetString("DB_PASSWORD"),
		Name:            v.GetString("DB_NAME"),
		SSLMode:         v.GetString("DB_SSLMODE"),
		MaxOpenConns:    v.GetInt("DB_MAX_OPEN_CONNS"),
		MaxIdleConns:    v.GetInt("DB_MAX_IDLE_CONNS"),
		ConnMaxLifetime: v.GetDuration("DB_CONN_MAX_LIFETIME"),
	}

	// JWT
	cfg.JWT = JWTConfig{
		Secret:        v.GetString("JWT_SECRET"),
		AccessExpiry:  v.GetDuration("JWT_ACCESS_EXPIRY"),
		RefreshExpiry: v.GetDuration("JWT_REFRESH_EXPIRY"),
	}

	// Encryption keys (supports rotation: ENCRYPTION_KEY_V1, V2, etc.)
	cfg.Encryption = loadEncryptionConfig(v)

	// OpenRouter
	cfg.OpenRouter = OpenRouterConfig{
		BaseURL:      v.GetString("OPENROUTER_BASE_URL"),
		DefaultModel: v.GetString("OPENROUTER_DEFAULT_MODEL"),
		Timeout:      v.GetDuration("OPENROUTER_TIMEOUT"),
		MaxRetries:   v.GetInt("OPENROUTER_MAX_RETRIES"),
	}

	// Privacy
	cfg.Privacy = PrivacyConfig{
		Mode: v.GetString("PRIVACY_MODE"),
	}

	// Rate Limiting
	cfg.RateLimit = RateLimitConfig{
		AuthRPM:   v.GetInt("RATE_LIMIT_AUTH_RPM"),
		ChatRPM:   v.GetInt("RATE_LIMIT_CHAT_RPM"),
		StreamRPM: v.GetInt("RATE_LIMIT_STREAM_RPM"),
	}

	// Timeout
	cfg.Timeout = TimeoutConfig{
		Default: v.GetDuration("REQUEST_TIMEOUT_DEFAULT"),
		Stream:  v.GetDuration("REQUEST_TIMEOUT_STREAM"),
	}

	// CORS
	cfg.CORS = CORSConfig{
		AllowedOrigins: splitAndTrim(v.GetString("CORS_ALLOWED_ORIGINS")),
		AllowedMethods: splitAndTrim(v.GetString("CORS_ALLOWED_METHODS")),
		AllowedHeaders: splitAndTrim(v.GetString("CORS_ALLOWED_HEADERS")),
	}

	// Logging
	cfg.Log = LogConfig{
		Level: v.GetString("LOG_LEVEL"),
	}

	if err := validate(cfg); err != nil {
		return nil, fmt.Errorf("config validation: %w", err)
	}

	return cfg, nil
}

// loadEncryptionConfig scans for ENCRYPTION_KEY_V{N} environment variables.
func loadEncryptionConfig(v *viper.Viper) EncryptionConfig {
	ec := EncryptionConfig{
		Keys:       make(map[string]string),
		CurrentVer: v.GetString("ENCRYPTION_KEY_CURRENT"),
	}

	// Scan for versioned keys: ENCRYPTION_KEY_V1, ENCRYPTION_KEY_V2, etc.
	for i := 1; i <= 10; i++ {
		key := fmt.Sprintf("ENCRYPTION_KEY_V%d", i)
		val := v.GetString(key)
		if val != "" {
			ver := fmt.Sprintf("v%d", i)
			ec.Keys[ver] = val
		}
	}

	return ec
}

func validate(cfg *Config) error {
	if cfg.Server.Port < 1 || cfg.Server.Port > 65535 {
		return fmt.Errorf("invalid server port: %d", cfg.Server.Port)
	}
	if cfg.JWT.Secret == "" {
		return fmt.Errorf("JWT_SECRET is required")
	}
	if len(cfg.JWT.Secret) < 32 {
		return fmt.Errorf("JWT_SECRET must be at least 32 characters")
	}
	if cfg.Encryption.CurrentVer == "" {
		return fmt.Errorf("ENCRYPTION_KEY_CURRENT is required")
	}
	if _, ok := cfg.Encryption.Keys[cfg.Encryption.CurrentVer]; !ok {
		return fmt.Errorf("ENCRYPTION_KEY_%s not found for current version %q",
			strings.ToUpper(cfg.Encryption.CurrentVer), cfg.Encryption.CurrentVer)
	}
	return nil
}

func setDefaults(v *viper.Viper) {
	v.SetDefault("SERVER_HOST", "0.0.0.0")
	v.SetDefault("SERVER_PORT", 8080)
	v.SetDefault("SERVER_ENV", "development")
	v.SetDefault("SERVER_READ_TIMEOUT", "30s")
	v.SetDefault("SERVER_WRITE_TIMEOUT", "30s")
	v.SetDefault("SERVER_SHUTDOWN_TIMEOUT", "15s")

	v.SetDefault("DB_HOST", "localhost")
	v.SetDefault("DB_PORT", 5432)
	v.SetDefault("DB_USER", "mindsync")
	v.SetDefault("DB_NAME", "mindsync")
	v.SetDefault("DB_SSLMODE", "disable")
	v.SetDefault("DB_MAX_OPEN_CONNS", 25)
	v.SetDefault("DB_MAX_IDLE_CONNS", 5)
	v.SetDefault("DB_CONN_MAX_LIFETIME", "5m")

	v.SetDefault("JWT_ACCESS_EXPIRY", "15m")
	v.SetDefault("JWT_REFRESH_EXPIRY", "168h")

	v.SetDefault("OPENROUTER_BASE_URL", "https://openrouter.ai/api/v1")
	v.SetDefault("OPENROUTER_DEFAULT_MODEL", "openai/gpt-4o")
	v.SetDefault("OPENROUTER_TIMEOUT", "120s")
	v.SetDefault("OPENROUTER_MAX_RETRIES", 3)

	v.SetDefault("PRIVACY_MODE", "standard")

	v.SetDefault("RATE_LIMIT_AUTH_RPM", 5)
	v.SetDefault("RATE_LIMIT_CHAT_RPM", 60)
	v.SetDefault("RATE_LIMIT_STREAM_RPM", 10)

	v.SetDefault("REQUEST_TIMEOUT_DEFAULT", "30s")
	v.SetDefault("REQUEST_TIMEOUT_STREAM", "120s")

	v.SetDefault("CORS_ALLOWED_ORIGINS", "http://localhost:3000")
	v.SetDefault("CORS_ALLOWED_METHODS", "GET,POST,PUT,DELETE,OPTIONS")
	v.SetDefault("CORS_ALLOWED_HEADERS", "Content-Type,Authorization,X-Request-ID")

	v.SetDefault("LOG_LEVEL", "info")
}

func splitAndTrim(s string) []string {
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}
