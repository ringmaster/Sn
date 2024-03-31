package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/alecthomas/kong"
	"github.com/ringmaster/Sn/sn"
)

var CLI struct {
	Serve struct {
	} `cmd:"serve" help:"Start the server"`
	Sql struct {
		Query string `arg:"" required:"" help:"The query to execute"`
	} `cmd:"sql" help:"Perform queries against repo data"`
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

func sql(query string) {
	fmt.Printf("Running Query: %s\n", query)
	sn.ConfigSetup()

	sn.RegisterTemplateHelpers()
	sn.RegisterPartials()

	sn.DBConnect()
	defer sn.DBClose()
	sn.DBLoadRepos()

	//rows, err := sn.DBQuery(query)

	//slug := "welcome"
	qry := sn.ItemQuery{PerPage: 5, Page: 1}

	result := sn.ItemsFromItemQuery(qry)

	fmt.Printf("Result:\n%#v\n\n", result)
}

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
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
		slog.Default().Info("started Sn serve")
		serve()
	case "sql <query>":
		sqlQuery := CLI.Sql.Query
		sql(sqlQuery)
	default:
		fmt.Println(ctx.Command())
	}

}
