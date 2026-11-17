package credential

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	appaudit "github.com/angwdev/smtp-gateway/internal/application/audit"
	domain "github.com/angwdev/smtp-gateway/internal/domain/credential"
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

func stored(t *testing.T) (c *domain.Credential, secret string) {
	t.Helper()
	c, secret, err := domain.New("nextcloud", "files", nil, 0, true, "ana@example.com", now.Add(-time.Hour))
	require.NoError(t, err)
	c.ID = 3
	return c, secret
}

func TestCreate(t *testing.T) {
	h, repo, spy := setup(t)
	repo.EXPECT().Create(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, c *domain.Credential) error {
		c.ID = 3
		return nil
	})
	c, secret, err := h.Create(context.Background(), CreateCommand{Username: "nextcloud", Enabled: true, CreatedBy: "ana@example.com"})
	require.NoError(t, err)
	assert.EqualValues(t, 3, c.ID)
	assert.Len(t, secret, domain.SecretLength)
	require.Len(t, spy.entries, 1)
	assert.Equal(t, "credential.create", spy.entries[0].Action)
	assert.Equal(t, "nextcloud", spy.entries[0].TargetLabel)
	assert.NotContains(t, fmt.Sprint(spy.entries[0].Changes), secret, "the secret must never reach the audit log")
	assert.NotContains(t, fmt.Sprint(spy.entries[0].Changes), c.SecretHash)
}

func TestCreateInvalidOrDuplicateIsNotAudited(t *testing.T) {
	h, repo, spy := setup(t)
	_, _, err := h.Create(context.Background(), CreateCommand{Username: "bad name"})
	var ve *shared.ValidationError
	require.ErrorAs(t, err, &ve)

	repo.EXPECT().Create(gomock.Any(), gomock.Any()).Return(shared.ErrConflict)
	_, _, err = h.Create(context.Background(), CreateCommand{Username: "nextcloud"})
	require.ErrorIs(t, err, shared.ErrConflict)
	assert.Empty(t, spy.entries)
}

func TestUpdate(t *testing.T) {
	off, rate := false, 5
	tests := []struct {
		name       string
		cmd        UpdateCommand
		wantAction string
		wantKeys   []string
	}{
		{"only enabled changes: a disable event", UpdateCommand{ID: 3, Enabled: &off}, "credential.disable", []string{"enabled"}},
		{"other fields: an update event", UpdateCommand{ID: 3, RatePerHour: &rate}, "credential.update", []string{"rate_per_hour"}},
		{"both", UpdateCommand{ID: 3, RatePerHour: &rate, Enabled: &off}, "credential.update", []string{"enabled", "rate_per_hour"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, repo, spy := setup(t)
			c, _ := stored(t)
			repo.EXPECT().FindByID(gomock.Any(), int64(3)).Return(c, nil)
			repo.EXPECT().Update(gomock.Any(), gomock.Any()).Return(nil)
			got, err := h.Update(context.Background(), tt.cmd)
			require.NoError(t, err)
			assert.Equal(t, now, got.UpdatedAt)
			require.Len(t, spy.entries, 1)
			assert.Equal(t, tt.wantAction, spy.entries[0].Action)
			keys := make([]string, 0, len(spy.entries[0].Changes))
			for k := range spy.entries[0].Changes {
				keys = append(keys, k)
			}
			assert.ElementsMatch(t, tt.wantKeys, keys)
		})
	}
}

func TestUpdateEnableAction(t *testing.T) {
	h, repo, spy := setup(t)
	c, _ := stored(t)
	c.Enabled = false
	on := true
	repo.EXPECT().FindByID(gomock.Any(), int64(3)).Return(c, nil)
	repo.EXPECT().Update(gomock.Any(), gomock.Any()).Return(nil)
	_, err := h.Update(context.Background(), UpdateCommand{ID: 3, Enabled: &on})
	require.NoError(t, err)
	require.Len(t, spy.entries, 1)
	assert.Equal(t, "credential.enable", spy.entries[0].Action)
}

func TestUpdateWithoutChangesWritesNothing(t *testing.T) {
	h, repo, spy := setup(t)
	c, _ := stored(t)
	repo.EXPECT().FindByID(gomock.Any(), int64(3)).Return(c, nil)
	same := "files"
	_, err := h.Update(context.Background(), UpdateCommand{ID: 3, Description: &same})
	require.NoError(t, err)
	assert.Empty(t, spy.entries)
}

func TestUpdateMissingCredential(t *testing.T) {
	h, repo, spy := setup(t)
	repo.EXPECT().FindByID(gomock.Any(), int64(99)).Return(nil, shared.ErrNotFound)
	_, err := h.Update(context.Background(), UpdateCommand{ID: 99})
	require.ErrorIs(t, err, shared.ErrNotFound)
	assert.Empty(t, spy.entries)
}

func TestUpdateInvalidValueDoesNotWrite(t *testing.T) {
	h, repo, spy := setup(t)
	c, _ := stored(t)
	bad := -1
	repo.EXPECT().FindByID(gomock.Any(), int64(3)).Return(c, nil)
	_, err := h.Update(context.Background(), UpdateCommand{ID: 3, RatePerHour: &bad})
	var ve *shared.ValidationError
	require.ErrorAs(t, err, &ve)
	assert.Empty(t, spy.entries)
}

func TestUpdatePropagatesRepositoryError(t *testing.T) {
	h, repo, spy := setup(t)
	c, _ := stored(t)
	rate := 7
	repo.EXPECT().FindByID(gomock.Any(), int64(3)).Return(c, nil)
	repo.EXPECT().Update(gomock.Any(), gomock.Any()).Return(assert.AnError)
	_, err := h.Update(context.Background(), UpdateCommand{ID: 3, RatePerHour: &rate})
	require.ErrorIs(t, err, assert.AnError)
	assert.Empty(t, spy.entries)
}

func TestGet(t *testing.T) {
	h, repo, _ := setup(t)
	c, _ := stored(t)
	repo.EXPECT().FindByID(gomock.Any(), int64(3)).Return(c, nil)
	got, err := h.Get(context.Background(), 3)
	require.NoError(t, err)
	assert.Equal(t, c, got)

	repo.EXPECT().FindByID(gomock.Any(), int64(99)).Return(nil, shared.ErrNotFound)
	_, err = h.Get(context.Background(), 99)
	require.ErrorIs(t, err, shared.ErrNotFound)
}

func TestList(t *testing.T) {
	h, repo, _ := setup(t)
	c, _ := stored(t)
	repo.EXPECT().List(gomock.Any()).Return([]*domain.Credential{c}, nil)
	list, err := h.List(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []*domain.Credential{c}, list)

	repo.EXPECT().List(gomock.Any()).Return(nil, assert.AnError)
	_, err = h.List(context.Background())
	require.ErrorIs(t, err, assert.AnError)
}

func TestRotate(t *testing.T) {
	h, repo, spy := setup(t)
	c, oldSecret := stored(t)
	oldHash := c.SecretHash
	repo.EXPECT().FindByID(gomock.Any(), int64(3)).Return(c, nil)
	repo.EXPECT().SetSecretHash(gomock.Any(), int64(3), gomock.Any(), now).DoAndReturn(
		func(_ context.Context, _ int64, hash string, _ time.Time) error {
			assert.NotEqual(t, oldHash, hash)
			assert.Equal(t, c.SecretHash, hash)
			return nil
		})
	got, secret, err := h.Rotate(context.Background(), 3)
	require.NoError(t, err)
	assert.NotEqual(t, oldSecret, secret)
	assert.Len(t, secret, domain.SecretLength)
	assert.Equal(t, c, got)
	require.Len(t, spy.entries, 1)
	assert.Equal(t, "credential.rotate", spy.entries[0].Action)
	assert.Empty(t, spy.entries[0].Changes)
}

func TestRotateMissingCredential(t *testing.T) {
	h, repo, spy := setup(t)
	repo.EXPECT().FindByID(gomock.Any(), int64(99)).Return(nil, shared.ErrNotFound)
	_, _, err := h.Rotate(context.Background(), 99)
	require.ErrorIs(t, err, shared.ErrNotFound)
	assert.Empty(t, spy.entries)
}

func TestRotateSetSecretHashError(t *testing.T) {
	h, repo, spy := setup(t)
	c, _ := stored(t)
	repo.EXPECT().FindByID(gomock.Any(), int64(3)).Return(c, nil)
	repo.EXPECT().SetSecretHash(gomock.Any(), int64(3), gomock.Any(), now).Return(assert.AnError)
	_, _, err := h.Rotate(context.Background(), 3)
	require.ErrorIs(t, err, assert.AnError)
	assert.Empty(t, spy.entries)
}

func TestDelete(t *testing.T) {
	h, repo, spy := setup(t)
	c, _ := stored(t)
	repo.EXPECT().FindByID(gomock.Any(), int64(3)).Return(c, nil)
	repo.EXPECT().Delete(gomock.Any(), int64(3)).Return(nil)
	err := h.Delete(context.Background(), 3)
	require.NoError(t, err)
	require.Len(t, spy.entries, 1)
	assert.Equal(t, "credential.delete", spy.entries[0].Action)
	assert.Equal(t, "nextcloud", spy.entries[0].TargetLabel)
}

func TestDeleteRepositoryError(t *testing.T) {
	h, repo, spy := setup(t)
	c, _ := stored(t)
	repo.EXPECT().FindByID(gomock.Any(), int64(3)).Return(c, nil)
	repo.EXPECT().Delete(gomock.Any(), int64(3)).Return(assert.AnError)
	err := h.Delete(context.Background(), 3)
	require.ErrorIs(t, err, assert.AnError)
	assert.Empty(t, spy.entries)
}

func TestDeleteMissingCredential(t *testing.T) {
	h, repo, spy := setup(t)
	repo.EXPECT().FindByID(gomock.Any(), int64(99)).Return(nil, shared.ErrNotFound)
	err := h.Delete(context.Background(), 99)
	require.ErrorIs(t, err, shared.ErrNotFound)
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
			c, secret := stored(t)
			c.LastUsedAt, c.LastUsedIP = tt.lastUsed, tt.lastIP
			repo.EXPECT().FindByUsername(gomock.Any(), "nextcloud").Return(c, nil)
			if tt.wantTouch {
				repo.EXPECT().Touch(gomock.Any(), int64(3), "203.0.113.9", now).Return(nil)
			}
			got, err := h.Authenticate(context.Background(), AuthenticateCommand{Username: "nextcloud", Password: secret, IP: "203.0.113.9"})
			require.NoError(t, err)
			assert.EqualValues(t, 3, got.ID)
		})
	}
}

func TestAuthenticateFailures(t *testing.T) {
	h, repo, _ := setup(t)
	c, secret := stored(t)

	repo.EXPECT().FindByUsername(gomock.Any(), "ghost").Return(nil, shared.ErrNotFound)
	_, err := h.Authenticate(context.Background(), AuthenticateCommand{Username: "ghost", Password: "x"})
	require.ErrorIs(t, err, shared.ErrInvalidLogin)

	repo.EXPECT().FindByUsername(gomock.Any(), "nextcloud").Return(c, nil)
	_, err = h.Authenticate(context.Background(), AuthenticateCommand{Username: "nextcloud", Password: secret + "x"})
	require.ErrorIs(t, err, shared.ErrInvalidLogin)

	c.Enabled = false
	repo.EXPECT().FindByUsername(gomock.Any(), "nextcloud").Return(c, nil)
	_, err = h.Authenticate(context.Background(), AuthenticateCommand{Username: "nextcloud", Password: secret})
	require.ErrorIs(t, err, shared.ErrInvalidLogin)

	boom := assert.AnError
	repo.EXPECT().FindByUsername(gomock.Any(), "nextcloud").Return(nil, boom)
	_, err = h.Authenticate(context.Background(), AuthenticateCommand{Username: "nextcloud", Password: secret})
	require.ErrorIs(t, err, boom)
	assert.NotErrorIs(t, err, shared.ErrInvalidLogin, "a database failure is not a wrong password")
}

func TestAuthenticateTouchError(t *testing.T) {
	h, repo, _ := setup(t)
	c, secret := stored(t)
	repo.EXPECT().FindByUsername(gomock.Any(), "nextcloud").Return(c, nil)
	repo.EXPECT().Touch(gomock.Any(), int64(3), "203.0.113.9", now).Return(assert.AnError)
	_, err := h.Authenticate(context.Background(), AuthenticateCommand{Username: "nextcloud", Password: secret, IP: "203.0.113.9"})
	require.ErrorIs(t, err, assert.AnError)
}
