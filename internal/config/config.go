package config

import "os"

type Config struct {
	DBPath     string
	Port       string
	StorageDir string
}

func Load() Config {
	return Config{
		DBPath:     envOrDefault("INCIPIT_DB_PATH", "/data/books.db"),
		Port:       envOrDefault("INCIPIT_PORT", "8080"),
		StorageDir: envOrDefault("INCIPIT_STORAGE_DIR", "/data"),
	}
}

func envOrDefault(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}
