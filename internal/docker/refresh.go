package docker

import (
	"context"
	"time"
)

// RefreshImagesPeriodically refreshes images every interval until ctx is done.
func RefreshImagesPeriodically(ctx context.Context, d DockerClient, images []string, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			d.RefreshImages(ctx, images)
		}
	}
}
