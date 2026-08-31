package main

import (
	"MatheusAlvesA/gohvq/src/config"
	"MatheusAlvesA/gohvq/src/log"
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
	logService := log.InitService()
	config := config.InitService()

	serverInstance.SetLogService(logService)
	serverInstance.SetRepository(repositoryInstance)
	repositoryInstance.SetLogService(logService)
	config.SetServices(serverInstance, repositoryInstance, logService)

	config.Start()
	repositoryInstance.Start()
	serverInstance.Start()

	<-shutdownCall
	fmt.Println()
	fmt.Println("Shutting Down...")
	serverInstance.Stop()
	repositoryInstance.Stop()
}
