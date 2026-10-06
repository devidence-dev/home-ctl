// Package household manages member authentication, synchronization from configuration,
// onboarding lifecycle, and member status tracking.
package household

import (
	"context"
	"errors"
	"time"

	"github.com/px1/home-ctl/internal/config"
)

var (
	// ErrNotMember is returned when an interaction originates from an unconfigured user.
	ErrNotMember = errors.New("user is not an authorized household member")
	// ErrMemberInactive is returned when a user was deactivated.
	ErrMemberInactive = errors.New("user is inactive")
)

// Member represents a household member.
type Member struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Username string `json:"username"`
	Locale   string `json:"locale"`
	Active   bool   `json:"active"`
}

// OnboardingState tracks whether a member has initiated a private conversation with the bot.
type OnboardingState struct {
	UserID      int64      `json:"user_id"`
	Connected   bool       `json:"connected"`
	ConnectedAt *time.Time `json:"connected_at"`
}

// MemberStatus describes a member's profile and current connection status.
type MemberStatus struct {
	Member    Member `json:"member"`
	Connected bool   `json:"connected"`
}

// Store defines persistence requirements for household members and onboarding.
type Store interface {
	SyncMembers(ctx context.Context, users []config.UserConfig, defaultLocale string) error
	GetMember(ctx context.Context, id int64) (*Member, error)
	ListMembers(ctx context.Context, activeOnly bool) ([]Member, error)
	RecordOnboarding(ctx context.Context, userID int64, connectedAt time.Time) error
	RecordDisconnect(ctx context.Context, userID int64) error
	GetOnboardingState(ctx context.Context, userID int64) (*OnboardingState, error)
	GetStatusList(ctx context.Context) ([]MemberStatus, error)
}

// Service coordinates household member operations.
type Service struct {
	store Store
}

// NewService constructs a household Service.
func NewService(store Store) *Service {
	return &Service{store: store}
}

// SyncFromConfig synchronizes the configured member list into SQLite, activating new/updated
// members and deactivating removed ones while preserving past history references.
func (s *Service) SyncFromConfig(ctx context.Context, users []config.UserConfig, defaultLocale string) error {
	return s.store.SyncMembers(ctx, users, defaultLocale)
}

// Authorize checks whether a user ID is an active configured member.
func (s *Service) Authorize(ctx context.Context, userID int64) (*Member, error) {
	m, err := s.store.GetMember(ctx, userID)
	if err != nil {
		return nil, ErrNotMember
	}
	if !m.Active {
		return nil, ErrMemberInactive
	}
	return m, nil
}

// HandlePrivateStart records successful onboarding when a member sends /start in a private chat.
func (s *Service) HandlePrivateStart(ctx context.Context, userID int64, at time.Time) (*Member, error) {
	m, err := s.Authorize(ctx, userID)
	if err != nil {
		return nil, err
	}
	if err := s.store.RecordOnboarding(ctx, userID, at); err != nil {
		return nil, err
	}
	return m, nil
}

// HandleDisconnect marks a member as disconnected when the bot is blocked or messaging fails.
func (s *Service) HandleDisconnect(ctx context.Context, userID int64) error {
	return s.store.RecordDisconnect(ctx, userID)
}

// GetStatus returns the connection status for all active members.
func (s *Service) GetStatus(ctx context.Context) ([]MemberStatus, error) {
	return s.store.GetStatusList(ctx)
}

// IsConnected returns whether the specified member has an active private chat connection.
func (s *Service) IsConnected(ctx context.Context, userID int64) (bool, error) {
	st, err := s.store.GetOnboardingState(ctx, userID)
	if err != nil {
		return false, err
	}
	return st.Connected, nil
}

// ListActiveMembers returns all active household members.
func (s *Service) ListActiveMembers(ctx context.Context) ([]Member, error) {
	return s.store.ListMembers(ctx, true)
}
