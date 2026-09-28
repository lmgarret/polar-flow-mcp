package flow

import (
	"context"
	"strconv"
	"sync"
	"time"
)

// sportCatalogTTL bounds how long the sport catalogue is reused. Polar adds
// sports rarely; an hour keeps validation cheap without pinning a stale list
// for the life of the process.
const sportCatalogTTL = time.Hour

type sportCatalog struct {
	mu      sync.Mutex
	fetched time.Time
	byID    map[int]string
}

// SportName returns the sport-name constant for id (e.g. 1 → "RUNNING") and
// whether the id is in Polar's catalogue. Several write endpoints
// (saveSport, favorite create/update) store unknown sport ids verbatim and
// editTraining answers them with a bare 500, so write tools check ids here
// before sending. The catalogue is fetched once and cached for an hour.
func (c *Client) SportName(ctx context.Context, id int) (string, bool, error) {
	c.sports.mu.Lock()
	defer c.sports.mu.Unlock()
	if c.sports.byID == nil || time.Since(c.sports.fetched) > sportCatalogTTL {
		m, err := c.ListSports(ctx)
		if err != nil {
			return "", false, err
		}
		byID := make(map[int]string, len(m))
		for k, v := range m {
			if n, err := strconv.Atoi(k); err == nil {
				byID[n] = v
			}
		}
		c.sports.byID = byID
		c.sports.fetched = time.Now()
	}
	name, ok := c.sports.byID[id]
	return name, ok, nil
}
