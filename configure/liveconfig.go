package configure

import (
	"encoding/json"
	"os"

	"github.com/kr/pretty"
	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
)

/*
{
  "server": [
    {
      "appname": "live",
      "live": true,
	  "hls": true,
	  "static_push": []
    }
  ]
}
*/

type Application struct {
	Appname    string   `mapstructure:"appname"`
	Live       bool     `mapstructure:"live"`
	Hls        bool     `mapstructure:"hls"`
	Flv        bool     `mapstructure:"flv"`
	Api        bool     `mapstructure:"api"`
	StaticPush []string `mapstructure:"static_push"`
}

type Applications []Application

type JWT struct {
	Secret    string `mapstructure:"secret"`
	Algorithm string `mapstructure:"algorithm"`
}
type Config struct {
	Level      string `mapstructure:"level"`
	ConfigFile string `mapstructure:"config_file"`

	FLVArchive bool   `mapstructure:"flv_archive"`
	FLVDir     string `mapstructure:"flv_dir"`
	RTMPNoAuth bool   `mapstructure:"rtmp_noauth"`
	RTMPAddr   string `mapstructure:"rtmp_addr"`
	RTMPSCert  string `mapstructure:"rtmps_cert"`
	RTMPSKey   string `mapstructure:"rtmps_key"`
	IsRTMPS    bool   `mapstructure:"enable_rtmps"`

	HTTPFLVAddr     string `mapstructure:"httpflv_addr"`
	HLSAddr         string `mapstructure:"hls_addr"`
	HLSKeepAfterEnd bool   `mapstructure:"hls_keep_after_end"`

	APIAddr string `mapstructure:"api_addr"`

	RedisAddr   string `mapstructure:"redis_addr"`
	RedisPwd    string `mapstructure:"redis_pwd"`
	ReadTimeout int    `mapstructure:"read_timeout"`

	WriteTimeout    int  `mapstructure:"write_timeout"`
	EnableTLSVerify bool `mapstructure:"enable_tls_verify"`
	GopNum          int  `mapstructure:"gop_num"`

	JWT JWT `mapstructure:"jwt"`

	Server Applications `mapstructure:"server"`
}

func defaultConfig() *Config {
	return &Config{
		ConfigFile:      "livego.yaml",
		FLVArchive:      false,
		RTMPNoAuth:      false,
		RTMPAddr:        ":1935",
		HTTPFLVAddr:     ":7001",
		HLSAddr:         ":7002",
		HLSKeepAfterEnd: false,
		APIAddr:         ":8090",
		WriteTimeout:    10,
		ReadTimeout:     10,
		EnableTLSVerify: true,
		GopNum:          1,
		Server: Applications{{
			Appname:    "live",
			Live:       true,
			Hls:        true,
			Flv:        true,
			Api:        true,
			StaticPush: nil,
		}},
	}
}

func LoadConfig(configPath string) (*Config, error) {
	cfg := defaultConfig()

	// If no config path specified, use default
	if configPath == "" {
		configPath = cfg.ConfigFile
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
