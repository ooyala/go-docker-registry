package logger

import (
	"log"
)

var debug = false

func DebugOn() {
	debug = true
}

func DebugOff() {
	debug = false
}

func Info(fmt string, args ...interface{}) {
	log.Printf("[INFO]  "+fmt, args...)
}

func Error(fmt string, args ...interface{}) {
	log.Printf("[ERROR] "+fmt, args...)
}

func Fatal(fmt string, args ...interface{}) {
	log.Fatalf("[FATAL] "+fmt, args...)
}

func Debug(fmt string, args ...interface{}) {
	if debug {
		log.Printf("[DEBUG] "+fmt, args...)
	}
}
