package main

import (
	"fmt"
	"log/slog"
	"os"
	"syscall"

	"github.com/alecthomas/kong"
	"github.com/joho/godotenv"
	"github.com/olekukonko/tablewriter"
	"github.com/ringmaster/Sn/sn"
	"github.com/spf13/viper"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/crypto/ssh/terminal"
)

var CLI struct {
	Serve struct {
	} `cmd:"serve" help:"Start the server"`
	Sql struct {
		Query string `arg:"" required:"" help:"The query to execute"`
	} `cmd:"sql" help:"Perform queries against repo data"`
	Passwd struct {
		Username string `arg:"" required:"" help:"The user"`
		Password string `arg:"" optional:"" help:"The password to set"`
	} `cmd:"passwd" help:"Create or update a user password"`
	RegenKeys struct {
	} `cmd:"regen-keys" help:"Regenerate ActivityPub keys (removes existing encrypted keys)"`
}

func serve() {
	_, err := sn.ConfigSetup()

	if err == nil {
		sn.RegisterTemplateHelpers()
		sn.RegisterPartials()

		sn.DBConnect()
		defer sn.DBClose()

		// Initialize ActivityPub after database connection
		err = sn.InitializeActivityPub()
		if err != nil {
			slog.Error(fmt.Sprintf("Error while initializing ActivityPub: %v", err))
			return
		}

		// Set up ActivityPub cleanup
		defer func() {
			if sn.ActivityPubManager != nil {
				if err := sn.ActivityPubManager.Close(); err != nil {
					slog.Error(fmt.Sprintf("Error cleaning up ActivityPub: %v", err))
				}
			}
		}()

		sn.DBLoadRepos()

		sn.WebserverStart()
	} else {
		slog.Error(fmt.Sprintf("Error while setting up config: %v", err))
	}
}

func sql(query string) {
	fmt.Printf("Connecting to Database\n")
	sn.ConfigSetup()

	sn.RegisterTemplateHelpers()
	sn.RegisterPartials()

	sn.DBConnect()
	defer sn.DBClose()
	sn.DBLoadReposSync()
	fmt.Printf("Running Query: %s\n", query)

	rows, _ := sn.DBQuery(query)
	fmt.Printf("Query Complete\n")

	cols, _ := rows.Columns()

	result, _ := sn.RowToMapSlice(rows)

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader(cols)

	for _, v := range result {
		table.Append(v)
	}
	table.Render() // Send output

}

func passwd(username string, passwords ...string) {
	var password string
	if len(passwords) == 0 {
		fmt.Printf("Please enter password for user %s: ", username)
		bytePassword, err := terminal.ReadPassword(int(syscall.Stdin))
		if err != nil {
			slog.Error(fmt.Sprintf("Error reading password: %v", err))
			return
		}
		password = string(bytePassword)
		fmt.Println()
	}
	if password == "" {
		slog.Error("No password provided")
		return
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		slog.Error(fmt.Sprintf("Error hashing password: %v", err))
		return
	}

	_, err = sn.ConfigSetup()
	if err != nil {
		slog.Error(fmt.Sprintf("Error loading config: %v", err))
		return
	}

	viper.Set("users."+username+".passwordhash", string(hashedPassword))
	viper.WriteConfig()

	slog.Info(fmt.Sprintf("Password for user %s has been set successfully", username))
}

func regenKeys() {
	slog.Info("Regenerating ActivityPub keys...")

	// Set up config and initialize ActivityPub to access the git repository
	_, err := sn.ConfigSetup()
	if err != nil {
		slog.Error(fmt.Sprintf("Error while setting up config: %v", err))
		return
	}

	// Check if ActivityPub is enabled
	if sn.ActivityPubManager == nil || !sn.ActivityPubManager.IsEnabled() {
		slog.Error("ActivityPub is not enabled - cannot regenerate keys")
		return
	}

	// In git mode, we need to force deletion and regeneration through the storage layer
	slog.Info("Removing corrupted ActivityPub keys...")

	// Force regeneration by calling the internal method
	err = sn.ForceRegenerateActivityPubKeys()
	if err != nil {
		slog.Error(fmt.Sprintf("Error regenerating ActivityPub keys: %v", err))
		return
	}

	slog.Info("ActivityPub keys regenerated successfully")
	slog.Info("Note: Existing followers will need to re-follow your accounts")
}

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	err := godotenv.Load()
	if err != nil {
		slog.Info("Not using .env file")
	}
	ctx := kong.Parse(
		&CLI,
		kong.Description("A simple web server that dynamically serves blog entries"),
		kong.UsageOnError(),
		kong.ConfigureHelp(kong.HelpOptions{
			Compact: true,
			Summary: true,
		}))
	switch ctx.Command() {
	case "passwd <username> <password>":
		slog.Default().Info("setting password")
		passwd(CLI.Passwd.Username, CLI.Passwd.Password)
	case "passwd <username>":
		slog.Default().Info("started Sn serve")
		passwd(CLI.Passwd.Username)
	case "serve":
		slog.Default().Info("started Sn serve")
		serve()
	case "sql <query>":
		sqlQuery := CLI.Sql.Query
		sql(sqlQuery)
	case "regen-keys":
		slog.Default().Info("regenerating ActivityPub keys")
		regenKeys()
	default:
		fmt.Println(ctx.Command())
	}

}
