package config

import (
	"bytes"
	_ "embed"
	"strings"

	"github.com/samber/do/v2"
	"github.com/spf13/viper"
)

type Config struct {
	BotToken        string `mapstructure:"botToken"`
	AccountCategory string `mapstructure:"accountCategory"`
	DBPath          string `mapstructure:"dbPath"`
}

var Package = do.Package(
	do.Lazy(NewConfig),
)

//go:embed config.yaml
var defaultConfig []byte

func NewConfig(i do.Injector) (*Config, error) {
	c := &Config{}

	viper.SetConfigType("yaml")
	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	if err := viper.ReadConfig(bytes.NewBuffer(defaultConfig)); err != nil {
		return nil, err
	}

	viper.SetConfigName("config")
	viper.AddConfigPath(".")
	_ = viper.MergeInConfig()

	_ = viper.BindEnv("botToken", "BOT_TOKEN")
	_ = viper.BindEnv("accountCategory", "ACCOUNT_CATEGORY")
	_ = viper.BindEnv("dbPath", "DB_PATH")

	if err := viper.Unmarshal(c); err != nil {
		return nil, err
	}

	return c, nil
}
