package main

import (
	"log"
	"time"

	"github.com/zuzuviewer/lik/cmd"
)

func main() {
	startTime := time.Now()
	defer func() {
		log.Printf("total cost %s", time.Since(startTime).String())
	}()
	cmd.Run()
}
