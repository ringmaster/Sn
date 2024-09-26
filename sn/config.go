package sn

import (
	"bytes"
	"fmt"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"text/template"

	"github.com/go-git/go-git/v5/plumbing/transport/http"

	"github.com/c4milo/afero2billy"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/storage/memory"
	"github.com/spf13/afero"
	"github.com/spf13/viper"
)

func ConfigSetup() {
	if snGitRepo := os.Getenv("SN_GIT_REPO"); snGitRepo != "" {
		// Assuming you have a function `CloneRepoToVFS` that clones the repo to a virtual filesystem
		vfs, err := CloneRepoToVFS(snGitRepo)
		if err != nil {
			slog.Error(fmt.Sprintf("Error while cloning git repo: %v", err))
			return
		}
		// Set the virtual filesystem as the source for viper config
		viper.SetFs(vfs)
	}

	viper.SetConfigName("sn")
	viper.AddConfigPath(".")
	viper.SetEnvPrefix("SN")
	viper.AutomaticEnv()
	viper.SetDefault("use_ssl", true)
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

func CloneRepoToVFS(snGitRepo string) (afero.Fs, error) {
	// Create an in-memory filesystem
	fs := afero.NewMemMapFs()

	billyFs := afero2billy.New(fs)

	// Retrieve username and password from environment variables
	username := os.Getenv("SN_GIT_USERNAME")
	password := os.Getenv("SN_GIT_PASSWORD")

	// Set up clone options with or without authentication
	cloneOptions := &git.CloneOptions{
		URL: snGitRepo,
	}

	if username != "" && password != "" {
		cloneOptions.Auth = &http.BasicAuth{
			Username: username, // can be anything except an empty string
			Password: password,
		}
	}

	// Clone the given repository to the in-memory filesystem
	_, err := git.Clone(memory.NewStorage(), billyFs, cloneOptions)
	if err != nil {
		slog.Error(fmt.Sprintf("Failed to clone repository: %s", err))
		return nil, err
	}

	slog.Info("Repository cloned successfully to virtual filesystem")
	return fs, nil
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
