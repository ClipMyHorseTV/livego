package rtmprelay

// Config configures static pushes.
type Config struct {
	StaticPush map[string][]string // StaticPush maps an application name to the base URLs its streams are relayed to
}

// StaticPushURLs returns the base URLs streams of appname are relayed to.
func (c *Config) StaticPushURLs(appname string) []string {
	return c.StaticPush[appname]
}
