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
	"github.com/dnswlt/swcat/internal/status"
)

const (
	DefaultBaseInterval  = 1 * time.Hour
	DefaultBackoffFactor = 2.0
	DefaultTaskTimeout   = 60 * time.Second
	DefaultBackoffMax    = 7 * 24 * time.Hour
)

// entityState tracks the per-entity scheduling state owned by the Run loop.
type entityState struct {
	ref      *catalog.Ref  // The entity this state refers to.
	nextDue  time.Time     // Earliest time at which the entity may run again.
	backoff  time.Duration // Current retry delay; reset to BaseInterval on success.
	inFlight bool          // True while a plugin run for this entity is in progress.
}

// taskResult is the outcome of a single plugin run, sent from a worker
// goroutine back to the scheduler loop.
type taskResult struct {
	ref     *catalog.Ref
	success bool
}

// SchedulerConfig is the YAML-serializable configuration for Scheduler.
// Zero or invalid fields are replaced by package defaults in NewScheduler.
type SchedulerConfig struct {
	// Enabled gates whether Run actually starts the loop.
	Enabled bool `yaml:"enabled"`
	// BaseInterval is the cadence at which a healthy entity is re-evaluated.
	BaseInterval time.Duration `yaml:"baseInterval"`
	// BackoffFactor is the multiplier applied to the current backoff after a failure.
	BackoffFactor float64 `yaml:"backoffFactor"`
	// BackoffMax caps the post-failure retry delay.
	BackoffMax time.Duration `yaml:"backoffMax"`
	// TaskTimeout is the maximum time a single plugin run is allowed to take.
	TaskTimeout time.Duration `yaml:"taskTimeout"`
}

// Scheduler periodically runs the registered plugins for each entity in the
// repository, with per-entity exponential backoff on failure.
type Scheduler struct {
	config        SchedulerConfig
	registry      *Registry
	provider      RepositoryProvider
	db            *sql.DB
	statusUpdater status.Updater
	entities      map[string]*entityState
	results       chan taskResult
	stats         SchedulerStats
}

type SchedulerStats struct {
	Processed   int       `json:"processed"`
	Errors      int       `json:"errors"`
	LastRunTime time.Time `json:"lastRunTime"`
}

// NewScheduler returns a Scheduler ready to be started with Run. Zero or
// invalid fields in config are replaced by their package defaults, and the
// resolved configuration is logged.
func NewScheduler(config SchedulerConfig, registry *Registry, provider RepositoryProvider, db *sql.DB, statusUpdater status.Updater) *Scheduler {
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
		config:        config,
		registry:      registry,
		provider:      provider,
		db:            db,
		statusUpdater: statusUpdater,
		entities:      make(map[string]*entityState),
		results:       make(chan taskResult, 64),
	}
}

// updateEntities reconciles the scheduler's entity table with the current
// repository: new entities are registered with a randomized initial delay
// (to avoid a thundering herd at startup), and entries for entities that no
// longer exist are dropped.
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

// processResult applies the aggregated plugin run result to entity and, if a
// database is configured, persists the entity's current observations.
// Only observations are propagated; annotations are stored as side-car files
// in git and cannot be updated here.
func (s *Scheduler) processResult(ctx context.Context, entity catalog.Entity, result *RunResult) error {
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

// runPlugins executes all matching plugins for the entity identified by ref
// and reports the outcome on results. It is intended to be called in its own
// goroutine; the result send is guarded by ctx so a cancelled run doesn't
// block on a full channel. An entity that has vanished from the repository
// in the meantime is reported as a non-success without invoking any plugins.
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

// nextBackoff returns the next retry delay derived from current by multiplying
// with BackoffFactor, capping at BackoffMax, and adding up to 10% jitter.
func (s *Scheduler) nextBackoff(current time.Duration) time.Duration {
	next := time.Duration(float64(current) * s.config.BackoffFactor)
	next = min(next, s.config.BackoffMax)
	jitter := time.Duration(rand.Int63n(int64(next / 10)))
	return next + jitter
}

// Run drives the scheduler loop until ctx is cancelled. On each tick it
// reconciles the entity table and dispatches due plugin runs in their own
// goroutines (capped per task by TaskTimeout). Successful runs reset the
// entity's cadence to BaseInterval; failed runs back off exponentially up to
// BackoffMax. If the scheduler is disabled, Run returns immediately.
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
			s.stats.LastRunTime = now
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
			s.statusUpdater.Update("pluginsScheduler", s.stats)

		case result := <-s.results:
			s.stats.Processed++
			state, ok := s.entities[result.ref.String()]
			if !ok {
				continue // Entity got removed in the meantime, ignore result
			}
			state.inFlight = false
			if result.success {
				state.backoff = s.config.BaseInterval
				state.nextDue = time.Now().Add(s.config.BaseInterval)
			} else {
				s.stats.Errors++
				state.backoff = s.nextBackoff(state.backoff)
				state.nextDue = time.Now().Add(state.backoff)
			}

		case <-ctx.Done():
			log.Printf("Terminating plugin scheduler (context is done).")
			return
		}
	}
}
