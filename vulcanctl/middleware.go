package main

import (
	"fmt"
	"github.com/codegangsta/cli"
	. "github.com/mailgun/vulcand/backend"
	. "github.com/mailgun/vulcand/plugin"
	"github.com/mailgun/vulcand/plugin/registry"
)

func NewMiddlewareCommands() []cli.Command {
	out := []cli.Command{}
	for _, spec := range registry.GetRegistry().GetSpecs() {
		if spec.CliFlags != nil && spec.FromCli != nil {
			out = append(out, makeMiddlewareCommands(spec))
		}
	}
	return out
}

func makeMiddlewareCommands(spec *MiddlewareSpec) cli.Command {
	return cli.Command{
		Name:  spec.Type,
		Usage: fmt.Sprintf("Operations on %s middlewares", spec.Type),
		Subcommands: []cli.Command{
			{
				Name:  "add",
				Usage: fmt.Sprintf("Add a new %s to location", spec.Type),
				Flags: append(spec.CliFlags,
					cli.StringFlag{"host", "", "location's host"},
					cli.StringFlag{"location, loc", "", "Location id"}),
				Action: makeAddMiddlewareAction(spec),
			},
			{
				Name:   "rm",
				Usage:  fmt.Sprintf("Remove %s from location", spec.Type),
				Action: makeDeleteMiddlewareAction(spec),
				Flags: []cli.Flag{
					cli.StringFlag{"host", "", "location's host"},
					cli.StringFlag{"location, loc", "", "Location id"},
					cli.StringFlag{"id", "", fmt.Sprintf("%s id", spec.Type)},
				},
			},
		},
	}
}

func makeAddMiddlewareAction(spec *MiddlewareSpec) func(c *cli.Context) {
	return func(c *cli.Context) {
		m, err := spec.FromCli(c)
		if err != nil {
			printError(err)
		} else {
			mi := &MiddlewareInstance{Id: c.String("id"), Middleware: m}
			response, err := client(c).AddMiddleware(spec, c.String("host"), c.String("loc"), mi)
			printResult("%s added", response, err)
		}
	}
}

func makeDeleteMiddlewareAction(spec *MiddlewareSpec) func(c *cli.Context) {
	return func(c *cli.Context) {
		printStatus(client(c).DeleteMiddleware(spec, c.String("host"), c.String("loc"), c.String("id")))
	}
}
