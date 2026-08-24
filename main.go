package main

import (
	"MatheusAlvesA/gohvq/src/repository"
	"MatheusAlvesA/gohvq/src/server"
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	fmt.Println("Go Human Virtual Queue!")

	shutdownCall := make(chan os.Signal, 1)
	signal.Notify(shutdownCall, syscall.SIGTERM, syscall.SIGINT)

	repositoryInstance := repository.InitRepository()
	serverInstance := server.InitServer()

	serverInstance.SetRepository(repositoryInstance)

	serverInstance.Start()

	<-shutdownCall
	fmt.Println("Shutting Down...")
}
