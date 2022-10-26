package netmon

import (
	"log"
	"sync"
)

// Logger definition.
type Logger interface {
	Printf(format string, v ...any)
}

// Log implementation
type Log struct {
	mu sync.Mutex
}

// Printf calls the underlying logger in a goroutine safe manner.
func (l *Log) Printf(format string, v ...any) {
	l.mu.Lock()
	log.Printf(format, v...)
	l.mu.Unlock()
}

// Fatalln calls the underlying logger in a goroutine safe manner.
func (l *Log) Fatalln(v ...any) {
	l.mu.Lock()
	log.Fatalln(v...)
	l.mu.Unlock()
}
