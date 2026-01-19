package sn

import (
	"bytes"
	"fmt"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/go-git/go-git/v5/plumbing/transport/http"

	"github.com/c4milo/afero2billy"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/storage/memory"
	"github.com/ringmaster/Sn/sn/activitypub"
	"github.com/spf13/afero"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

var (
	Vfs           afero.Fs
	GitMemStorage *memory.Storage
	Repo          *git.Repository
)

func ConfigSetup() (afero.Fs, error) {
	var err error

	viper.SetConfigName("sn")
	viper.SetEnvPrefix("SN")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "__"))
	viper.AutomaticEnv()

	viper.SetDefault("use_ssl", true)
	viper.SetDefault("activitypub.enabled", false)
	viper.SetDefault("activitypub.branch", "activitypub-data")
	viper.SetDefault("activitypub.commit_interval_minutes", 10)
	viper.AddConfigPath("/")
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

	_, err = afero.ReadDir(Vfs, "/")
	if err != nil {
		slog.Error(fmt.Sprintf("Failed to read root directory: %s", err))
		return nil, err
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

	fmt.Printf("The passwordhash for the user test from the config is: %#q\n", viper.GetString("users.test.passwordhash"))

	return Vfs, nil
}

// InitializeActivityPub initializes the ActivityPub manager after database connection
func InitializeActivityPub() error {
	var err error
	ActivityPubManager, err = activitypub.NewManager(Vfs, db)
	if err != nil {
		slog.Error(fmt.Sprintf("Failed to initialize ActivityPub: %v", err))
		return err
	}
	return nil
}

// ForceRegenerateActivityPubKeys forces regeneration of ActivityPub keys
// This is used by the regen-keys command to recover from corrupted keys
func ForceRegenerateActivityPubKeys() error {
	if ActivityPubManager == nil || !ActivityPubManager.IsEnabled() {
		return fmt.Errorf("ActivityPub is not enabled")
	}

	return ActivityPubManager.ForceRegenerateKeys()
}

// LoadActivityPubComments loads all comments from git storage into SQLite
// This should be called at startup after both ActivityPub and database are initialized
func LoadActivityPubComments() error {
	if ActivityPubManager == nil || !ActivityPubManager.IsEnabled() {
		return nil
	}

	comments, err := ActivityPubManager.GetAllComments()
	if err != nil {
		return fmt.Errorf("failed to load comments from git storage: %w", err)
	}

	if len(comments) == 0 {
		slog.Info("No ActivityPub comments to load into SQLite")
		return nil
	}

	loaded := 0
	for _, comment := range comments {
		if err := InsertComment(comment); err != nil {
			slog.Warn("Failed to insert comment into SQLite", "comment_id", comment.ID, "error", err)
			continue
		}
		loaded++
	}

	slog.Info("Loaded ActivityPub comments into SQLite", "total", len(comments), "loaded", loaded)
	return nil
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
		slog.Info("Using basic authentication for git clone", "username", username)
		cloneOptions.Auth = &http.BasicAuth{
			Username: username, // can be anything except an empty string
			Password: password,
		}
	} else {
		slog.Info("Basic authentication for git clone is not set - write potentially disabled")
	}

	GitMemStorage = memory.NewStorage()

	// Clone the given repository to the in-memory filesystem
	var err error
	Repo, err = git.Clone(GitMemStorage, billyFs, cloneOptions)
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

// GetRepoOrder returns the repo names in the order they appear in the config file.
// This preserves the YAML key order which viper.GetStringMap() loses.
func GetRepoOrder() []string {
	configFile := viper.ConfigFileUsed()
	if configFile == "" || Vfs == nil {
		// Fallback to unordered keys
		repos := viper.GetStringMap("repos")
		result := make([]string, 0, len(repos))
		for k := range repos {
			result = append(result, k)
		}
		return result
	}

	// Read from the virtual filesystem that viper uses
	data, err := afero.ReadFile(Vfs, configFile)
	if err != nil {
		slog.Warn("Could not read config file for repo order", "error", err, "file", configFile)
		// Fallback to unordered keys
		repos := viper.GetStringMap("repos")
		result := make([]string, 0, len(repos))
		for k := range repos {
			result = append(result, k)
		}
		return result
	}

	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		slog.Warn("Could not parse config file for repo order", "error", err)
		return nil
	}

	// Navigate to find "repos" key
	if root.Kind != yaml.DocumentNode || len(root.Content) == 0 {
		return nil
	}

	mapping := root.Content[0]
	if mapping.Kind != yaml.MappingNode {
		return nil
	}

	// Find the "repos" key in the mapping
	for i := 0; i < len(mapping.Content)-1; i += 2 {
		keyNode := mapping.Content[i]
		valueNode := mapping.Content[i+1]

		if keyNode.Value == "repos" && valueNode.Kind == yaml.MappingNode {
			// Extract keys in order
			result := make([]string, 0, len(valueNode.Content)/2)
			for j := 0; j < len(valueNode.Content)-1; j += 2 {
				result = append(result, valueNode.Content[j].Value)
			}
			return result
		}
	}

	return nil
}
