package main

import (
	"log/slog"
	"os"

	"github.com/alecthomas/kong"
	"github.com/ringmaster/Sn/sn"
)

var CLI struct {
	Serve struct {
	} `cmd:"" help:"Start the server"`
	Fm struct {
	} `cmd:"" help:"Perform operations on frontmatter"`
}

func serve() {
	sn.ConfigSetup()

	sn.RegisterTemplateHelpers()
	sn.RegisterPartials()

	sn.DBConnect()
	defer sn.DBClose()
	sn.DBLoadRepos()

	sn.WebserverStart()
}

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	slog.Default().Info("started Sn")
	ctx := kong.Parse(
		&CLI,
		kong.Description("A simple web server that dynamically serves blog entries"),
		kong.UsageOnError(),
		kong.ConfigureHelp(kong.HelpOptions{
			Compact: true,
			Summary: true,
		}))
	switch ctx.Command() {
	case "serve":
		serve()
	}

}
