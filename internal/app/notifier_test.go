package app

import (
	"context"
	"sync"
	"testing"
	"time"

	"DomainC/cfclient"
	"DomainC/config"
	"DomainC/domain"
	"DomainC/telegram"

	cloudflare "github.com/cloudflare/cloudflare-go"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type fakeSender struct {
	mu       sync.Mutex
	messages []string
	buttons  []string
}

func (f *fakeSender) Send(ctx context.Context, msg string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.messages = append(f.messages, msg)
	return nil
}

func (f *fakeSender) SendWithButtons(ctx context.Context, msg string, buttons [][]telegram.Button) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.messages = append(f.messages, msg)
	for _, row := range buttons {
		for _, b := range row {
			f.buttons = append(f.buttons, b.CallbackData)
		}
	}
	return nil
}
func (f *fakeSender) SendDocumentPath(ctx context.Context, filepath string, caption string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.messages = append(f.messages, "DOC:"+filepath+"|"+caption)
	return nil
}
func (f *fakeSender) EditButtons(ctx context.Context, chatID int64, messageID int, buttons [][]telegram.Button) error {
	return nil
}
func (f *fakeSender) ClearButtons(ctx context.Context, chatID int64, messageID int) error {
	return nil
}
func (f *fakeSender) AnswerCallback(ctx context.Context, callbackID, text string) error {
	return nil
}
func (f *fakeSender) StartListener(ctx context.Context, handleCallback func(cb *tgbotapi.CallbackQuery), handleMessage func(msg *tgbotapi.Message)) error {
	<-ctx.Done()
	return nil
}

type fakeCF struct{ deleted []string }

func (f *fakeCF) FetchAllDomains(ctx context.Context, account config.CF) ([]cfclient.DomainInfo, error) {
	return nil, nil
}
func (f *fakeCF) ListDNSRecords(ctx context.Context, account config.CF, domain string) ([]cloudflare.DNSRecord, error) {
	return nil, nil
}
func (f *fakeCF) PauseDomain(ctx context.Context, account config.CF, domain string, pause bool) error {
	return nil
}
func (f *fakeCF) DeleteDomain(ctx context.Context, account config.CF, domain string) error {
	f.deleted = append(f.deleted, domain)
	return nil
}
func (f *fakeCF) GetZoneDetails(ctx context.Context, account config.CF, domain string) (cfclient.ZoneDetail, error) {
	return cfclient.ZoneDetail{}, nil
}
func (f *fakeCF) CreateZone(ctx context.Context, account config.CF, domain string) (cfclient.ZoneDetail, error) {
	return cfclient.ZoneDetail{}, nil
}
func (f *fakeCF) UpsertDNSRecord(ctx context.Context, account config.CF, domain string, params cfclient.DNSRecordParams) (cloudflare.DNSRecord, error) {
	return cloudflare.DNSRecord{}, nil
}
func (f *fakeCF) DeleteDNSRecord(ctx context.Context, account config.CF, domain string, recordName string) (int, error) {
	return 0, nil
}
func (f *fakeCF) ListZones(ctx context.Context, account config.CF) ([]cfclient.ZoneDetail, error) {
	return nil, nil
}
func (f *fakeCF) PurgeZoneCache(ctx context.Context, account config.CF, zoneID string) error {
	return nil
}

func (f *fakeCF) CreateOriginCertificate(ctx context.Context, account config.CF, hostnames []string) (cfclient.OriginCert, error) {
	return cfclient.OriginCert{}, nil
}
func (f *fakeCF) ListOriginCACertificates(ctx context.Context, account config.CF) ([]cfclient.OriginCACertInfo, error) {
	return nil, nil
}
func (f *fakeCF) ListCustomLists(ctx context.Context, account config.CF) ([]cloudflare.List, error) {
	return nil, nil
}
func (f *fakeCF) GetCustomList(ctx context.Context, account config.CF, listID string) (cloudflare.List, error) {
	return cloudflare.List{}, nil
}
func (f *fakeCF) ListCustomListItems(ctx context.Context, account config.CF, listID string) ([]cloudflare.ListItem, error) {
	return nil, nil
}
func (f *fakeCF) CreateCustomListItem(ctx context.Context, account config.CF, listID string, ip string, comment string) ([]cloudflare.ListItem, error) {
	return nil, nil
}
func (f *fakeCF) DeleteCustomListItem(ctx context.Context, account config.CF, listID string, itemID string) ([]cloudflare.ListItem, error) {
	return nil, nil
}

func TestNotifierSendsAlertsAndDeletes(t *testing.T) {
	sender := &fakeSender{}
	cf := &fakeCF{}
	notifier := &NotifierService{Sender: sender, CFClient: cf, DeleteTimeout: time.Second}

	expiry := time.Now().Add(48 * time.Hour).Format("2006-01-02")
	domains := []domain.DomainSource{{Domain: "example.com", Source: "acc", Expiry: expiry, IsCF: true}}

	cfg := config.CF{Label: "acc"}
	config.Cfg.CloudflareAccounts = []config.CF{cfg}

	if err := notifier.Notify(context.Background(), domains); err != nil {
		t.Fatalf("notify returned error: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	if len(sender.messages) == 0 {
		t.Fatalf("expected messages to be sent")
	}
	if len(cf.deleted) == 0 || cf.deleted[0] != "example.com" {
		t.Fatalf("expected domain deletion to be triggered")
	}
}
