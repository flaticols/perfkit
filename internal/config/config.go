package config

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Config struct {
	DataDir     string       `yaml:"data_dir"`
	Project     string       `yaml:"project"`
	Server      ServerConfig `yaml:"server"`
	DefaultTags []string     `yaml:"default_tags"`
}

type ServerConfig struct {
	Host        string `yaml:"host"`
	Port        int    `yaml:"port"`
	EnablePprof bool   `yaml:"enable_pprof"`
}

func Default() *Config {
	return &Config{
		DataDir:     ".perfkit",
		Project:     "",
		DefaultTags: []string{},
		Server: ServerConfig{
			Host: "localhost",
			Port: 8080,
		},
	}
}

func Load(configPath string) (*Config, error) {
	cfg := Default()

	// Try to detect project name from current directory
	if cwd, err := os.Getwd(); err == nil {
		cfg.Project = filepath.Base(cwd)
	}

	// If no config path specified, look for .perfkit.yaml in current directory
	if configPath == "" {
		configPath = ".perfkit.yaml"
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil // Use defaults if no config file
		}
		return nil, err
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (c *Config) DBPath() string {
	return filepath.Join(c.DataDir, "perfkit.db")
}

func (c *Config) EnsureDataDir() error {
	return os.MkdirAll(c.DataDir, 0755)
}
