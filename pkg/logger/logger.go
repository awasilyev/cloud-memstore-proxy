package logger

import (
	"fmt"
	"log"
	"os"
	"strings"
)

var (
	infoLog  *log.Logger
	errorLog *log.Logger
	debugLog *log.Logger
	traceLog *log.Logger
	verbose  bool
	tracing  bool
)

func Init(v bool) {
	verbose = v
	infoLog = log.New(os.Stdout, "INFO: ", log.Ldate|log.Ltime)
	errorLog = log.New(os.Stderr, "ERROR: ", log.Ldate|log.Ltime)
	debugLog = log.New(os.Stdout, "DEBUG: ", log.Ldate|log.Ltime|log.Lshortfile)
	traceLog = log.New(os.Stdout, "TRACE: ", log.Ldate|log.Ltime)
}

func SetTracing(t bool) {
	tracing = t
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

func Trace(direction string, data []byte) {
	if !tracing {
		return
	}
	if traceLog == nil {
		Init(false)
	}

	// Format the trace with direction and hex dump of the raw protocol
	hexDump := fmt.Sprintf("%x", data)
	// Also show readable representation where possible
	readable := escapeBytes(data)

	traceLog.Printf("[%s] %s (hex: %s)", direction, readable, hexDump)
}

// escapeBytes converts bytes to a readable string, escaping non-printable characters
func escapeBytes(data []byte) string {
	var buf strings.Builder
	for _, b := range data {
		if b >= 32 && b < 127 && b != '\\' {
			buf.WriteByte(b)
		} else if b == '\r' {
			buf.WriteString("\\r")
		} else if b == '\n' {
			buf.WriteString("\\n")
		} else if b == '\\' {
			buf.WriteString("\\\\")
		} else {
			buf.WriteString(fmt.Sprintf("\\x%02x", b))
		}
	}
	return buf.String()
}
