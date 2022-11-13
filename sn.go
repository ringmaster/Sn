package main

import (
	"github.com/ringmaster/Sn/sn"
)

func main() {
	sn.ConfigSetup()

	sn.RegisterTemplateHelpers()

	sn.DBConnect()
	defer sn.DBClose()
	sn.DBLoadRepos()

	sn.WebserverStart()
}
