package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

// Config mappings for structural unmarshaling
type Config struct {
	Name      string `yaml:"name"`
	Syncthing struct {
		URL    string `yaml:"url"`
		APIKey string `yaml:"api_key"`
	} `yaml:"syncthing"`

	Smart struct {
		Drives []string `yaml:"drives"`
	} `yaml:"smart"`

	Disk struct {
		Mounts []string `yaml:"mounts"`
	} `yaml:"disk"`
}

// Global configuration instance accessible across packages
var AppConfig *Config

// LoadConfig reads and parses the target YAML file path into memory
func LoadConfig(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	var cfg Config
	decoder := yaml.NewDecoder(file)
	if err := decoder.Decode(&cfg); err != nil {
		return err
	}

	AppConfig = &cfg
	return nil
}
