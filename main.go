package main

import (
	"MatheusAlvesA/gohvq/src/server"
	"fmt"
)

func main() {
	fmt.Println("Hello, World!")

	server := server.InitServer()

	server.Start()
}
