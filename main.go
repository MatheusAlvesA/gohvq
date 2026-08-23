package main

import (
	"MatheusAlvesA/gohvq/src/repository"
	"MatheusAlvesA/gohvq/src/server"
	"fmt"
)

func main() {
	fmt.Println("Go Human Virtual Queue!")

	repositoryInstance := repository.InitRepository()
	serverInstance := server.InitServer()

	serverInstance.SetRepository(repositoryInstance)

	serverInstance.Start()
}
