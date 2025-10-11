package logger

import (
	"fmt"
	"log"
	"os"
)

var (
	infoLog  *log.Logger
	errorLog *log.Logger
	debugLog *log.Logger
	verbose  bool
)

func Init(v bool) {
	verbose = v
	infoLog = log.New(os.Stdout, "INFO: ", log.Ldate|log.Ltime)
	errorLog = log.New(os.Stderr, "ERROR: ", log.Ldate|log.Ltime)
	debugLog = log.New(os.Stdout, "DEBUG: ", log.Ldate|log.Ltime|log.Lshortfile)
}

func Info(msg string) {
	if infoLog == nil {
		Init(false)
	}
	infoLog.Println(msg)
}

func Error(msg string) {
	if errorLog == nil {
		Init(false)
	}
	errorLog.Println(msg)
}

func Debug(msg string) {
	if !verbose {
		return
	}
	if debugLog == nil {
		Init(false)
	}
	debugLog.Println(msg)
}

func Fatal(msg string) {
	if errorLog == nil {
		Init(false)
	}
	errorLog.Println(msg)
	os.Exit(1)
}

func Debugf(format string, args ...interface{}) {
	Debug(fmt.Sprintf(format, args...))
}

func Infof(format string, args ...interface{}) {
	Info(fmt.Sprintf(format, args...))
}

func Errorf(format string, args ...interface{}) {
	Error(fmt.Sprintf(format, args...))
}
