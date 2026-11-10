package configure

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/kr/pretty"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
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
	Appname    string   `json:"appname"     mapstructure:"appname"`
	Live       bool     `json:"live"        mapstructure:"live"`
	Hls        bool     `json:"hls"         mapstructure:"hls"`
	Flv        bool     `json:"flv"         mapstructure:"flv"`
	API        bool     `json:"api"         mapstructure:"api"`
	StaticPush []string `json:"static_push" mapstructure:"static_push"`
}

type Applications []Application

type JWT struct {
	Secret    string `json:"secret"    mapstructure:"secret"`
	Algorithm string `json:"algorithm" mapstructure:"algorithm"`
}
type ServerCfg struct {
	Level           string       `json:"level"              mapstructure:"level"`
	ConfigFile      string       `json:"config_file"        mapstructure:"config_file"`
	FLVArchive      bool         `json:"flv_archive"        mapstructure:"flv_archive"`
	FLVDir          string       `json:"flv_dir"            mapstructure:"flv_dir"`
	RTMPNoAuth      bool         `json:"rtmp_noauth"        mapstructure:"rtmp_noauth"`
	RTMPAddr        string       `json:"rtmp_addr"          mapstructure:"rtmp_addr"`
	HTTPFLVAddr     string       `json:"httpflv_addr"       mapstructure:"httpflv_addr"`
	HLSAddr         string       `json:"hls_addr"           mapstructure:"hls_addr"`
	HLSKeepAfterEnd bool         `json:"hls_keep_after_end" mapstructure:"hls_keep_after_end"`
	APIAddr         string       `json:"api_addr"           mapstructure:"api_addr"`
	RedisAddr       string       `json:"redis_addr"         mapstructure:"redis_addr"`
	RedisPwd        string       `json:"redis_pwd"          mapstructure:"redis_pwd"`
	ReadTimeout     int          `json:"read_timeout"       mapstructure:"read_timeout"`
	WriteTimeout    int          `json:"write_timeout"      mapstructure:"write_timeout"`
	EnableTLSVerify bool         `json:"enable_tls_verify"  mapstructure:"enable_tls_verify"`
	GopNum          int          `json:"gop_num"            mapstructure:"gop_num"`
	JWT             JWT          `json:"jwt"                mapstructure:"jwt"`
	Server          Applications `json:"server"             mapstructure:"server"`
}

// default config.
var defaultConf = ServerCfg{
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
		API:        true,
		StaticPush: nil,
	}},
}

var (
	Config = viper.New()

	// BypassInit can be used to bypass the init() function by setting this
	// value to True at compile time.
	//
	// go build -ldflags "-X 'github.com/gwuhaolin/livego/configure.BypassInit=true'" -o livego main.go.
	BypassInit string = ""
)

func initLog() {
	if l, err := log.ParseLevel(Config.GetString("level")); err == nil {
		log.SetLevel(l)
		log.SetReportCaller(l == log.DebugLevel)
	}
}

//nolint:gochecknoinits // importers rely on Config being loaded on import (e.g. the rtmp package timeouts); BypassInit opts out.
func init() {
	if BypassInit == "" {
		initDefault()
	}
}

func initDefault() {
	defer Init()

	// Default config
	b, err := json.Marshal(defaultConf)
	if err != nil {
		log.Panic("marshal default config: ", err)
	}
	defaultConfig := bytes.NewReader(b)
	viper.SetConfigType("json")
	if err := viper.ReadConfig(defaultConfig); err != nil {
		log.Panic("read default config: ", err)
	}
	if err := Config.MergeConfigMap(viper.AllSettings()); err != nil {
		log.Panic("merge default config: ", err)
	}

	// Flags
	pflag.String("rtmp_addr", ":1935", "RTMP server listen address")
	pflag.Bool("enable_rtmps", false, "enable server session RTMPS")
	pflag.String("rtmps_cert", "server.crt", "cert file path required for RTMPS")
	pflag.String("rtmps_key", "server.key", "key file path required for RTMPS")
	pflag.String("httpflv_addr", ":7001", "HTTP-FLV server listen address")
	pflag.String("hls_addr", ":7002", "HLS server listen address")
	pflag.String("api_addr", ":8090", "HTTP manage interface server listen address")
	pflag.String("config_file", "livego.yaml", "configure filename")
	pflag.String("level", "info", "Log level")
	pflag.Bool("hls_keep_after_end", false, "Maintains the HLS after the stream ends")
	pflag.String("flv_dir", "tmp", "output flv file at flvDir/APP/KEY_TIME.flv")
	pflag.Int("read_timeout", 10, "read time out")
	pflag.Int("write_timeout", 10, "write time out")
	pflag.Int("gop_num", 1, "gop num")
	pflag.Bool("enable_tls_verify", true, "Use system root CA to verify RTMPS connection, set this flag to false on Windows")
	pflag.Parse()
	if err := Config.BindPFlags(pflag.CommandLine); err != nil {
		log.Warning("bind flags: ", err)
	}

	// File
	Config.SetConfigFile(Config.GetString("config_file"))
	Config.AddConfigPath(".")
	if err := Config.ReadInConfig(); err != nil {
		log.Warning(err)
		log.Info("Using default config")
	} else if err := Config.MergeInConfig(); err != nil {
		log.Warning("merge config file: ", err)
	}

	// Environment
	replacer := strings.NewReplacer(".", "_")
	Config.SetEnvKeyReplacer(replacer)
	Config.AllowEmptyEnv(true)
	Config.AutomaticEnv()

	// Log
	initLog()

	// Print final config
	c := ServerCfg{}
	if err := Config.Unmarshal(&c); err != nil {
		log.Warning("unmarshal config: ", err)
	}
	log.Debugf("Current configurations: \n%# v", pretty.Formatter(c))
}

func CheckAppName(appname string) bool {
	apps := Applications{}
	if err := Config.UnmarshalKey("server", &apps); err != nil {
		log.Warning("unmarshal server config: ", err)
		return false
	}
	for _, app := range apps {
		if app.Appname == appname {
			return app.Live
		}
	}
	return false
}

func GetStaticPushURLList(appname string) ([]string, bool) {
	apps := Applications{}
	if err := Config.UnmarshalKey("server", &apps); err != nil {
		log.Warning("unmarshal server config: ", err)
		return nil, false
	}
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
