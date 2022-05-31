package main

import (
	"fmt"
	"github.com/urfave/cli/v2"
	"log"
	"os"
	"strconv"
)

var MaxMessageSize = 1000
var DefaultTTL uint32 = 100
var DefaultWorkerCount = 5
var DefaultWorkerQueueSize = 10

func main() {
	app := &cli.App{
		Name:  "dns-resolver",
		Usage: "Another useless dns resolver",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "worker-count",
				Value: strconv.Itoa(DefaultWorkerCount),
				Usage: "dns resolver worker count",
			},
			&cli.StringFlag{
				Name:  "queue-size",
				Value: strconv.Itoa(DefaultWorkerQueueSize),
				Usage: "each worker queue size",
			},
		},

		Action: func(c *cli.Context) error {
			if c.NArg() != 1 {
				fmt.Println("Required zone file path")
				return nil
			}
			zoneFilePath := c.Args().Get(0)
			workerCount := c.Int("worker-count")
			queueSize := c.Int("queue-size")
			StartServer(zoneFilePath, workerCount, queueSize)
			return nil
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}
