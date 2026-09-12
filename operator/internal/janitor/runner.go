package janitor

import (
	"context"
	"time"

	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

// DefaultSweepInterval is how often the periodic sweep runs when nothing has
// triggered one. Slow on purpose: the deletion-triggered sweep does the real
// work; the tick only converges what a restart or a draining garbage
// collection left behind.
const DefaultSweepInterval = 10 * time.Minute

// Periodic returns a manager Runnable that sweeps on the given interval,
// under leader election like the controllers (two replicas must not both
// delete). A failing sweep is logged and retried on the next tick; nothing
// here is urgent enough to crash the manager.
func Periodic(j *Janitor, interval time.Duration) manager.Runnable {
	return manager.RunnableFunc(func(ctx context.Context) error {
		log := logf.FromContext(ctx).WithName("janitor")
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return nil
			case <-ticker.C:
				outcome, err := j.Sweep(ctx)
				if err != nil {
					log.Error(err, "Periodic sweep failed; retrying on the next tick")
					continue
				}
				if outcome.SatellitesRemoved > 0 || len(outcome.Verdicts) > 0 {
					log.Info("Periodic sweep complete",
						"platformsRemaining", outcome.PlatformsRemaining,
						"satellitesRemoved", outcome.SatellitesRemoved,
						"subOperators", outcome.SortedVerdictNames())
				}
			}
		}
	})
}

// NeedsLeaderElection makes the periodic sweep a leader-only runnable:
// controller-runtime asks this through the LeaderElectionRunnable interface
// when the Runnable implements it. RunnableFunc does not, so Periodic's
// result is wrapped by LeaderOnly.
type leaderOnly struct{ manager.Runnable }

func (leaderOnly) NeedsLeaderElection() bool { return true }

// LeaderOnly wraps a Runnable so it runs on the elected leader only.
func LeaderOnly(r manager.Runnable) manager.Runnable { return leaderOnly{r} }
