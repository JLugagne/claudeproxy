package usageuc

import (
	"context"

	"github.com/JLugagne/claudeproxy/internal/proxy/domain"
)

// MetricsQueries is the read side of the usage store: the dashboard's only
// dependency on recorded metrics.
type MetricsQueries interface {
	Totals(ctx context.Context, r domain.MetricsRange) (domain.MetricsTotals, error)
	Volume(ctx context.Context, r domain.MetricsRange) ([]domain.MetricsPoint, error)
	Projects(ctx context.Context, r domain.MetricsRange) ([]domain.ProjectStat, error)
	Models(ctx context.Context, r domain.MetricsRange) ([]domain.ModelStat, error)
	Sessions(ctx context.Context, r domain.MetricsRange, limit int) ([]domain.SessionStat, error)
	Providers(ctx context.Context, r domain.MetricsRange) ([]domain.ProviderStat, error)
	Requests(ctx context.Context, r domain.MetricsRange, f domain.UsageFilter) ([]domain.UsageRow, int64, error)
}
