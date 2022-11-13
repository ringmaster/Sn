package sn

import (
	"bytes"
	"fmt"
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
		fmt.Printf("Loading configuration file: %s\n", snConfigFile)
		viper.SetConfigFile(snConfigFile)
	}

	viper.WatchConfig()
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			fmt.Println("Could not find configuration file")
		} else {
			fmt.Println("Error while loading configuration file")
			fmt.Printf("%q", err)
		}
	}
	viper.SetDefault("path", filepath.Dir(viper.ConfigFileUsed()))
	fmt.Printf("Used configuration file: %s\n", viper.ConfigFileUsed())
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
	fmt.Printf("Rendered path template: %#q\n", renderedPathTemplate)

	if renderedPathTemplate[0] == '/' && DirExists(renderedPathTemplate) {
		return renderedPathTemplate
	}

	base, err := filepath.Abs(viper.GetString("path"))
	if err != nil {
		panic(fmt.Sprintf("Configpath for %s does not have absolute path at %s", renderedPathTemplate, viper.GetString("path")))
	}

	fmt.Printf("configPath: %s %s\n", base, renderedPathTemplate)
	base = path.Join(base, renderedPathTemplate)
	if options.MustExist && !DirExists(base) {
		panic(fmt.Sprintf("Configpath for %s does not exist at %s", renderedPathTemplate, base))
	}
	return base
}
