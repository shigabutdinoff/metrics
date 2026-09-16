package main

import stdos "os"

func main() {
	stdos.Exit(1) // want "прямой вызов os.Exit"
}
