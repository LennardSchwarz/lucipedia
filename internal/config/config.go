package config

import (
	"encoding/json"
	"os"
	"strconv"
	"time"

	"github.com/rotisserie/eris"
)

// Config holds runtime configuration values for the Wikipedai server.
type Config struct {
	DBPath        string
	ServerPort    int
	LogLevel      string
	LLMEndpoint   string
	LLMAPIKey     string
	LLMModels     []string
	SentryDSN     string
	Environment   string
	ShutdownGrace time.Duration
}

const (
	defaultDBPath        = "./data/lucipedia.db"
	defaultServerPort    = 8080
	defaultLogLevel      = "info"
	defaultEnvironment   = "development"
	defaultShutdownGrace = 10 * time.Second
)

// Load reads configuration values from environment variables, applying defaults where necessary.
func Load() (*Config, error) {
	cfg := &Config{
		DBPath:        getEnv("DB_PATH", defaultDBPath),
		LogLevel:      getEnv("LOG_LEVEL", defaultLogLevel),
		LLMEndpoint:   os.Getenv("LLM_ENDPOINT"),
		LLMAPIKey:     os.Getenv("LLM_API_KEY"),
		SentryDSN:     os.Getenv("SENTRY_DSN"),
		Environment:   os.Getenv("ENV"),
		ShutdownGrace: defaultShutdownGrace,
	}

	if modelsJSON := os.Getenv("LLM_MODELS"); modelsJSON != "" {
		models, err := parseModels(modelsJSON)
		if err != nil {
			return nil, eris.Wrap(err, "parsing LLM_MODELS")
		}
		cfg.LLMModels = models
	}

	portValue := getEnv("SERVER_PORT", strconv.Itoa(defaultServerPort))
	port, err := strconv.Atoi(portValue)
	if err != nil {
		return nil, eris.Wrapf(err, "invalid SERVER_PORT value: %s", portValue)
	}
	cfg.ServerPort = port

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func parseModels(raw string) ([]string, error) {
	// Accept either a JSON array of strings or an object with a `models` field.
	var arrayInput []string
	if err := json.Unmarshal([]byte(raw), &arrayInput); err == nil {
		return arrayInput, nil
	}

	var objectInput struct {
		Models []string `json:"models"`
	}
	if err := json.Unmarshal([]byte(raw), &objectInput); err != nil {
		return nil, eris.Wrap(err, "decoding JSON")
	}

	if len(objectInput.Models) == 0 {
		return nil, eris.New("models list is empty")
	}

	return objectInput.Models, nil
}
