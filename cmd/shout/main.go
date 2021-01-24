package main

import (
	"fmt"
	"os"
	"sort"

	"github.com/sirupsen/logrus"
	"github.com/superfly/shout"
	"github.com/urfave/cli/v2"
)

func main() {
	logger := logrus.New()
	hostname, err := os.Hostname()
	if err != nil {
		logger.Fatalf("error getting hostname: %v", err)
	}
	app := &cli.App{
		Name:  "shout",
		Usage: "Shout[ing] is a way to communicate with other hot air balloon operators",
		Commands: []*cli.Command{
			{
				Name:  "run",
				Usage: "run the shout state propagator",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "db",
						Value: "./db.sqlite",
						Usage: "path to sqlite database",
					},
					&cli.StringFlag{
						Name:  "rpc-addr",
						Value: "0.0.0.0:7373",
						Usage: "serf RPC address",
					},
					&cli.IntFlag{
						Name:  "memberlist-port",
						Value: 7946,
						Usage: "memberlist bind port",
					},
					&cli.StringFlag{
						Name:  "node",
						Value: hostname,
						Usage: "node name",
					},
					&cli.StringFlag{
						Name:  "handlers",
						Value: "./handlers",
						Usage: "path to event and query handlers",
					},
					&cli.StringFlag{
						Name:  "migrations",
						Value: "file://migrations",
						Usage: "path to migration files",
					},
				},
				Action: func(c *cli.Context) error {
					return run(c, logger)
				},
			},
		},
	}

	sort.Sort(cli.FlagsByName(app.Flags))
	sort.Sort(cli.CommandsByName(app.Commands))

	if err := app.Run(os.Args); err != nil {
		logger.Fatal(err)
	}
}

func run(c *cli.Context, logger *logrus.Logger) error {

	s, err := shout.New(&shout.Config{
		DBPath:           c.String("db"),
		RPCAddr:          c.String("rpc-addr"),
		MemberlistPort:   c.Int("memberlist-port"),
		NodeName:         c.String("node"),
		HandlersPath:     c.String("handlers"),
		MigrationsSource: c.String("migrations"),

		Logger: logger,
	})
	if err != nil {
		return fmt.Errorf("could not create shout instance: %v", err)
	}
	s.Run()
	return nil
}
