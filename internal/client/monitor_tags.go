package client

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
)

// TagMonitorTypes are the taggable monitor types (TAG_MONITOR_TYPES, server/lib/tags/monitor-types.ts).
var TagMonitorTypes = []string{"website", "dns", "icmp", "smtp", "ssh", "tcp", "ftp", "imap_pop", "customer_domain", "customer_ip"}

// MaxTagsPerMonitor mirrors MAX_TAGS_PER_MONITOR in server/api/monitor-tags/index.post.ts.
const MaxTagsPerMonitor = 5

// MonitorTagRef is the POST/DELETE /api/monitor-tags body.
type MonitorTagRef struct {
	MonitorType string `json:"monitorType"`
	MonitorID   int64  `json:"monitorId"`
	TagID       int64  `json:"tagId"`
}

// MonitorTag is the assignment row POST answers.
type MonitorTag struct {
	ID             int64  `json:"id"`
	TagID          int64  `json:"tagId"`
	MonitorType    string `json:"monitorType"`
	MonitorID      int64  `json:"monitorId"`
	OrganizationID int64  `json:"organizationId"`
	CreatedAt      string `json:"createdAt"`
}

// ListMonitorTags reads the tags on one monitor (envelope form). Empty when the monitor does not exist.
func (c *Client) ListMonitorTags(ctx context.Context, monitorType string, monitorID int64) ([]Tag, error) {
	q := url.Values{"monitorType": {monitorType}, "monitorId": {strconv.FormatInt(monitorID, 10)}, "envelope": {"items"}}
	var page struct {
		Items []Tag `json:"items"`
	}
	return page.Items, c.Do(ctx, http.MethodGet, "/api/monitor-tags?"+q.Encode(), nil, &page)
}

// AddMonitorTag is idempotent: an existing assignment is returned as is.
func (c *Client) AddMonitorTag(ctx context.Context, in MonitorTagRef) (*MonitorTag, error) {
	var mt MonitorTag
	return &mt, c.Do(ctx, http.MethodPost, "/api/monitor-tags", in, &mt)
}

func (c *Client) RemoveMonitorTag(ctx context.Context, in MonitorTagRef) error {
	return c.Do(ctx, http.MethodDelete, "/api/monitor-tags", in, nil)
}
