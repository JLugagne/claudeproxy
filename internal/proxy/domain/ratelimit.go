package domain

import (
	"sort"
	"time"
)

// QuotaWindow is one rolling subscription window as the provider last
// reported it. Present is false when the provider sent no figure for it.
type QuotaWindow struct {
	// Utilization is the consumed share of the window, from 0 to 1.
	Utilization float64
	ResetsAt    time.Time
	Status      string
	Present     bool
}

// RateLimitSnapshot is the subscription quota a response reported, tied to the
// request that carried it.
type RateLimitSnapshot struct {
	Time time.Time
	Attribution
	FiveHour QuotaWindow
	SevenDay QuotaWindow
	// Claim names the window currently constraining the account.
	Claim string
}

// QuotaDimension selects what AttributeQuota groups by.
type QuotaDimension int

const (
	// QuotaByProject groups consumption by project.
	QuotaByProject QuotaDimension = iota
	// QuotaBySession groups consumption by session.
	QuotaBySession
)

// QuotaShare is the quota one project or session consumed, as ratios of each
// window. Project is only set when grouping by session.
type QuotaShare struct {
	Name     string
	Project  string
	FiveHour float64
	SevenDay float64
}

// AttributeQuota charges every rise of a window's utilization to the request
// that observed it. snapshots must be ordered by time. Drops, which are window
// resets, charge nothing. Concurrent sessions make the split approximate, and
// the first snapshot only seeds the baseline. Shares are sorted by seven-day
// consumption, then five-hour, then name.
func AttributeQuota(snapshots []RateLimitSnapshot, by QuotaDimension) []QuotaShare {
	shares := make(map[string]*QuotaShare)
	for i := 1; i < len(snapshots); i++ {
		prev, cur := snapshots[i-1], snapshots[i]
		name := cur.Project
		if by == QuotaBySession {
			name = cur.Session
		}
		share, ok := shares[name]
		if !ok {
			share = &QuotaShare{Name: name}
			if by == QuotaBySession {
				share.Project = cur.Project
			}
			shares[name] = share
		}
		share.FiveHour += rise(prev.FiveHour, cur.FiveHour)
		share.SevenDay += rise(prev.SevenDay, cur.SevenDay)
	}

	out := make([]QuotaShare, 0, len(shares))
	for _, share := range shares {
		out = append(out, *share)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].SevenDay != out[j].SevenDay {
			return out[i].SevenDay > out[j].SevenDay
		}
		if out[i].FiveHour != out[j].FiveHour {
			return out[i].FiveHour > out[j].FiveHour
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func rise(prev, cur QuotaWindow) float64 {
	if !prev.Present || !cur.Present || cur.Utilization <= prev.Utilization {
		return 0
	}
	return cur.Utilization - prev.Utilization
}

// DownsampleQuota keeps the newest snapshot of each of at most max equal time
// buckets, so a chart stays light however many requests were recorded.
// snapshots must be ordered by time.
func DownsampleQuota(snapshots []RateLimitSnapshot, max int) []RateLimitSnapshot {
	if max <= 0 || len(snapshots) <= max {
		return snapshots
	}
	first, last := snapshots[0].Time, snapshots[len(snapshots)-1].Time
	span := last.Sub(first)
	if span <= 0 {
		return snapshots[len(snapshots)-1:]
	}
	out := make([]RateLimitSnapshot, 0, max)
	lastBucket := -1
	for _, snapshot := range snapshots {
		bucket := min(int(float64(snapshot.Time.Sub(first))/float64(span)*float64(max)), max-1)
		if bucket == lastBucket {
			out[len(out)-1] = snapshot
			continue
		}
		out = append(out, snapshot)
		lastBucket = bucket
	}
	return out
}
