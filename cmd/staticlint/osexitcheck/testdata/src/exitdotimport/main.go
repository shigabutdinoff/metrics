package main

import . "os"

func main() {
	Exit(1) // want "прямой вызов os.Exit"
}
