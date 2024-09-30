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

var Vfs afero.Fs

func ConfigSetup() (afero.Fs, error) {
	var err error

	viper.SetConfigName("sn")
	viper.SetEnvPrefix("SN")
	viper.AutomaticEnv()

	viper.SetDefault("use_ssl", true)
	//viper.AddConfigPath("/")
	viper.AddConfigPath("")

	if snGitRepo := os.Getenv("SN_GIT_REPO"); snGitRepo != "" {
		Vfs, err = CloneRepoToVFS(snGitRepo)
		if err != nil {
			slog.Error(fmt.Sprintf("Error while cloning git repo: %v", err))
			return nil, err
		}
		var snConfigFile string
		if snConfigFile = os.Getenv("SN_CONFIG"); snConfigFile == "" {
			snConfigFile = "sn.yaml"
		}
		viper.SetConfigFile(snConfigFile)
	} else if snConfigFile := os.Getenv("SN_CONFIG"); snConfigFile != "" {
		snConfigFile, _ := filepath.Abs(snConfigFile)
		slog.Info(fmt.Sprintf("Config file was specified in environment: %s", snConfigFile))
		configFileDir := filepath.Dir(snConfigFile)
		slog.Info(fmt.Sprintf("Rooting virtual file system here: %s", configFileDir))
		Vfs = afero.NewBasePathFs(afero.NewOsFs(), configFileDir)
		viper.SetConfigFile(filepath.Base(snConfigFile))
	} else {
		currentDir, err := os.Getwd()
		if err != nil {
			slog.Error(fmt.Sprintf("Error getting current directory: %v", err))
			return nil, err
		}
		slog.Info(fmt.Sprintf("Rooting virtual file system here: %s", currentDir))
		Vfs = afero.NewBasePathFs(afero.NewOsFs(), currentDir)
	}
	// Set the virtual filesystem as the source for viper config
	viper.SetFs(Vfs)

	files, err := afero.ReadDir(Vfs, "/")
	if err != nil {
		slog.Error(fmt.Sprintf("Failed to read root directory: %s", err))
		return nil, err
	}

	for _, file := range files {
		fmt.Println(file.Name())
	}

	viper.WatchConfig()
	if err := viper.ReadInConfig(); err != nil {
		// Output the files in the root of the virtual filesystem
		files, err2 := afero.ReadDir(Vfs, "/")
		if err2 != nil {
			slog.Error(fmt.Sprintf("Failed to read root directory of virtual filesystem: %s", err))
			return nil, err2
		}

		for _, file := range files {
			slog.Info(file.Name())
		}

		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			slog.Error("Could not find configuration file")
		} else {
			slog.Error(fmt.Sprintf("Error while loading configuration file %#q", err))
		}
		panic(err)
	}
	viper.SetDefault("path", filepath.Dir(viper.ConfigFileUsed()))

	return Vfs, nil
}

func CloneRepoToVFS(snGitRepo string) (afero.Fs, error) {
	slog.Info("Cloning repository to virtual filesystem")

	// Create an in-memory filesystem
	fs := afero.NewMemMapFs()

	billyFs := afero2billy.New(fs)

	// Retrieve username and password from environment variables
	username := os.Getenv("SN_GIT_USERNAME")
	password := os.Getenv("SN_GIT_PASSWORD")

	// Set up clone options with or without authentication
	cloneOptions := &git.CloneOptions{
		URL:    snGitRepo,
		Mirror: true,
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

	// List files in the root of the in-memory filesystem
	/*
		files, err := afero.ReadDir(fs, "/")
		if err != nil {
			slog.Error(fmt.Sprintf("Failed to read root directory: %s", err))
			return nil, err
		}

		for _, file := range files {
			fmt.Println(file.Name())
		}
	*/

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

// ConfigPath returns the absolute path defined for a configuration key
func ConfigPath(configKey string, opts ...ConfigPathOptionFn) string {
	options := &ConfigPathOptions{
		HasDefault: false,
		Default:    "",
		MustExist:  true,
	}

	for _, applyOpt := range opts {
		applyOpt(options)
	}

	if !viper.IsSet(configKey) {
		if options.HasDefault {
			return options.Default
		} else {
			panic(fmt.Sprintf("Required config value for %s is not set in settings yaml", configKey))
		}
	}

	longpath := viper.GetString(configKey)

	// Allow any values from the config to replace named variables in the path obtained from the config
	configVars := viper.AllSettings()
	pathTemplate := template.Must(template.New("").Parse(longpath))
	buf := bytes.Buffer{}
	pathTemplate.Execute(&buf, configVars)
	var renderedPathTemplate string = buf.String()

	if DirExistsFs(Vfs, renderedPathTemplate) {
		return renderedPathTemplate
	}

	base := viper.GetString("path")

	base = path.Join(base, renderedPathTemplate)
	if options.MustExist && !DirExistsFs(Vfs, base) {
		panic(fmt.Sprintf("Configpath for %s does not exist at %s", renderedPathTemplate, base))
	}
	return base
}

func DirExistsFs(fs afero.Fs, path string) bool {
	info, err := fs.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}
