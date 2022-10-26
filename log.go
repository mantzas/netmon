package netmon

import (
	"log"
	"sync"
)

type Logger interface {
	Printf(format string, v ...any)
}

type Log struct {
	mu sync.Mutex
}

func (l *Log) Printf(format string, v ...any) {
	l.mu.Lock()
	log.Printf(format, v...)
	l.mu.Unlock()
}

func (l *Log) Fatalln(v ...any) {
	l.mu.Lock()
	log.Fatalln(v...)
	l.mu.Unlock()
}
