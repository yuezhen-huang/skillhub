package config

import (
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

// Config holds the application configuration
type Config struct {
	Hub      HubConfig      `mapstructure:"hub"`
	Storage  StorageConfig  `mapstructure:"storage"`
	Skill    SkillConfig    `mapstructure:"skill"`
	Log      LogConfig      `mapstructure:"log"`
}

// HubConfig holds hub-specific configuration
type HubConfig struct {
	GRPCAddr string `mapstructure:"grpc_addr"`
	HTTPAddr string `mapstructure:"http_addr"`
}

// StorageConfig holds storage configuration
type StorageConfig struct {
	Type string `mapstructure:"type"`
	Path string `mapstructure:"path"`
}

// SkillConfig holds skill runtime configuration
type SkillConfig struct {
	SkillsDir    string `mapstructure:"skills_dir"`
	AgentDirs    []string `mapstructure:"agent_dirs"`
	PortStart    int    `mapstructure:"port_start"`
	PortEnd      int    `mapstructure:"port_end"`
	BuildTimeout int    `mapstructure:"build_timeout"`
}

// LogConfig holds logging configuration
type LogConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
}

// Load loads the configuration from file or environment
func Load(configPath string) (*Config, error) {
	v := viper.New()

	// Set defaults
	setDefaults(v)

	if configPath != "" {
		v.SetConfigFile(configPath)
	} else {
		// Look for config in standard locations
		v.SetConfigName("config")
		v.SetConfigType("yaml")
		v.AddConfigPath(".")
		v.AddConfigPath("/etc/skillhub")
		v.AddConfigPath("$HOME/.skillhub")
	}

	v.AutomaticEnv()
	v.SetEnvPrefix("SKILLHUB")

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, err
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// Default returns a default configuration
func Default() *Config {
	homeDir, _ := os.UserHomeDir()
	return &Config{
		Hub: HubConfig{
			GRPCAddr: ":50051",
			HTTPAddr: ":8080",
		},
		Storage: StorageConfig{
			Type: "sqlite",
			Path: filepath.Join(homeDir, ".skillhub", "skillhub.db"),
		},
		Skill: SkillConfig{
			SkillsDir:    filepath.Join(homeDir, ".skillhub", "skills"),
			AgentDirs:    nil,
			PortStart:    51000,
			PortEnd:      52000,
			BuildTimeout: 300,
		},
		Log: LogConfig{
			Level:  "info",
			Format: "text",
		},
	}
}

func setDefaults(v *viper.Viper) {
	homeDir, _ := os.UserHomeDir()

	v.SetDefault("hub.grpc_addr", ":50051")
	v.SetDefault("hub.http_addr", ":8080")
	v.SetDefault("storage.type", "sqlite")
	v.SetDefault("storage.path", filepath.Join(homeDir, ".skillhub", "skillhub.db"))
	v.SetDefault("skill.skills_dir", filepath.Join(homeDir, ".skillhub", "skills"))
	v.SetDefault("skill.agent_dirs", []string{})
	v.SetDefault("skill.port_start", 51000)
	v.SetDefault("skill.port_end", 52000)
	v.SetDefault("skill.build_timeout", 300)
	v.SetDefault("log.level", "info")
	v.SetDefault("log.format", "text")
}
