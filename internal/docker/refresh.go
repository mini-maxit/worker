package docker

import (
	"context"
	"time"
)

// RefreshImagesPeriodically re-pulls images every interval until ctx is done.
// Failures are logged by RefreshImage and retried on the next tick.
func RefreshImagesPeriodically(ctx context.Context, d DockerClient, images []string, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for _, img := range images {
				_ = d.RefreshImage(ctx, img)
			}
		}
	}
}
