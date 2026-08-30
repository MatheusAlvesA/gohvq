package log

import (
	"fmt"
)

// Define ANSI color codes
const (
	Reset   = "\033[0m"
	Error   = "\033[31m"
	Success = "\033[32m"
	Warning = "\033[33m"
	Info    = "\033[36m"
)

type LogService struct{}

func (s *LogService) PrintLn(logType string, tag string, message string) {
	fmt.Printf("%s[%s]%s %s\n", logType, tag, Reset, message)
}

func InitService() *LogService {
	return &LogService{}
}
