package client

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

// Incident Management (server/lib/api-contract/im-teams.ts, im-tiers.ts, im-schedules.ts).
// Every /api/im/* call needs IM activated for the organization (POST /api/im/activate).

// IMTeam is GET /api/im/teams/{id} (members are left out: uptimeify_im_team_member owns them).
type IMTeam struct {
	ID                            int64   `json:"id"`
	Name                          string  `json:"name"`
	LabelColor                    *string `json:"labelColor"`
	AnalyticsEnabled              bool    `json:"analyticsEnabled"`
	OrganizationID                int64   `json:"organizationId"`
	CreatedAt                     string  `json:"createdAt"`
	Timezone                      *string `json:"timezone"`
	EscalationRepeats             int64   `json:"escalationRepeats"`
	EscalationRepeatsAfterMinutes *int64  `json:"escalationRepeatsAfterMinutes"`
	EscalationRepeatsStopMode     string  `json:"escalationRepeatsStopMode"`
	EngagementReportEnabled       bool    `json:"engagementReportEnabled"`
	EngagementReportDayOfWeek     *int64  `json:"engagementReportDayOfWeek"`
	EngagementReportTime          *string `json:"engagementReportTime"`
}

// IMTeamListItem is one row of GET /api/im/teams (bare array, no envelope form).
type IMTeamListItem struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type IMTeamCreate struct {
	Name             string  `json:"name"`
	LabelColor       *string `json:"labelColor,omitempty"`
	AnalyticsEnabled bool    `json:"analyticsEnabled"`
}

func imPath(format string, ids ...any) string { return "/api/im/" + fmt.Sprintf(format, ids...) }

func (c *Client) ListIMTeams(ctx context.Context) ([]IMTeamListItem, error) {
	var out []IMTeamListItem
	return out, c.Do(ctx, http.MethodGet, imPath("teams"), nil, &out)
}

func (c *Client) GetIMTeam(ctx context.Context, id int64) (*IMTeam, error) {
	var t IMTeam
	return &t, c.Do(ctx, http.MethodGet, imPath("teams/%d", id), nil, &t)
}

// CreateIMTeam returns the new id; POST takes name, labelColor and analyticsEnabled only, the
// settings go through UpdateIMTeam.
func (c *Client) CreateIMTeam(ctx context.Context, in IMTeamCreate) (int64, error) {
	var out struct {
		ID int64 `json:"id"`
	}
	return out.ID, c.Do(ctx, http.MethodPost, imPath("teams"), in, &out)
}

func (c *Client) UpdateIMTeam(ctx context.Context, id int64, patch map[string]any) error {
	return c.Do(ctx, http.MethodPatch, imPath("teams/%d", id), patch, nil)
}

func (c *Client) DeleteIMTeam(ctx context.Context, id int64) error {
	return c.Do(ctx, http.MethodDelete, imPath("teams/%d", id), nil, nil)
}

// IMTeamMember is GET /api/im/teams/{id}/members/{memberId}.
type IMTeamMember struct {
	ID     int64  `json:"id"`
	UserID string `json:"userId"`
	IMRole string `json:"imRole"`
}

func (c *Client) GetIMTeamMember(ctx context.Context, teamID, memberID int64) (*IMTeamMember, error) {
	var m IMTeamMember
	return &m, c.Do(ctx, http.MethodGet, imPath("teams/%d/members/%d", teamID, memberID), nil, &m)
}

// AddIMTeamMember adds an existing user of the organization (POST /members).
func (c *Client) AddIMTeamMember(ctx context.Context, teamID int64, userID, role string) (*IMTeamMember, error) {
	var m IMTeamMember
	return &m, c.Do(ctx, http.MethodPost, imPath("teams/%d/members", teamID), map[string]any{"userId": userID, "imRole": role}, &m)
}

// InviteIMTeamMember adds by email; an unknown address gets a new account and an invitation mail.
func (c *Client) InviteIMTeamMember(ctx context.Context, teamID int64, email, role string) (*IMTeamMember, error) {
	var m IMTeamMember
	return &m, c.Do(ctx, http.MethodPost, imPath("teams/%d/invite", teamID), map[string]any{"email": email, "imRole": role}, &m)
}

func (c *Client) UpdateIMTeamMemberRole(ctx context.Context, teamID, memberID int64, role string) error {
	return c.Do(ctx, http.MethodPatch, imPath("teams/%d/members/%d", teamID, memberID), map[string]any{"imRole": role}, nil)
}

func (c *Client) DeleteIMTeamMember(ctx context.Context, teamID, memberID int64) error {
	return c.Do(ctx, http.MethodDelete, imPath("teams/%d/members/%d", teamID, memberID), nil, nil)
}

// WeeklyWindow is a weekly time window; Until may be "24:00".
type WeeklyWindow struct {
	From  string  `json:"from"`
	Until string  `json:"until"`
	Days  []int64 `json:"days"`
}

type IMRotationMember struct {
	UserID      string `json:"userId"`
	MemberOrder int64  `json:"memberOrder"`
}

type IMRotation struct {
	ID            int64              `json:"id"`
	RotationOrder int64              `json:"rotationOrder"`
	Members       []IMRotationMember `json:"members"`
}

// IMTierSchedule is a schedule inside the tier tree (with rotations).
type IMTierSchedule struct {
	ID        int64        `json:"id"`
	Rotations []IMRotation `json:"rotations"`
}

// IMTier is GET /api/im/teams/{id}/tiers/{tierId}.
type IMTier struct {
	ID                         int64            `json:"id"`
	TierOrder                  int64            `json:"tierOrder"`
	AutoEscalationEnabled      bool             `json:"autoEscalationEnabled"`
	AutoEscalationAfterMinutes int64            `json:"autoEscalationAfterMinutes"`
	AutoEscalationStopMode     string           `json:"autoEscalationStopMode"`
	AutoEscalationSeverities   []string         `json:"autoEscalationSeverities"`
	AutoEscalationTimeFilters  []WeeklyWindow   `json:"autoEscalationTimeFilters"`
	Repeats                    int64            `json:"repeats"`
	RepeatsAfterMinutes        *int64           `json:"repeatsAfterMinutes"`
	RepeatsStopMode            string           `json:"repeatsStopMode"`
	Schedules                  []IMTierSchedule `json:"schedules"`
}

func (c *Client) GetIMTier(ctx context.Context, teamID, tierID int64) (*IMTier, error) {
	var t IMTier
	return &t, c.Do(ctx, http.MethodGet, imPath("teams/%d/tiers/%d", teamID, tierID), nil, &t)
}

// ListIMTiers reads the team's tiers via GET /escalations, ordered by tierOrder.
func (c *Client) ListIMTiers(ctx context.Context, teamID int64) ([]IMTier, error) {
	var out struct {
		Tiers []IMTier `json:"tiers"`
	}
	return out.Tiers, c.Do(ctx, http.MethodGet, imPath("teams/%d/escalations", teamID), nil, &out)
}

// CreateIMTier appends a tier. The API also creates a default schedule ("Tier N schedule") with
// one rotation of memberUserIds; it comes back in Schedules[0].
func (c *Client) CreateIMTier(ctx context.Context, teamID int64, memberUserIDs []string) (*IMTier, error) {
	var t IMTier
	return &t, c.Do(ctx, http.MethodPost, imPath("teams/%d/tiers", teamID), map[string]any{"memberUserIds": memberUserIDs}, &t)
}

func (c *Client) UpdateIMTier(ctx context.Context, teamID, tierID int64, patch map[string]any) error {
	return c.Do(ctx, http.MethodPatch, imPath("teams/%d/tiers/%d", teamID, tierID), patch, nil)
}

// ReorderIMTiers sets the order; tierIDs must be exactly the team's current tiers.
func (c *Client) ReorderIMTiers(ctx context.Context, teamID int64, tierIDs []int64) error {
	return c.Do(ctx, http.MethodPost, imPath("teams/%d/tiers/reorder", teamID), map[string]any{"tierIds": tierIDs}, nil)
}

func (c *Client) DeleteIMTier(ctx context.Context, teamID, tierID int64) error {
	return c.Do(ctx, http.MethodDelete, imPath("teams/%d/tiers/%d", teamID, tierID), nil, nil)
}

// IMSchedule is GET /api/im/schedules/{id} (no rotations: those are in the tier tree).
type IMSchedule struct {
	ID                  int64          `json:"id"`
	TeamID              int64          `json:"teamId"`
	TierID              int64          `json:"tierId"`
	Name                string         `json:"name"`
	Timezone            string         `json:"timezone"`
	EffectiveFrom       *string        `json:"effectiveFrom"`
	EffectiveUntil      *string        `json:"effectiveUntil"`
	WeeklySchedules     []WeeklyWindow `json:"weeklySchedules"`
	RotationMode        string         `json:"rotationMode"`
	RotationRepeats     *string        `json:"rotationRepeats"`
	CustomRepeatUnit    *string        `json:"customRepeatUnit"`
	CustomRepeatValue   *int64         `json:"customRepeatValue"`
	AutoRotationSize    *int64         `json:"autoRotationSize"`
	RoundRobinSize      int64          `json:"roundRobinSize"`
	StartsOnDayOfWeek   *int64         `json:"startsOnDayOfWeek"`
	StartsOnDateOfMonth *int64         `json:"startsOnDateOfMonth"`
	StartsOnTime        *string        `json:"startsOnTime"`
}

func (c *Client) GetIMSchedule(ctx context.Context, id int64) (*IMSchedule, error) {
	var s IMSchedule
	return &s, c.Do(ctx, http.MethodGet, imPath("schedules/%d", id), nil, &s)
}

// CreateIMTierSchedule uses the flat POST /api/im/tiers/{tierId}/schedules (full body incl. rotations).
func (c *Client) CreateIMTierSchedule(ctx context.Context, tierID int64, body map[string]any) (int64, error) {
	var out struct {
		ID int64 `json:"id"`
	}
	return out.ID, c.Do(ctx, http.MethodPost, imPath("tiers/%d/schedules", tierID), body, &out)
}

// UpdateIMSchedule patches; "rotations", when given, replaces all rotation groups.
func (c *Client) UpdateIMSchedule(ctx context.Context, id int64, patch map[string]any) error {
	return c.Do(ctx, http.MethodPatch, imPath("schedules/%d", id), patch, nil)
}

func (c *Client) DeleteIMSchedule(ctx context.Context, id int64) error {
	return c.Do(ctx, http.MethodDelete, imPath("schedules/%d", id), nil, nil)
}

// IMOverride is the read form of an override (GET .../overrides/{overrideId}).
type IMOverride struct {
	ID                  int64  `json:"id"`
	ScheduleID          *int64 `json:"scheduleId"`
	UserID              string `json:"userId"`
	Type                string `json:"type"`
	ApplyScope          string `json:"applyScope"`
	EscalationTierOrder *int64 `json:"escalationTierOrder"`
	StartsAt            string `json:"startsAt"`
	EndsAt              string `json:"endsAt"`
	CreatedAt           string `json:"createdAt"`
	CoveredBy           []struct {
		UserID string `json:"userId"`
	} `json:"coveredBy"`
}

// OverrideParent is "schedules/<id>" or "teams/<id>".
func OverrideParent(kind string, id int64) string { return fmt.Sprintf("%s/%d", kind, id) }

func (c *Client) GetIMOverride(ctx context.Context, parent string, id int64) (*IMOverride, error) {
	var o IMOverride
	return &o, c.Do(ctx, http.MethodGet, imPath("%s/overrides/%d", parent, id), nil, &o)
}

func (c *Client) CreateIMOverride(ctx context.Context, parent string, body map[string]any) (int64, error) {
	var out struct {
		ID int64 `json:"id"`
	}
	return out.ID, c.Do(ctx, http.MethodPost, imPath("%s/overrides", parent), body, &out)
}

// UpdateIMOverride: the PATCH body is the full override (userId, type, startsAt, endsAt required).
func (c *Client) UpdateIMOverride(ctx context.Context, parent string, id int64, body map[string]any) error {
	return c.Do(ctx, http.MethodPatch, imPath("%s/overrides/%d", parent, id), body, nil)
}

func (c *Client) DeleteIMOverride(ctx context.Context, parent string, id int64) error {
	return c.Do(ctx, http.MethodDelete, imPath("%s/overrides/%d", parent, id), nil, nil)
}

// User is one login of GET /api/users.
type User struct {
	ID       string  `json:"id"`
	Email    string  `json:"email"`
	Name     *string `json:"name"`
	Role     string  `json:"role"`
	IsActive bool    `json:"isActive"`
}

// ListUsers reads GET /api/users ({ users, limits }, no envelope form).
func (c *Client) ListUsers(ctx context.Context) ([]User, error) {
	var out struct {
		Users []User `json:"users"`
	}
	return out.Users, c.Do(ctx, http.MethodGet, "/api/users", nil, &out)
}

// FindUserByEmail matches case-insensitively.
func (c *Client) FindUserByEmail(ctx context.Context, email string) (*User, error) {
	users, err := c.ListUsers(ctx)
	if err != nil {
		return nil, err
	}
	for i := range users {
		if strings.EqualFold(users[i].Email, email) {
			return &users[i], nil
		}
	}
	return nil, &APIError{StatusCode: http.StatusNotFound, StatusMessage: "no visible user with email " + email}
}
