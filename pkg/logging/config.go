package logging

var (
	DefaultConfig = Config{
		Console:    true,
		Level:      "INFO",
		Source:     false,
		FileDir:    "./logs",
		FileName:   "voce.log",
		MaxSize:    50,
		MaxBackups: 5,
		MaxAge:     10,
		Compress:   false,
	}
)

type Config struct {
	Console    bool   `json:"console" yaml:"console" mapstructure:"console"`
	Level      string `json:"level" yaml:"level" mapstructure:"level"`
	Source     bool   `json:"source" yaml:"source" mapstructure:"source"`
	FileDir    string `json:"file_dir" yaml:"file_dir" mapstructure:"file_dir"`
	FileName   string `json:"file_name" yaml:"file_name" mapstructure:"file_name"`
	MaxSize    int    `json:"max_size" yaml:"max_size" mapstructure:"max_size"`
	MaxBackups int    `json:"max_backups" yaml:"max_backups" mapstructure:"max_backups"`
	MaxAge     int    `json:"max_age" yaml:"max_age" mapstructure:"max_age"`
	Compress   bool   `json:"compress" yaml:"compress" mapstructure:"compress"`
}

func (c *Config) SetDefaults() {
	if _, ok := loggerLevelMap[c.Level]; !ok {
		c.Level = DefaultConfig.Level
	}
	if c.MaxSize <= 0 {
		c.MaxSize = DefaultConfig.MaxSize
	}
	if c.MaxBackups <= 0 {
		c.MaxBackups = DefaultConfig.MaxBackups
	}
	if c.MaxAge <= 0 {
		c.MaxAge = DefaultConfig.MaxAge
	}
	if c.FileDir == "" {
		c.FileDir = DefaultConfig.FileDir
	}
	if c.FileName == "" {
		c.FileName = DefaultConfig.FileName
	}
}
