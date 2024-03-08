package sn

import (
	"bytes"
	"fmt"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"text/template"

	"github.com/spf13/viper"
)

func ConfigSetup() {
	viper.SetConfigName("sn")
	viper.AddConfigPath(".")
	if snConfigFile := os.Getenv("SN_CONFIG"); snConfigFile != "" {
		snConfigFile, _ := filepath.Abs(snConfigFile)
		viper.SetConfigFile(snConfigFile)
	}

	viper.WatchConfig()
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			slog.Error("Could not find configuration file")
		} else {
			slog.Error(fmt.Sprintf("Error while loading configuration file %#q", err))
		}
	}
	viper.SetDefault("path", filepath.Dir(viper.ConfigFileUsed()))
}

func ConfigStringDefault(configLocation string, defaultVal string) string {
	if viper.IsSet(configLocation) {
		return viper.GetString(configLocation)
	} else {
		return defaultVal
	}
}

type ConfigPathOptions struct {
	HasDefault bool
	Default    string
	MustExist  bool
}

type ConfigPathOptionFn func(f *ConfigPathOptions)

func WithDefault(def string) ConfigPathOptionFn {
	return func(f *ConfigPathOptions) {
		f.HasDefault = true
		f.Default = def
	}
}

func MustExist() ConfigPathOptionFn {
	return func(f *ConfigPathOptions) {
		f.MustExist = true
	}
}

func OptionallyExist() ConfigPathOptionFn {
	return func(f *ConfigPathOptions) {
		f.MustExist = false
	}
}

func ConfigPath(shortpath string, opts ...ConfigPathOptionFn) string {
	options := &ConfigPathOptions{
		HasDefault: false,
		Default:    "",
		MustExist:  true,
	}

	for _, applyOpt := range opts {
		applyOpt(options)
	}

	if !viper.IsSet(shortpath) {
		if options.HasDefault {
			return options.Default
		} else {
			panic(fmt.Sprintf("Required config value for %s is not set in settings yaml", shortpath))
		}
	}

	longpath := viper.GetString(shortpath)

	configVars := viper.AllSettings()

	pathTemplate := template.Must(template.New("").Parse(longpath))
	buf := bytes.Buffer{}
	pathTemplate.Execute(&buf, configVars)
	var renderedPathTemplate string = buf.String()

	if path.IsAbs(renderedPathTemplate) && DirExists(renderedPathTemplate) {
		return renderedPathTemplate
	}

	base, err := filepath.Abs(viper.GetString("path"))
	if err != nil {
		panic(fmt.Sprintf("There is no absolute path at %s", viper.GetString("path")))
	}

	base = path.Join(base, renderedPathTemplate)
	if options.MustExist && !DirExists(base) {
		panic(fmt.Sprintf("Configpath for %s does not exist at %s", renderedPathTemplate, base))
	}
	return base
}
