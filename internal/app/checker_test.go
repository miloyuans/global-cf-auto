package app

import (
	"context"
	"testing"
	"time"

	"DomainC/domain"
)

type fakeWhois struct{ result string }

func (f fakeWhois) Query(ctx context.Context, domain string) (string, error) {
	return f.result, nil
}

type countingWhois struct{ calls int }

func (c *countingWhois) Query(ctx context.Context, domain string) (string, error) {
	c.calls++
	return "", nil
}

type fakeRepo struct{ saved []domain.DomainSource }

func (r *fakeRepo) LoadSources() ([]domain.DomainSource, error) { return nil, nil }

func (r *fakeRepo) SaveExpiring(domains []domain.DomainSource) error {
	r.saved = append(r.saved, domains...)
	return nil
}
func (r *fakeRepo) LoadExpiryCache() ([]domain.DomainSource, error) {
	return nil, nil
}
func (r *fakeRepo) SaveExpiryCache(domains []domain.DomainSource) error {
	return nil
}
func (r *fakeRepo) SaveFailures([]domain.FailureRecord) error { return nil }

func TestExpiryCheckerFiltersByAlertWindow(t *testing.T) {
	expiry := time.Now().Add(24 * time.Hour).Format("2006-01-02")
	checker := &ExpiryCheckerService{
		Whois:       fakeWhois{result: "Expiration Date: " + expiry},
		Repo:        &fakeRepo{},
		AlertWithin: 48 * time.Hour,
	}

	domains := []domain.DomainSource{{Domain: "example.com", Source: "test"}}
	got, failures, err := checker.Check(context.Background(), domains)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(failures) != 0 {
		t.Fatalf("expected 0 failures, got %d", len(failures))
	}

	if len(got) != 1 {
		t.Fatalf("expected 1 domain, got %d", len(got))
	}
	if got[0].Expiry != expiry {
		t.Fatalf("unexpected expiry %s", got[0].Expiry)
	}
}

func TestExpiryCheckerReturnsFailures(t *testing.T) {
	checker := &ExpiryCheckerService{
		Whois:       fakeWhois{result: "NOTICE: The expiration date displayed in this record is the date the"},
		Repo:        &fakeRepo{},
		AlertWithin: 48 * time.Hour,
	}

	domains := []domain.DomainSource{{Domain: "nodate.com", Source: "test"}}
	expiring, failures, err := checker.Check(context.Background(), domains)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(expiring) != 0 {
		t.Fatalf("expected no expiring domains, got %d", len(expiring))
	}
	if len(failures) != 1 {
		t.Fatalf("expected 1 failure, got %d", len(failures))
	}
	if failures[0].Domain != "nodate.com" {
		t.Fatalf("unexpected domain in failure: %s", failures[0].Domain)
	}
}
func TestExpiryCheckerUsesProvidedExpiry(t *testing.T) {
	expiry := time.Now().Add(24 * time.Hour).Format("2006-01-02")
	repo := &fakeRepo{}
	whois := &countingWhois{}
	checker := &ExpiryCheckerService{
		Whois:       whois,
		Repo:        repo,
		AlertWithin: 48 * time.Hour,
	}

	domains := []domain.DomainSource{{Domain: "prefilled.com", Source: "file", Expiry: expiry}}
	got, failures, err := checker.Check(context.Background(), domains)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(failures) != 0 {
		t.Fatalf("expected no failures, got %d", len(failures))
	}

	if len(got) != 1 || got[0].Domain != "prefilled.com" {
		t.Fatalf("expected domain from provided expiry to be returned")
	}

	if whois.calls != 0 {
		t.Fatalf("expected whois not to be called when expiry is provided")
	}
}
