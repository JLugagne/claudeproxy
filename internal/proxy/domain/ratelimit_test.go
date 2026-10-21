package domain_test

import (
	"testing"
	"time"

	"github.com/JLugagne/claudeproxy/internal/proxy/domain"
)

func snapshot(at int, project, session string, fiveHour, sevenDay float64) domain.RateLimitSnapshot {
	return domain.RateLimitSnapshot{
		Time:        time.Unix(int64(at), 0),
		Attribution: domain.Attribution{Project: project, Session: session},
		FiveHour:    domain.QuotaWindow{Utilization: fiveHour, Present: true},
		SevenDay:    domain.QuotaWindow{Utilization: sevenDay, Present: true},
	}
}

func TestAttributeQuotaChargesRiseToTheObservingSession(t *testing.T) {
	snaps := []domain.RateLimitSnapshot{
		snapshot(1, "a/x", "s1", 0.10, 0.20),
		snapshot(2, "a/x", "s1", 0.15, 0.21),
		snapshot(3, "b/y", "s2", 0.18, 0.25),
		snapshot(4, "b/y", "s2", 0.02, 0.25),
		snapshot(5, "a/x", "s1", 0.03, 0.26),
	}

	got := domain.AttributeQuota(snaps, domain.QuotaBySession)

	want := map[string]domain.QuotaShare{
		"s1": {Name: "s1", Project: "a/x", FiveHour: 0.06, SevenDay: 0.02},
		"s2": {Name: "s2", Project: "b/y", FiveHour: 0.03, SevenDay: 0.04},
	}
	if len(got) != len(want) {
		t.Fatalf("shares = %+v, want %d entries", got, len(want))
	}
	for _, share := range got {
		w := want[share.Name]
		if share.Project != w.Project || !near(share.FiveHour, w.FiveHour) || !near(share.SevenDay, w.SevenDay) {
			t.Errorf("share %s = %+v, want %+v", share.Name, share, w)
		}
	}
	if got[0].Name != "s2" {
		t.Errorf("first share = %s, want the largest 7d consumer s2", got[0].Name)
	}
}

func TestAttributeQuotaGroupsByProject(t *testing.T) {
	snaps := []domain.RateLimitSnapshot{
		snapshot(1, "a/x", "s1", 0.10, 0.20),
		snapshot(2, "a/x", "s2", 0.12, 0.22),
		snapshot(3, "a/x", "s1", 0.13, 0.23),
	}

	got := domain.AttributeQuota(snaps, domain.QuotaByProject)

	if len(got) != 1 || got[0].Name != "a/x" || !near(got[0].SevenDay, 0.03) || !near(got[0].FiveHour, 0.03) {
		t.Fatalf("shares = %+v", got)
	}
}

func TestDownsampleQuotaKeepsTheLatestOfEachBucket(t *testing.T) {
	var snaps []domain.RateLimitSnapshot
	for i := 0; i < 100; i++ {
		snaps = append(snaps, snapshot(i, "p", "s", float64(i)/100, 0))
	}

	got := domain.DownsampleQuota(snaps, 10)

	if len(got) > 10 {
		t.Fatalf("len = %d, want at most 10", len(got))
	}
	if last := got[len(got)-1]; last.Time != snaps[99].Time {
		t.Errorf("last = %v, want the newest snapshot %v", last.Time, snaps[99].Time)
	}
	if small := domain.DownsampleQuota(snaps[:5], 10); len(small) != 5 {
		t.Errorf("short series resampled to %d, want 5", len(small))
	}
}

func near(a, b float64) bool {
	d := a - b
	return d < 1e-9 && d > -1e-9
}
