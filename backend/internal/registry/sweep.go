package registry

import (
	"context"
	"log"
	"time"
)

const bundlePrefix = "registry/"

// OrphanMinAge exceeds any upload request lifetime and UploadTTL, so neither an
// object between storage write and transaction commit nor a completed part
// upload awaiting its save is mistaken for an orphan.
const OrphanMinAge = 24 * time.Hour

type ObjectLister interface {
	ListOlder(ctx context.Context, prefix string, cutoff time.Time) ([]string, error)
}

// SweepOrphanBundles removes bundles left behind when request-time cleanup
// failed. Each key is rechecked against the database before deletion.
func (s *Service) SweepOrphanBundles(ctx context.Context, lister ObjectLister, now time.Time) (int, error) {
	keys, err := lister.ListOlder(ctx, bundlePrefix, now.Add(-OrphanMinAge))
	if err != nil {
		return 0, err
	}
	deleted := 0
	for _, key := range keys {
		used, err := s.repo.BundleReferenced(ctx, key)
		if err != nil {
			return deleted, err
		}
		if used {
			continue
		}
		if err = s.storage.Delete(ctx, key); err != nil {
			return deleted, err
		}
		deleted++
	}
	return deleted, nil
}

// RunOrphanSweeper sweeps shortly after start and then at each interval until ctx ends.
func (s *Service) RunOrphanSweeper(ctx context.Context, lister ObjectLister, interval time.Duration) {
	timer := time.NewTimer(time.Minute)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		if n, err := s.SweepExpiredUploads(ctx, time.Now()); err != nil && ctx.Err() == nil {
			log.Print("registry: expired bundle upload sweep failed; retrying next interval")
		} else if n > 0 {
			log.Printf("registry: ended %d expired bundle uploads", n)
		}
		if n, err := s.SweepOrphanBundles(ctx, lister, time.Now()); err != nil && ctx.Err() == nil {
			log.Print("registry: orphan bundle sweep failed; retrying next interval")
		} else if n > 0 {
			log.Printf("registry: removed %d orphan bundles", n)
		}
		timer.Reset(interval)
	}
}
