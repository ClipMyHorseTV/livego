package configure

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/kr/pretty"
	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
)

type Applications []Application

type Application struct {
	Appname    string   `mapstructure:"appname"`
	Live       bool     `mapstructure:"live"`
	Hls        bool     `mapstructure:"hls"`
	Flv        bool     `mapstructure:"flv"`
	Api        bool     `mapstructure:"api"`
	StaticPush []string `mapstructure:"static_push"`
}

func (a *Application) Validate() error {
	if a.Appname == "" {
		return fmt.Errorf("appname cannot be empty")
	}
	return nil
}

func DefaultApplication() Application {
	return Application{
		Appname:    "live",
		Live:       true,
		Hls:        true,
		Flv:        true,
		Api:        true,
		StaticPush: nil,
	}
}

type JWT struct {
	Secret    string `mapstructure:"secret"`
	Algorithm string `mapstructure:"algorithm"`
}

type RTMPSConfig struct {
	CertFile        string `mapstructure:"cert_file"`
	KeyFile         string `mapstructure:"key_file"`
	EnableTLSVerify bool   `mapstructure:"enable_tls_verify"`
}

type RTMPConfig struct {
	Address string `mapstructure:"address"`

	NoAuth bool `mapstructure:"no_auth"`

	FLVArchive bool   `mapstructure:"flv_archive"`
	FLVDir     string `mapstructure:"flv_dir"`

	ReadTimeout  time.Duration `mapstructure:"read_timeout"`
	WriteTimeout time.Duration `mapstructure:"write_timeout"`

	RTMPSConfig *RTMPSConfig `mapstructure:"rtmps"`
}

func (c *RTMPConfig) Validate() error {
	if c.Address == "" {
		return fmt.Errorf("rtmp address cannot be empty")
	}

	if c.ReadTimeout <= 0 || c.ReadTimeout > 30*time.Second {
		return fmt.Errorf("rtmp read_timeout must be between 1s and 30s. Value found: '%s'", c.ReadTimeout)
	}
	if c.WriteTimeout <= 0 || c.WriteTimeout > 30*time.Second {
		return fmt.Errorf("rtmp write_timeout must be between 1s and 30s. Value found: '%s'", c.WriteTimeout)
	}

	if c.RTMPSConfig != nil {
		if c.RTMPSConfig.CertFile == "" || c.RTMPSConfig.KeyFile == "" {
			return fmt.Errorf("both cert_file and key_file must be provided for RTMPS if RTMPS is defined in the config")
		}
	}

	return nil
}

func DefaultRTMPConfig() RTMPConfig {
	return RTMPConfig{
		Address:      ":1935",
		NoAuth:       false,
		FLVArchive:   false,
		FLVDir:       "flv",
		ReadTimeout:  time.Second * 10,
		WriteTimeout: time.Second * 10,
		RTMPSConfig:  nil,
	}
}

type HLSConfig struct {
	Address     string `mapstructure:"address"`
	HTTPFLVAddr string `mapstructure:"httpflv_addr"` // address to server the HTTP-FLV stream, e.g. ":7001"

	KeepAfterEnd    bool `mapstructure:"keep_after_end"`
	SegmentDuration int  `mapstructure:"segment_duration"`
	EnableTLSVerify bool `mapstructure:"enable_tls_verify"`

	ReadTimeout  time.Duration `mapstructure:"read_timeout"`
	WriteTimeout time.Duration `mapstructure:"write_timeout"`
}

func DefaultHLSConfig() HLSConfig {
	return HLSConfig{
		Address:         ":7002",
		HTTPFLVAddr:     ":7001",
		KeepAfterEnd:    false,
		SegmentDuration: 5,
		EnableTLSVerify: true,
		ReadTimeout:     time.Second * 10,
		WriteTimeout:    time.Second * 10,
	}
}

type RedisConfig struct {
	Enabled bool   `mapstructure:"enabled"`
	Addr    string `mapstructure:"addr"`
	Pwd     string `mapstructure:"pwd"`
}

func DefaultRedisConfig() RedisConfig {
	return RedisConfig{
		Enabled: false,
		Addr:    "localhost:6379",
		Pwd:     "",
	}
}

type Config struct {
	Level        string `mapstructure:"level"`
	Path         string `mapstructure:"config_file"`
	RunAsLibrary bool   `mapstructure:"run_as_library"`

	RTMP  RTMPConfig  `mapstructure:"rtmp"`
	HLS   HLSConfig   `mapstructure:"hls"`
	Redis RedisConfig `mapstructure:"redis"`

	APIAddr string `mapstructure:"api_addr"`

	GopNum int `mapstructure:"gop_num"`

	Server Applications `mapstructure:"server"`

	JWT *JWT `mapstructure:"jwt"`
}

func (c *Config) Validate() error {
	if c.RunAsLibrary {
		return nil // Skip validation when running as library
	}

	if c.APIAddr == "" {
		return fmt.Errorf("api_addr cannot be empty")
	}

	return nil
}

func DefaultConfig() *Config {
	return &Config{
		Level: "info",
		Path:  "config.yaml",

		RTMP:  DefaultRTMPConfig(),
		HLS:   DefaultHLSConfig(),
		Redis: DefaultRedisConfig(),

		APIAddr: ":8080",

		GopNum: 10,

		Server: Applications{DefaultApplication()},
	}

}

func LoadConfig(configPath string) (*Config, error) {
	cfg := DefaultConfig()

	// If no config path specified, use default
	if configPath == "" {
		configPath = cfg.Path
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			log.Infof("Config file '%s' not found, using defaults", configPath)
			return cfg, nil
		}
		return nil, err
	}

	fileExt := configPath[len(configPath)-5:]

	if fileExt == ".json" {
		if err := json.Unmarshal(data, cfg); err != nil {
			return nil, err
		}
	} else if fileExt == ".yaml" || fileExt == ".yml" {
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, err
		}
	} else {
		// Try YAML first, then JSON
		if err := yaml.Unmarshal(data, cfg); err != nil {
			if err := json.Unmarshal(data, cfg); err != nil {
				return nil, err
			}
		}
	}

	if cfg.RunAsLibrary {
		log.Info("Running as library, skipping config validation")
		return cfg, nil
	}

	if cfg.Server != nil {
		for i, app := range cfg.Server {
			if err := app.Validate(); err != nil {
				return nil, fmt.Errorf("validation failed for server[%d]: %v", i, err)
			}
		}
	}

	return cfg, nil
}

// InitConfig initializes the global config
func InitConfig(configPath string) (*Config, error) {
	cfg, err := LoadConfig(configPath)
	if err != nil {
		return nil, err
	}

	// Set log level
	if l, err := log.ParseLevel(cfg.Level); err == nil {
		log.SetLevel(l)
		log.SetReportCaller(l == log.DebugLevel)
	}

	// Print final config
	log.Debugf("Current configurations: \n%# v", pretty.Formatter(cfg))

	return cfg, nil
}

func (cfg *Config) CheckAppName(appname string) bool {
	apps := cfg.Server
	for _, app := range apps {
		if app.Appname == appname {
			return app.Live
		}
	}
	return false
}

func (cfg *Config) GetStaticPushUrlList(appname string) ([]string, bool) {
	apps := cfg.Server
	for _, app := range apps {
		if (app.Appname == appname) && app.Live {
			if len(app.StaticPush) > 0 {
				return app.StaticPush, true
			} else {
				return nil, false
			}
		}
	}
	return nil, false
}
