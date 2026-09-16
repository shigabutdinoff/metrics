package main

import "os"

func main() {
	shutdown()
}

func shutdown() {
	os.Exit(1)
}
