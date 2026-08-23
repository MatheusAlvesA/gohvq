package main

import (
	"MatheusAlvesA/gohvq/src/repository"
	"MatheusAlvesA/gohvq/src/server"
	"fmt"
)

func main() {
	fmt.Println("Hello, World!")

	repositoryInstance := repository.InitRepository()
	serverInstance := server.InitServer()

	serverInstance.SetRepository(repositoryInstance)

	serverInstance.Start()
}
