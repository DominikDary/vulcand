package main

import (
	"github.com/codegangsta/cli"
)

func NewUpstreamCommand() cli.Command {
	return cli.Command{
		Name:  "upstream",
		Usage: "Operations with vulcan upstreams",
		Subcommands: []cli.Command{
			{
				Name:   "add",
				Usage:  "Add a new upstream to vulcan",
				Action: addUpstreamAction,
				Flags: []cli.Flag{
					cli.StringFlag{"id", "", "upstream id"},
				},
			},
			{
				Name:   "rm",
				Usage:  "Remove upstream from vulcan",
				Action: deleteUpstreamAction,
				Flags: []cli.Flag{
					cli.StringFlag{"id", "", "upstream id"},
				},
			},
			{
				Name:   "ls",
				Usage:  "List upstreams",
				Action: listUpstreamsAction,
			},
			{
				Name:  "drain",
				Usage: "Wait till there are no more connections for endpoints in the upstream",
				Flags: []cli.Flag{
					cli.StringFlag{"id", "", "upstream id"},
					cli.IntFlag{"timeout", 5, "timeout in seconds"},
				},
				Action: upstreamDrainConnections,
			},
		},
	}
}

func addUpstreamAction(c *cli.Context) {
	u, err := client(c).AddUpstream(c.String("id"))
	printResult("%s added", u, err)
}

func deleteUpstreamAction(c *cli.Context) {
	printStatus(client(c).DeleteUpstream(c.String("id")))
}

func listUpstreamsAction(c *cli.Context) {
	out, err := client(c).GetUpstreams()
	if err != nil {
		printError(err)
	} else {
		printUpstreams(out)
	}
}

func upstreamDrainConnections(c *cli.Context) {
	connections, err := client(c).DrainUpstreamConnections(c.String("id"), c.String("timeout"))
	if err != nil {
		printError(err)
		return
	}
	if connections == 0 {
		printOk("Connections: %d", connections)
	} else {
		printInfo("Connections: %d", connections)
	}
}
