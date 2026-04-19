package plugins

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"maps"
	"math/rand"
	"time"

	"github.com/dnswlt/swcat/internal/catalog"
	"github.com/dnswlt/swcat/internal/database"
)

const (
	DefaultBaseInterval  = 1 * time.Hour
	DefaultBackoffFactor = 2.0
	DefaultTaskTimeout   = 60 * time.Second
	DefaultBackoffMax    = 7 * 24 * time.Hour
)

type entityState struct {
	ref      *catalog.Ref
	nextDue  time.Time
	backoff  time.Duration
	inFlight bool
}

type taskResult struct {
	ref     *catalog.Ref
	success bool
}

type SchedulerConfig struct {
	Enabled       bool          `yaml:"enabled"`
	BaseInterval  time.Duration `yaml:"baseInterval"`
	BackoffFactor float64       `yaml:"backoffFactor"`
	BackoffMax    time.Duration `yaml:"backoffMax"`
	// Maximum time a single task is allowed to run.
	TaskTimeout time.Duration `yaml:"taskTimeout"`
}

type Scheduler struct {
	config   SchedulerConfig
	registry *Registry
	provider RepositoryProvider
	db       *sql.DB
	entities map[string]*entityState
	results  chan taskResult
}

func NewScheduler(config SchedulerConfig, registry *Registry, provider RepositoryProvider, db *sql.DB) *Scheduler {
	if config.BackoffFactor <= 1 {
		config.BackoffFactor = DefaultBackoffFactor
	}
	if config.BaseInterval <= 0 {
		config.BaseInterval = DefaultBaseInterval
	}
	if config.BackoffMax <= 0 {
		config.BackoffMax = DefaultBackoffMax
	}
	if config.TaskTimeout <= 0 {
		config.TaskTimeout = DefaultTaskTimeout
	}
	log.Printf("Plugin scheduler config: %+v", config)
	return &Scheduler{
		config:   config,
		registry: registry,
		provider: provider,
		db:       db,
		entities: make(map[string]*entityState),
		results:  make(chan taskResult, 64),
	}
}

func (s *Scheduler) updateEntities() {
	entities := make(map[string]bool)
	now := time.Now()
	repo := s.provider.GetRepository()
	if repo == nil {
		return
	}
	for _, e := range repo.AllEntities() {
		ref := e.GetRef()
		id := ref.String()
		entities[id] = true // Mark as present
		if _, ok := s.entities[id]; !ok {
			// Register new entity with a random initial delay to avoid
			// thundering herd on startup.
			initialDelay := time.Duration(rand.Int63n(int64(s.config.BaseInterval)))
			s.entities[id] = &entityState{
				ref:     e.GetRef(),
				nextDue: now.Add(initialDelay),
				backoff: s.config.BaseInterval,
			}
		}
	}
	// Delete entities that no longer exist
	maps.DeleteFunc(s.entities, func(id string, e *entityState) bool {
		return !entities[id]
	})
}

func (s *Scheduler) processResult(ctx context.Context, entity catalog.Entity, result *RunResult) error {
	// Only store observations; annotations are stored as side-car files in git and cannot get updated here.
	catalog.MergeObservations(entity, result.Observations)
	if s.db != nil {
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		err := database.StoreObservations(ctx, s.db, entity)
		cancel()
		if err != nil {
			return fmt.Errorf("failed to store observations in DB: %v", err)
		}
	}
	return nil
}

func (s *Scheduler) runPlugins(ctx context.Context, ref *catalog.Ref, results chan<- taskResult) {
	repo := s.provider.GetRepository()
	var entity catalog.Entity
	if repo != nil {
		entity = repo.Entity(ref)
	}
	success := false
	if entity != nil {
		result, err := s.registry.Run(ctx, s.provider, entity)
		if err != nil {
			log.Printf("Plugin run failed for %s: %v", ref, err)
		} else if err := s.processResult(ctx, entity, result); err != nil {
			log.Print(err)
		} else {
			success = true
		}
	}
	select {
	case results <- taskResult{
		ref:     ref,
		success: success,
	}:
	case <-ctx.Done():
	}
}

func (s *Scheduler) nextBackoff(current time.Duration) time.Duration {
	next := time.Duration(float64(current) * s.config.BackoffFactor)
	next = min(next, s.config.BackoffMax)
	jitter := time.Duration(rand.Int63n(int64(next / 10)))
	return next + jitter
}

func (s *Scheduler) Run(ctx context.Context) {

	if !s.config.Enabled {
		log.Printf("Called Scheduler.Run on disabled scheduler. Terminating.")
		return
	}

	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			now := time.Now()
			s.updateEntities()
			for _, state := range s.entities {
				if state.inFlight || now.Before(state.nextDue) {
					continue
				}
				state.inFlight = true
				taskCtx, cancel := context.WithTimeout(ctx, s.config.TaskTimeout)
				go func(ref *catalog.Ref) {
					defer cancel()
					s.runPlugins(taskCtx, ref, s.results)
				}(state.ref)
			}

		case result := <-s.results:
			state, ok := s.entities[result.ref.String()]
			if !ok {
				continue // Entity got removed in the meantime, ignore result
			}
			state.inFlight = false
			if result.success {
				state.backoff = s.config.BaseInterval
				state.nextDue = time.Now().Add(s.config.BaseInterval)
			} else {
				state.backoff = s.nextBackoff(state.backoff)
				state.nextDue = time.Now().Add(state.backoff)
			}

		case <-ctx.Done():
			log.Printf("Terminating plugin scheduler (context is done).")
			return
		}
	}
}
