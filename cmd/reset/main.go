package main

import (
	"log"

	"github.com/shigabutdinoff/metrics/internal/resetgen"
)

func main() {
	if err := resetgen.Run("./..."); err != nil {
		log.Fatal(err)
	}
}
