package main

import "os"

type fakeOS struct{}

func (fakeOS) Exit(code int) {}

func main() {
	os := fakeOS{}
	os.Exit(1)
}

func home() string {
	return os.Getenv("HOME")
}
