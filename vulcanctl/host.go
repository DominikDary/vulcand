package main

import (
	"github.com/codegangsta/cli"
)

func NewHostCommand() cli.Command {
	return cli.Command{
		Name:  "host",
		Usage: "Operations with vulcan hosts",
		Subcommands: []cli.Command{
			{
				Name: "add",
				Flags: []cli.Flag{
					cli.StringFlag{"name", "", "hostname"},
				},
				Usage:  "Add a new host to vulcan proxy",
				Action: addHostAction,
			},
			{
				Name: "rm",
				Flags: []cli.Flag{
					cli.StringFlag{"name", "", "hostname"},
				},
				Usage:  "Remove a host from vulcan",
				Action: deleteHostAction,
			},
		},
	}
}

func addHostAction(c *cli.Context) {
	host, err := client(c).AddHost(c.String("name"))
	printResult("%s added", host, err)
}

func deleteHostAction(c *cli.Context) {
	printStatus(client(c).DeleteHost(c.String("name")))
}
