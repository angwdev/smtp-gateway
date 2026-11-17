package apitoken

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	appaudit "github.com/angwdev/smtp-gateway/internal/application/audit"
	domain "github.com/angwdev/smtp-gateway/internal/domain/apitoken"
	"github.com/angwdev/smtp-gateway/internal/domain/shared"
)

type spyAuditor struct{ entries []appaudit.Entry }

//nolint:gocritic // hugeParam: e matches the Auditor interface's value API
func (s *spyAuditor) Record(_ context.Context, e appaudit.Entry) { s.entries = append(s.entries, e) }

var now = time.Unix(1_800_000_000, 0)

func setup(t *testing.T) (*Handler, *domain.MockRepository, *spyAuditor) {
	t.Helper()
	repo := domain.NewMockRepository(gomock.NewController(t))
	spy := &spyAuditor{}
	h := NewHandler(repo, spy)
	h.now = func() time.Time { return now }
	return h, repo, spy
}

func stored(t *testing.T) (tok *domain.Token, secret string) {
	t.Helper()
	tok, secret, err := domain.New("grafana", domain.ScopeRead, nil, "ana@example.com", now.Add(-time.Hour))
	require.NoError(t, err)
	tok.ID = 2
	return tok, secret
}

func TestCreate(t *testing.T) {
	h, repo, spy := setup(t)
	repo.EXPECT().Create(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, t *domain.Token) error {
		t.ID = 2
		return nil
	})
	tok, secret, err := h.Create(context.Background(), CreateCommand{Name: "grafana", Scope: domain.ScopeRead, CreatedBy: "ana@example.com"})
	require.NoError(t, err)
	assert.EqualValues(t, 2, tok.ID)
	assert.NotEmpty(t, secret)
	require.Len(t, spy.entries, 1)
	assert.Equal(t, "token.create", spy.entries[0].Action)
	assert.Equal(t, "grafana", spy.entries[0].TargetLabel)
	changes := spy.entries[0].Changes
	require.Contains(t, changes, "scope")
	assert.Equal(t, "read", changes["scope"].To)
	assert.NotContains(t, changes, "expires_at", "nil before and nil after is not a change")
}

func TestCreateWithExpiry(t *testing.T) {
	h, repo, spy := setup(t)
	expires := now.Add(24 * time.Hour)
	repo.EXPECT().Create(gomock.Any(), gomock.Any()).Return(nil)
	_, _, err := h.Create(context.Background(), CreateCommand{Name: "grafana", Scope: domain.ScopeWrite, ExpiresAt: &expires, CreatedBy: "ana@example.com"})
	require.NoError(t, err)
	require.Len(t, spy.entries, 1)
	changes := spy.entries[0].Changes
	require.Contains(t, changes, "scope")
	assert.Equal(t, "write", changes["scope"].To)
	require.Contains(t, changes, "expires_at")
	assert.Equal(t, expires.UTC().Format(time.RFC3339), changes["expires_at"].To)
}

func TestCreateInvalidOrDuplicateIsNotAudited(t *testing.T) {
	h, repo, spy := setup(t)
	_, _, err := h.Create(context.Background(), CreateCommand{Name: "", Scope: domain.ScopeRead})
	var ve *shared.ValidationError
	require.ErrorAs(t, err, &ve)

	repo.EXPECT().Create(gomock.Any(), gomock.Any()).Return(shared.ErrConflict)
	_, _, err = h.Create(context.Background(), CreateCommand{Name: "grafana", Scope: domain.ScopeRead})
	require.ErrorIs(t, err, shared.ErrConflict)
	assert.Empty(t, spy.entries)
}

func TestList(t *testing.T) {
	h, repo, _ := setup(t)
	tok, _ := stored(t)
	repo.EXPECT().List(gomock.Any()).Return([]*domain.Token{tok}, nil)
	list, err := h.List(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []*domain.Token{tok}, list)

	repo.EXPECT().List(gomock.Any()).Return(nil, assert.AnError)
	_, err = h.List(context.Background())
	require.ErrorIs(t, err, assert.AnError)
}

func TestRevoke(t *testing.T) {
	h, repo, spy := setup(t)
	tok, _ := stored(t)
	repo.EXPECT().List(gomock.Any()).Return([]*domain.Token{tok}, nil)
	repo.EXPECT().Revoke(gomock.Any(), int64(2), now).Return(nil)
	err := h.Revoke(context.Background(), 2)
	require.NoError(t, err)
	require.Len(t, spy.entries, 1)
	assert.Equal(t, "token.revoke", spy.entries[0].Action)
	assert.Equal(t, "grafana", spy.entries[0].TargetLabel)
}

func TestRevokeMissing(t *testing.T) {
	h, repo, spy := setup(t)
	repo.EXPECT().List(gomock.Any()).Return(nil, nil)
	err := h.Revoke(context.Background(), 99)
	require.ErrorIs(t, err, shared.ErrNotFound)
	assert.Empty(t, spy.entries)
}

func TestRevokeListError(t *testing.T) {
	h, repo, spy := setup(t)
	repo.EXPECT().List(gomock.Any()).Return(nil, assert.AnError)
	err := h.Revoke(context.Background(), 2)
	require.ErrorIs(t, err, assert.AnError)
	assert.Empty(t, spy.entries)
}

func TestRevokeRepositoryError(t *testing.T) {
	h, repo, spy := setup(t)
	tok, _ := stored(t)
	repo.EXPECT().List(gomock.Any()).Return([]*domain.Token{tok}, nil)
	repo.EXPECT().Revoke(gomock.Any(), int64(2), now).Return(assert.AnError)
	err := h.Revoke(context.Background(), 2)
	require.ErrorIs(t, err, assert.AnError)
	assert.Empty(t, spy.entries)
}

func TestAuthenticate(t *testing.T) {
	recent, old := now.Add(-10*time.Second), now.Add(-2*time.Minute)
	tests := []struct {
		name      string
		lastUsed  *time.Time
		lastIP    string
		wantTouch bool
	}{
		{"first use is recorded", nil, "", true},
		{"same address within a minute is not written again", &recent, "203.0.113.9", false},
		{"a new address is recorded at once", &recent, "198.51.100.1", true},
		{"after a minute it is recorded again", &old, "203.0.113.9", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, repo, _ := setup(t)
			tok, secret := stored(t)
			tok.LastUsedAt, tok.LastUsedIP = tt.lastUsed, tt.lastIP
			repo.EXPECT().FindByHash(gomock.Any(), shared.HashSecret(secret)).Return(tok, nil)
			if tt.wantTouch {
				repo.EXPECT().Touch(gomock.Any(), int64(2), "203.0.113.9", now).Return(nil)
			}
			got, err := h.Authenticate(context.Background(), secret, "203.0.113.9")
			require.NoError(t, err)
			assert.EqualValues(t, 2, got.ID)
		})
	}
}

func TestAuthenticateNoPrefix(t *testing.T) {
	h, repo, _ := setup(t)
	_, err := h.Authenticate(context.Background(), "not-a-token", "203.0.113.9")
	require.ErrorIs(t, err, shared.ErrUnauthorized)
	_ = repo // repo must not be touched
}

func TestAuthenticateUnknown(t *testing.T) {
	h, repo, _ := setup(t)
	raw := domain.Prefix + "unknown"
	repo.EXPECT().FindByHash(gomock.Any(), shared.HashSecret(raw)).Return(nil, shared.ErrNotFound)
	_, err := h.Authenticate(context.Background(), raw, "203.0.113.9")
	require.ErrorIs(t, err, shared.ErrUnauthorized)
}

func TestAuthenticateRevoked(t *testing.T) {
	h, repo, _ := setup(t)
	tok, secret := stored(t)
	revokedAt := now.Add(-time.Minute)
	tok.RevokedAt = &revokedAt
	repo.EXPECT().FindByHash(gomock.Any(), shared.HashSecret(secret)).Return(tok, nil)
	_, err := h.Authenticate(context.Background(), secret, "203.0.113.9")
	require.ErrorIs(t, err, shared.ErrUnauthorized)
}

func TestAuthenticateExpired(t *testing.T) {
	h, repo, _ := setup(t)
	tok, secret := stored(t)
	expiresAt := now.Add(-time.Minute)
	tok.ExpiresAt = &expiresAt
	repo.EXPECT().FindByHash(gomock.Any(), shared.HashSecret(secret)).Return(tok, nil)
	_, err := h.Authenticate(context.Background(), secret, "203.0.113.9")
	require.ErrorIs(t, err, shared.ErrUnauthorized)
}

func TestAuthenticateDatabaseErrorIsNotUnauthorized(t *testing.T) {
	h, repo, _ := setup(t)
	raw := domain.Prefix + "boom"
	boom := assert.AnError
	repo.EXPECT().FindByHash(gomock.Any(), shared.HashSecret(raw)).Return(nil, boom)
	_, err := h.Authenticate(context.Background(), raw, "203.0.113.9")
	require.ErrorIs(t, err, boom)
	assert.NotErrorIs(t, err, shared.ErrUnauthorized)
}

func TestAuthenticateTouchError(t *testing.T) {
	h, repo, _ := setup(t)
	tok, secret := stored(t)
	repo.EXPECT().FindByHash(gomock.Any(), shared.HashSecret(secret)).Return(tok, nil)
	repo.EXPECT().Touch(gomock.Any(), int64(2), "203.0.113.9", now).Return(assert.AnError)
	_, err := h.Authenticate(context.Background(), secret, "203.0.113.9")
	require.ErrorIs(t, err, assert.AnError)
}
