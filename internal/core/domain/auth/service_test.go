package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/MadManJJ/room-reservation-api/internal/core/domain/user"
)

type fakeRepository struct {
	u               user.User
	admin           user.User
	resolveEmail    string
	resolveStudent  string
	resolveFirst    string
	resolveLast     string
	resolveCalls    int
	createAdminCall int
	err             error
}

func (r *fakeRepository) ResolveGoogleUser(_ context.Context, email, student, first, last string) (user.User, error) {
	r.resolveCalls++
	r.resolveEmail, r.resolveStudent, r.resolveFirst, r.resolveLast = email, student, first, last
	return r.u, r.err
}
func (r *fakeRepository) CreateAdmin(_ context.Context, email string) (user.User, error) {
	r.createAdminCall++
	if r.err != nil {
		return user.User{}, r.err
	}
	r.admin = user.User{ID: "admin-id", Email: email, Role: user.RoleAdmin, Active: true}
	return r.admin, nil
}
func (r *fakeRepository) FindAdmin(context.Context) (user.User, error) {
	if r.admin.ID == "" {
		return user.User{}, ErrNotFound
	}
	return r.admin, nil
}
func (r *fakeRepository) FindUserByID(context.Context, string) (user.User, error) {
	return r.u, r.err
}

type fakeTokens struct{}

func (fakeTokens) Issue(string) (string, error)  { return "app-jwt", nil }
func (fakeTokens) Verify(string) (string, error) { return "user-id", nil }

func TestEligibleGoogleEmail(t *testing.T) {
	for _, tc := range []struct {
		name, email, hd string
		verified        bool
		want            bool
	}{
		{"numeric student", "6500000021@student.chula.ac.th", "student.chula.ac.th", true, true},
		{"non numeric", "john21@student.chula.ac.th", "student.chula.ac.th", true, true},
		{"case insensitive domain", "john21@STUDENT.CHULA.AC.TH", "STUDENT.CHULA.AC.TH", true, true},
		{"starts with 21", "2100000000@student.chula.ac.th", "student.chula.ac.th", true, false},
		{"wrong suffix", "6500000022@student.chula.ac.th", "student.chula.ac.th", true, false},
		{"gmail", "john21@gmail.com", "student.chula.ac.th", true, false},
		{"unverified", "john21@student.chula.ac.th", "student.chula.ac.th", false, false},
		{"missing email", "", "student.chula.ac.th", true, false},
		{"missing hd", "john21@student.chula.ac.th", "", true, false},
		{"wrong hd", "john21@student.chula.ac.th", "chula.ac.th", true, false},
		{"invalid email", "john21@@student.chula.ac.th", "student.chula.ac.th", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := EligibleGoogleEmail(tc.email, tc.hd, tc.verified, "student.chula.ac.th", "21"); got != tc.want {
				t.Fatalf("eligible = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestSignInGoogle(t *testing.T) {
	repo := &fakeRepository{u: user.User{ID: "user-id", Email: "6500000021@student.chula.ac.th", Role: user.RoleUserA, Active: true}}
	s := New(repo, fakeTokens{}, "student.chula.ac.th", "21")
	identity := GoogleIdentity{Subject: "google-sub", Email: "6500000021@STUDENT.CHULA.AC.TH", EmailVerified: true, HostedDomain: "student.chula.ac.th", FirstName: " First ", LastName: " Last "}
	result, err := s.SignInGoogle(context.Background(), identity)
	if err != nil || result.AccessToken != "app-jwt" || result.User.ID != "user-id" || repo.resolveStudent != "6500000021" || repo.resolveEmail != "6500000021@student.chula.ac.th" || repo.resolveFirst != "First" || repo.resolveLast != "Last" {
		t.Fatalf("sign in: result=%+v repo=%+v err=%v", result, repo, err)
	}
	identity.Email = "john21@student.chula.ac.th"
	if _, err := s.SignInGoogle(context.Background(), identity); err != nil || repo.resolveStudent != "" {
		t.Fatalf("nonnumeric local part: %q %v", repo.resolveStudent, err)
	}
	identity.EmailVerified = false
	if _, err := s.SignInGoogle(context.Background(), identity); !errors.Is(err, ErrIneligible) || repo.resolveCalls != 2 {
		t.Fatalf("ineligible account reached repository: %v", err)
	}
	repo.u.Active = false
	identity.EmailVerified = true
	if _, err := s.SignInGoogle(context.Background(), identity); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("disabled user accepted: %v", err)
	}
}

func TestBootstrapAdmin(t *testing.T) {
	repo := &fakeRepository{}
	ctx := context.Background()
	if _, err := BootstrapAdmin(ctx, repo, "admin@gmail.com", "student.chula.ac.th", "21"); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("ineligible admin email: %v", err)
	}
	created, err := BootstrapAdmin(ctx, repo, "6500000021@student.chula.ac.th", "student.chula.ac.th", "21")
	if err != nil || !created || repo.admin.Role != user.RoleAdmin || repo.createAdminCall != 1 {
		t.Fatalf("admin not created: %+v %v", repo, err)
	}
	created, err = BootstrapAdmin(ctx, repo, "", "student.chula.ac.th", "21")
	if err != nil || created || repo.createAdminCall != 1 {
		t.Fatalf("second admin created: %+v %v", repo, err)
	}
}

func TestParseOSStudentIDs(t *testing.T) {
	ids, err := ParseOSStudentIDs("6730000021, 6720000021,6730000021", "21")
	if err != nil || len(ids) != 2 || ids[0] != "6730000021" || ids[1] != "6720000021" {
		t.Fatalf("parse: %v %v", ids, err)
	}
	if ids, err = ParseOSStudentIDs("", "21"); err != nil || len(ids) != 0 {
		t.Fatalf("empty: %v %v", ids, err)
	}
	for _, raw := range []string{"john21", "6730000022", "6730000021,", "6730000021,,6720000021"} {
		if _, err := ParseOSStudentIDs(raw, "21"); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("accepted %q", raw)
		}
	}
}

type fakeOSSeeder struct {
	ids []string
	err error
}

func (r *fakeOSSeeder) SeedOSUsers(_ context.Context, ids []string) (int, error) {
	r.ids = ids
	return len(ids), r.err
}

func TestBootstrapOS(t *testing.T) {
	ids, err := ParseOSStudentIDs("6730000021, 6720000021,6730000021", "21")
	if err != nil {
		t.Fatal(err)
	}
	repo := &fakeOSSeeder{}
	changed, err := BootstrapOS(context.Background(), repo, ids)
	if err != nil || changed != 2 || len(repo.ids) != 2 {
		t.Fatalf("bootstrap OS: changed=%d ids=%v err=%v", changed, repo.ids, err)
	}
	if changed, err := BootstrapOS(context.Background(), nil, nil); err != nil || changed != 0 {
		t.Fatalf("empty bootstrap: changed=%d err=%v", changed, err)
	}
	repo.err = ErrIdentityConflict
	if _, err := BootstrapOS(context.Background(), repo, ids); !errors.Is(err, ErrIdentityConflict) {
		t.Fatalf("repository error: %v", err)
	}
}
