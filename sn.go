package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/alecthomas/kong"
	"github.com/joho/godotenv"
	"github.com/olekukonko/tablewriter"
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
	_, err := sn.ConfigSetup()

	if err == nil {
		sn.RegisterTemplateHelpers()
		sn.RegisterPartials()

		sn.DBConnect()
		defer sn.DBClose()
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

func main() {
	err := godotenv.Load()
	if err != nil {
		fmt.Printf("Error loading .env file")
	}
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
