// Package netmon handles network monitoring tasks.
package netmon

import (
	"context"
	"errors"
	"log"
	"time"
)

type (
	// PingFunc definition.
	PingFunc func(address string) error
	// SpeedTestFunc definition.
	SpeedTestFunc func(ctx context.Context, serverIDs []int) error
)

// SpeedConfig definition.
type SpeedConfig struct {
	ServerIDs []int
	Interval  time.Duration
}

// PingConfig definition.
type PingConfig struct {
	Addresses []string
	Interval  time.Duration
}

// Scheduler definition.
type Scheduler struct {
	pingCfg       PingConfig
	speedCfg      SpeedConfig
	pingFunc      PingFunc
	speedTestFunc SpeedTestFunc
}

// NewScheduler constructor.
func NewScheduler(pingCfg PingConfig, speedCfg SpeedConfig, pingFunc PingFunc, speedTestFunc SpeedTestFunc) (*Scheduler, error) {
	if pingFunc == nil {
		return nil, errors.New("ping func is nil")
	}
	if speedTestFunc == nil {
		return nil, errors.New("speed test func is nil")
	}
	return &Scheduler{pingCfg: pingCfg, speedCfg: speedCfg, pingFunc: pingFunc, speedTestFunc: speedTestFunc}, nil
}

// Schedule ping and speed test according to the provided configuration.
func (s *Scheduler) Schedule(ctx context.Context) {
	// run initial measurement
	s.ping()
	s.speedTest(ctx)

	pingTimer := time.NewTicker(s.pingCfg.Interval)
	speedTimer := time.NewTicker(s.speedCfg.Interval)

	for {
		select {
		case <-ctx.Done():
			return
		case <-pingTimer.C:
			pingTimer.Stop()
			s.ping()
			pingTimer.Reset(s.pingCfg.Interval)
		case <-speedTimer.C:
			speedTimer.Stop()
			s.speedTest(ctx)
			speedTimer.Reset(s.speedCfg.Interval)
		}
	}
}

func (s *Scheduler) ping() {
	for _, address := range s.pingCfg.Addresses {
		err := s.pingFunc(address)
		if err != nil {
			log.Printf("failed to ping: %v\n", err)
		}
	}
}

func (s *Scheduler) speedTest(ctx context.Context) {
	err := s.speedTestFunc(ctx, s.speedCfg.ServerIDs)
	if err != nil {
		log.Printf("failed to speed test: %v\n", err)
	}
}
