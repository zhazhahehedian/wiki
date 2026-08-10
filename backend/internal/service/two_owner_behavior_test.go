package service

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/zenith-wang/it-wiki/backend/internal/domain"
	"github.com/zenith-wang/it-wiki/backend/internal/domain/ports"
	"github.com/zenith-wang/it-wiki/backend/internal/repo/generated"
)

func twoOwnerUUID(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: id, Valid: true}
}

type twoOwnerKBRepo struct {
	kbs map[uuid.UUID]generated.KnowledgeBase
}

func newTwoOwnerKBRepo(ownerA, ownerB, kbA, kbB uuid.UUID) *twoOwnerKBRepo {
	return &twoOwnerKBRepo{kbs: map[uuid.UUID]generated.KnowledgeBase{
		kbA: {ID: kbA, Name: "A", Settings: []byte("{}"), OwnerUserID: twoOwnerUUID(ownerA)},
		kbB: {ID: kbB, Name: "B", Settings: []byte("{}"), OwnerUserID: twoOwnerUUID(ownerB)},
	}}
}

func (r *twoOwnerKBRepo) CreateKnowledgeBaseForOwner(_ context.Context, arg generated.CreateKnowledgeBaseForOwnerParams) (generated.KnowledgeBase, error) {
	row := generated.KnowledgeBase{ID: uuid.New(), Name: arg.Name, Description: arg.Description, EmbedModel: arg.EmbedModel, EmbedDim: arg.EmbedDim, Settings: arg.Settings, OwnerUserID: arg.OwnerUserID}
	r.kbs[row.ID] = row
	return row, nil
}

func (r *twoOwnerKBRepo) GetKnowledgeBaseForOwner(_ context.Context, arg generated.GetKnowledgeBaseForOwnerParams) (generated.KnowledgeBase, error) {
	row, ok := r.kbs[arg.ID]
	if !ok || row.OwnerUserID != arg.OwnerUserID {
		return generated.KnowledgeBase{}, pgx.ErrNoRows
	}
	return row, nil
}

func (r *twoOwnerKBRepo) ListKnowledgeBasesForOwner(_ context.Context, arg generated.ListKnowledgeBasesForOwnerParams) ([]generated.KnowledgeBase, error) {
	var rows []generated.KnowledgeBase
	for _, row := range r.kbs {
		if row.OwnerUserID == arg.OwnerUserID {
			rows = append(rows, row)
		}
	}
	return rows, nil
}

func (r *twoOwnerKBRepo) CountKnowledgeBasesForOwner(_ context.Context, owner pgtype.UUID) (int64, error) {
	rows, _ := r.ListKnowledgeBasesForOwner(context.Background(), generated.ListKnowledgeBasesForOwnerParams{OwnerUserID: owner})
	return int64(len(rows)), nil
}

func (r *twoOwnerKBRepo) DeleteKnowledgeBaseForOwner(_ context.Context, arg generated.DeleteKnowledgeBaseForOwnerParams) error {
	row, ok := r.kbs[arg.ID]
	if !ok || row.OwnerUserID != arg.OwnerUserID {
		return pgx.ErrNoRows
	}
	delete(r.kbs, arg.ID)
	return nil
}

func TestTwoOwnerKnowledgeBaseGetListAndDeleteBehavior(t *testing.T) {
	ownerA, ownerB, kbA, kbB := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	repo := newTwoOwnerKBRepo(ownerA, ownerB, kbA, kbB)
	svc := NewKB(repo, "embed", 3)

	items, total, err := svc.List(context.Background(), ownerA.String(), 20, 0)
	if err != nil || total != 1 || len(items) != 1 || items[0].ID != kbA.String() {
		t.Fatalf("owner A list items=%#v total=%d err=%v", items, total, err)
	}
	if _, err := svc.Get(context.Background(), ownerB.String(), kbA.String()); !isKBNotFound(err) {
		t.Fatalf("owner B Get(A) error=%v", err)
	}
	if err := svc.Delete(context.Background(), ownerB.String(), kbA.String()); !isKBNotFound(err) {
		t.Fatalf("owner B Delete(A) error=%v", err)
	}
	if _, ok := repo.kbs[kbA]; !ok {
		t.Fatal("foreign delete removed owner A knowledge base")
	}
	if err := svc.Delete(context.Background(), ownerA.String(), kbA.String()); err != nil {
		t.Fatal(err)
	}
	if _, ok := repo.kbs[kbA]; ok {
		t.Fatal("owner delete did not remove knowledge base")
	}
}

func isKBNotFound(err error) bool {
	var target *ErrKBNotFound
	return errors.As(err, &target)
}

type twoOwnerDocumentRepo struct {
	*twoOwnerKBRepo
	docs map[uuid.UUID]generated.Document
}

func (r *twoOwnerDocumentRepo) GetDocumentForOwner(_ context.Context, arg generated.GetDocumentForOwnerParams) (generated.Document, error) {
	row, ok := r.docs[arg.ID]
	if !ok || !r.ownerOwnsKB(arg.OwnerUserID, row.KbID) {
		return generated.Document{}, pgx.ErrNoRows
	}
	return row, nil
}

func (r *twoOwnerDocumentRepo) ListDocumentsByKBForOwner(_ context.Context, arg generated.ListDocumentsByKBForOwnerParams) ([]generated.Document, error) {
	if !r.ownerOwnsKB(arg.OwnerUserID, arg.KbID) {
		return nil, pgx.ErrNoRows
	}
	var rows []generated.Document
	for _, row := range r.docs {
		if row.KbID == arg.KbID && (arg.Status == nil || row.Status == *arg.Status) {
			rows = append(rows, row)
		}
	}
	return rows, nil
}

func (r *twoOwnerDocumentRepo) CountDocumentsByKBForOwner(ctx context.Context, arg generated.CountDocumentsByKBForOwnerParams) (int64, error) {
	rows, err := r.ListDocumentsByKBForOwner(ctx, generated.ListDocumentsByKBForOwnerParams{KbID: arg.KbID, OwnerUserID: arg.OwnerUserID, Status: arg.Status})
	return int64(len(rows)), err
}

func (r *twoOwnerDocumentRepo) DeleteDocumentForOwner(_ context.Context, arg generated.DeleteDocumentForOwnerParams) (generated.DeleteDocumentForOwnerRow, error) {
	row, ok := r.docs[arg.ID]
	if !ok || !r.ownerOwnsKB(arg.OwnerUserID, row.KbID) {
		return generated.DeleteDocumentForOwnerRow{}, pgx.ErrNoRows
	}
	delete(r.docs, arg.ID)
	return generated.DeleteDocumentForOwnerRow{ContentRef: row.ContentRef, PendingContentRef: row.PendingContentRef}, nil
}

func (r *twoOwnerDocumentRepo) FindDocumentByChecksumForOwner(_ context.Context, arg generated.FindDocumentByChecksumForOwnerParams) (generated.Document, error) {
	if !r.ownerOwnsKB(arg.OwnerUserID, arg.KbID) {
		return generated.Document{}, pgx.ErrNoRows
	}
	for _, row := range r.docs {
		if row.KbID == arg.KbID && row.Checksum == arg.Checksum && row.SourceType == "local-upload" {
			return row, nil
		}
	}
	return generated.Document{}, pgx.ErrNoRows
}

func (r *twoOwnerDocumentRepo) CreateDocumentForOwner(_ context.Context, arg generated.CreateDocumentForOwnerParams) (generated.Document, error) {
	if !r.ownerOwnsKB(arg.OwnerUserID, arg.KbID) {
		return generated.Document{}, pgx.ErrNoRows
	}
	row := generated.Document{ID: uuid.New(), KbID: arg.KbID, SourceType: arg.SourceType, SourceRef: arg.SourceRef, Title: arg.Title, MimeType: arg.MimeType, Bytes: arg.Bytes, Checksum: arg.Checksum, Status: arg.Status, Metadata: arg.Metadata}
	r.docs[row.ID] = row
	return row, nil
}

func (r *twoOwnerDocumentRepo) UpdateDocumentStatusForOwner(_ context.Context, arg generated.UpdateDocumentStatusForOwnerParams) error {
	row, ok := r.docs[arg.ID]
	if !ok || !r.ownerOwnsKB(arg.OwnerUserID, row.KbID) {
		return pgx.ErrNoRows
	}
	row.Status, row.ErrorMessage = arg.Status, arg.ErrorMessage
	r.docs[arg.ID] = row
	return nil
}

func (r *twoOwnerDocumentRepo) ownerOwnsKB(owner pgtype.UUID, kbID uuid.UUID) bool {
	kb, ok := r.kbs[kbID]
	return ok && kb.OwnerUserID == owner
}

type twoOwnerStorage struct{ puts []string }

func (s *twoOwnerStorage) Put(_ context.Context, key string, _ io.Reader, _ int64, _ string) error {
	s.puts = append(s.puts, key)
	return nil
}
func (*twoOwnerStorage) Get(context.Context, string) (io.ReadCloser, error) { return nil, nil }
func (*twoOwnerStorage) Delete(context.Context, string) error               { return nil }

type twoOwnerIngestionQueue struct{ ids []string }

func (q *twoOwnerIngestionQueue) EnqueueIngestion(_ context.Context, id string) error {
	q.ids = append(q.ids, id)
	return nil
}

func TestTwoOwnerDocumentUploadAndReingestBehavior(t *testing.T) {
	ownerA, ownerB, kbA, kbB := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	docA, docB := uuid.New(), uuid.New()
	repo := &twoOwnerDocumentRepo{
		twoOwnerKBRepo: newTwoOwnerKBRepo(ownerA, ownerB, kbA, kbB),
		docs: map[uuid.UUID]generated.Document{
			docA: {ID: docA, KbID: kbA, SourceType: "local-upload", Status: string(domain.StatusReady), Metadata: []byte("{}")},
			docB: {ID: docB, KbID: kbB, SourceType: "local-upload", Status: string(domain.StatusReady), Metadata: []byte("{}")},
		},
	}
	storage, queue := &twoOwnerStorage{}, &twoOwnerIngestionQueue{}
	docs := NewDocument(repo, storage)
	ingestion := NewIngestion(repo, storage, queue)

	items, total, err := docs.ListByKB(context.Background(), ownerA.String(), kbA.String(), nil, 20, 0)
	if err != nil || total != 1 || len(items) != 1 || items[0].ID != docA.String() {
		t.Fatalf("owner A docs=%#v total=%d err=%v", items, total, err)
	}
	if _, err := docs.Get(context.Background(), ownerB.String(), docA.String()); !isDocNotFound(err) {
		t.Fatalf("owner B Get(A doc) error=%v", err)
	}
	if err := docs.Delete(context.Background(), ownerB.String(), docA.String()); !isDocNotFound(err) {
		t.Fatalf("owner B Delete(A doc) error=%v", err)
	}
	if _, ok := repo.docs[docA]; !ok {
		t.Fatal("foreign delete removed owner A document")
	}

	if _, err := ingestion.Upload(context.Background(), ownerB.String(), UploadInput{KBID: kbA.String(), Title: "foreign.txt", Body: strings.NewReader("secret"), Size: 6}); !isKBNotFound(err) {
		t.Fatalf("owner B Upload(A KB) error=%v", err)
	}
	if len(storage.puts) != 0 {
		t.Fatalf("foreign upload wrote storage: %#v", storage.puts)
	}
	if _, err := ingestion.Reingest(context.Background(), ownerB.String(), docA.String()); !isDocNotFound(err) {
		t.Fatalf("owner B Reingest(A doc) error=%v", err)
	}
	if len(queue.ids) != 0 {
		t.Fatalf("foreign reingest enqueued work: %#v", queue.ids)
	}

	uploaded, err := ingestion.Upload(context.Background(), ownerA.String(), UploadInput{KBID: kbA.String(), Title: "owned.txt", Body: strings.NewReader("owned"), Size: 5})
	if err != nil || uploaded.KBID != kbA.String() || len(storage.puts) != 1 || len(queue.ids) != 1 {
		t.Fatalf("owned upload doc=%#v puts=%#v queue=%#v err=%v", uploaded, storage.puts, queue.ids, err)
	}
	if _, err := ingestion.Reingest(context.Background(), ownerA.String(), docA.String()); err != nil || len(queue.ids) != 2 {
		t.Fatalf("owned reingest queue=%#v err=%v", queue.ids, err)
	}
}

func isDocNotFound(err error) bool {
	var target *ErrDocNotFound
	return errors.As(err, &target)
}

type twoOwnerChatRepo struct {
	kbs      map[uuid.UUID]pgtype.UUID
	convs    map[uuid.UUID]generated.Conversation
	messages map[uuid.UUID][]generated.Message
}

func (r *twoOwnerChatRepo) GetKnowledgeBaseForOwner(_ context.Context, arg generated.GetKnowledgeBaseForOwnerParams) (generated.KnowledgeBase, error) {
	owner, ok := r.kbs[arg.ID]
	if !ok || owner != arg.OwnerUserID {
		return generated.KnowledgeBase{}, pgx.ErrNoRows
	}
	return generated.KnowledgeBase{ID: arg.ID, OwnerUserID: owner, Settings: []byte("{}")}, nil
}

func (r *twoOwnerChatRepo) CreateConversationForOwner(_ context.Context, arg generated.CreateConversationForOwnerParams) (generated.Conversation, error) {
	if owner, ok := r.kbs[arg.KbID]; !ok || owner != arg.OwnerUserID {
		return generated.Conversation{}, pgx.ErrNoRows
	}
	row := generated.Conversation{ID: uuid.New(), KbID: arg.KbID, Title: arg.Title, Mode: arg.Mode, UserID: arg.OwnerUserID.String(), OwnerUserID: arg.OwnerUserID, AgentID: arg.AgentID, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	r.convs[row.ID] = row
	return row, nil
}

func (r *twoOwnerChatRepo) GetConversationForOwner(_ context.Context, arg generated.GetConversationForOwnerParams) (generated.Conversation, error) {
	row, ok := r.convs[arg.ID]
	if !ok || row.OwnerUserID != arg.OwnerUserID {
		return generated.Conversation{}, pgx.ErrNoRows
	}
	return row, nil
}

func (r *twoOwnerChatRepo) ListConversationsByKBForOwner(_ context.Context, arg generated.ListConversationsByKBForOwnerParams) ([]generated.Conversation, error) {
	var rows []generated.Conversation
	for _, row := range r.convs {
		if row.KbID == arg.KbID && row.OwnerUserID == arg.OwnerUserID {
			rows = append(rows, row)
		}
	}
	return rows, nil
}

func (r *twoOwnerChatRepo) CountConversationsByKBForOwner(ctx context.Context, arg generated.CountConversationsByKBForOwnerParams) (int64, error) {
	rows, err := r.ListConversationsByKBForOwner(ctx, generated.ListConversationsByKBForOwnerParams{KbID: arg.KbID, OwnerUserID: arg.OwnerUserID})
	return int64(len(rows)), err
}

func (r *twoOwnerChatRepo) CreateMessageForOwner(_ context.Context, arg generated.CreateMessageForOwnerParams) (generated.Message, error) {
	conv, ok := r.convs[arg.ConversationID]
	if !ok || conv.OwnerUserID != arg.OwnerUserID {
		return generated.Message{}, pgx.ErrNoRows
	}
	row := generated.Message{ID: uuid.New(), ConversationID: arg.ConversationID, Role: arg.Role, Content: arg.Content, Citations: arg.Citations, ToolCalls: arg.ToolCalls, TokenUsage: arg.TokenUsage, CreatedAt: time.Now()}
	r.messages[arg.ConversationID] = append(r.messages[arg.ConversationID], row)
	return row, nil
}

func (r *twoOwnerChatRepo) ListMessagesByConversationForOwner(_ context.Context, arg generated.ListMessagesByConversationForOwnerParams) ([]generated.Message, error) {
	conv, ok := r.convs[arg.ConversationID]
	if !ok || conv.OwnerUserID != arg.OwnerUserID {
		return nil, pgx.ErrNoRows
	}
	return append([]generated.Message(nil), r.messages[arg.ConversationID]...), nil
}

func (r *twoOwnerChatRepo) CountMessagesByConversationForOwner(_ context.Context, arg generated.CountMessagesByConversationForOwnerParams) (int64, error) {
	conv, ok := r.convs[arg.ConversationID]
	if !ok || conv.OwnerUserID != arg.OwnerUserID {
		return 0, pgx.ErrNoRows
	}
	return int64(len(r.messages[arg.ConversationID])), nil
}

func (r *twoOwnerChatRepo) ListRecentMessagesByConversationForOwner(_ context.Context, arg generated.ListRecentMessagesByConversationForOwnerParams) ([]generated.Message, error) {
	conv, ok := r.convs[arg.ConversationID]
	if !ok || conv.OwnerUserID != arg.OwnerUserID {
		return nil, pgx.ErrNoRows
	}
	return append([]generated.Message(nil), r.messages[arg.ConversationID]...), nil
}

func (r *twoOwnerChatRepo) TouchConversationForOwner(_ context.Context, arg generated.TouchConversationForOwnerParams) error {
	conv, ok := r.convs[arg.ID]
	if !ok || conv.OwnerUserID != arg.OwnerUserID {
		return pgx.ErrNoRows
	}
	return nil
}

func (r *twoOwnerChatRepo) UpdateConversationModeForOwner(_ context.Context, arg generated.UpdateConversationModeForOwnerParams) (generated.Conversation, error) {
	conv, ok := r.convs[arg.ID]
	if !ok || conv.OwnerUserID != arg.OwnerUserID {
		return generated.Conversation{}, pgx.ErrNoRows
	}
	conv.Mode = arg.Mode
	r.convs[arg.ID] = conv
	return conv, nil
}

func TestTwoOwnerChatMessageAndStreamBehavior(t *testing.T) {
	ownerA, ownerB, kbA, kbB, convA := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	repo := &twoOwnerChatRepo{
		kbs: map[uuid.UUID]pgtype.UUID{kbA: twoOwnerUUID(ownerA), kbB: twoOwnerUUID(ownerB)},
		convs: map[uuid.UUID]generated.Conversation{
			convA: {ID: convA, KbID: kbA, Mode: domain.ConversationModeRAG, OwnerUserID: twoOwnerUUID(ownerA), AgentID: ports.DefaultAgentID},
		},
		messages: map[uuid.UUID][]generated.Message{
			convA: {{ID: uuid.New(), ConversationID: convA, Role: domain.RoleUser, Content: "owner A", Citations: []byte("[]"), ToolCalls: []byte("[]"), TokenUsage: []byte("{}")}},
		},
	}
	svc := NewChat(repo, nil, nil, "", 0, nil, nil)

	if _, err := svc.CreateConversation(context.Background(), ownerB.String(), kbA.String(), ""); !isKBNotFound(err) {
		t.Fatalf("owner B CreateConversation(A KB) error=%v", err)
	}
	if _, _, err := svc.ListConversations(context.Background(), ownerB.String(), kbA.String(), 20, 0); !isKBNotFound(err) {
		t.Fatalf("owner B ListConversations(A KB) error=%v", err)
	}
	if _, _, err := svc.ListMessages(context.Background(), ownerB.String(), convA.String(), 20, 0); !isConversationNotFound(err) {
		t.Fatalf("owner B ListMessages(A conversation) error=%v", err)
	}
	sink := &recordingSink{events: &eventLog{}}
	before := len(repo.messages[convA])
	if err := svc.AskStream(context.Background(), ownerB.String(), convA.String(), "steal context", sink); !isConversationNotFound(err) {
		t.Fatalf("owner B AskStream(A conversation) error=%v", err)
	}
	if sink.Started() || len(repo.messages[convA]) != before {
		t.Fatalf("foreign stream started=%v messages=%#v", sink.Started(), repo.messages[convA])
	}
	items, total, err := svc.ListMessages(context.Background(), ownerA.String(), convA.String(), 20, 0)
	if err != nil || total != 1 || len(items) != 1 || items[0].Content != "owner A" {
		t.Fatalf("owner A messages=%#v total=%d err=%v", items, total, err)
	}
}

func isConversationNotFound(err error) bool {
	var target *ErrConversationNotFound
	return errors.As(err, &target)
}

type staticSourceResolver struct{ ref domain.ResourceRef }

func (r staticSourceResolver) Resolve(string) (domain.ResourceRef, error) { return r.ref, nil }

type twoOwnerFeishuRepo struct {
	kbOwners      map[uuid.UUID]uuid.UUID
	accountOwners map[uuid.UUID]uuid.UUID
	docs          map[uuid.UUID]generated.Document
}

func (r *twoOwnerFeishuRepo) CreateFeishuDocumentAndEnqueue(ctx context.Context, input CreateFeishuDocumentInput, enqueuer FeishuSyncEnqueuer) (generated.Document, error) {
	if r.kbOwners[input.KBID] != input.OwnerUserID || r.accountOwners[input.OAuthAccountID] != input.OwnerUserID {
		return generated.Document{}, pgx.ErrNoRows
	}
	revision := InitialFeishuRevision
	row := generated.Document{ID: uuid.New(), KbID: input.KBID, SourceType: input.SourceType, SourceRef: input.SourceRef, Status: string(domain.StatusPending), SyncStatus: "idle", RemoteRevision: &revision, OauthAccountID: twoOwnerUUID(input.OAuthAccountID), Metadata: []byte("{}")}
	if err := enqueuer.EnqueueFeishuSync(ctx, row.ID.String(), InitialFeishuRevision); err != nil {
		return generated.Document{}, err
	}
	r.docs[row.ID] = row
	return row, nil
}

func (r *twoOwnerFeishuRepo) GetDocumentForOwner(_ context.Context, arg generated.GetDocumentForOwnerParams) (generated.Document, error) {
	row, ok := r.docs[arg.ID]
	if !ok || r.kbOwners[row.KbID] != arg.OwnerUserID.Bytes {
		return generated.Document{}, pgx.ErrNoRows
	}
	return row, nil
}

func (r *twoOwnerFeishuRepo) GetFeishuDocumentForOwnerAndAccount(_ context.Context, arg generated.GetFeishuDocumentForOwnerAndAccountParams) (generated.Document, error) {
	row, err := r.GetDocumentForOwner(context.Background(), generated.GetDocumentForOwnerParams{ID: arg.ID, OwnerUserID: arg.OwnerUserID})
	if err != nil || !row.OauthAccountID.Valid || row.OauthAccountID.Bytes != arg.OauthAccountID || r.accountOwners[arg.OauthAccountID] != arg.OwnerUserID.Bytes {
		return generated.Document{}, pgx.ErrNoRows
	}
	return row, nil
}

type twoOwnerFeishuQueue struct{ ids []string }

func (q *twoOwnerFeishuQueue) EnqueueFeishuSync(_ context.Context, id, _ string) error {
	q.ids = append(q.ids, id)
	return nil
}

func TestTwoOwnerFeishuImportAndSyncBehavior(t *testing.T) {
	ownerA, ownerB, kbA, kbB, accountA, accountB := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	repo := &twoOwnerFeishuRepo{
		kbOwners:      map[uuid.UUID]uuid.UUID{kbA: ownerA, kbB: ownerB},
		accountOwners: map[uuid.UUID]uuid.UUID{accountA: ownerA, accountB: ownerB},
		docs:          map[uuid.UUID]generated.Document{},
	}
	safeURL, err := domain.NewSafeURL("https://acme.feishu.cn/docx/token")
	if err != nil {
		t.Fatal(err)
	}
	ref, err := domain.NewResourceRef(domain.ResourceRefInput{Type: domain.ResourceDocx, ProviderHost: "feishu.cn", Token: "token", CanonicalURL: safeURL, OriginalURL: safeURL})
	if err != nil {
		t.Fatal(err)
	}
	queue := &twoOwnerFeishuQueue{}
	imports := NewFeishuImport(staticSourceResolver{ref: ref}, repo, queue)

	if _, err := imports.Import(context.Background(), FeishuImportInput{UserID: ownerB.String(), KBID: kbA.String(), OAuthAccountID: accountB.String(), URL: safeURL.String()}); !isFeishuOwnership(err) {
		t.Fatalf("owner B Import(A KB) error=%v", err)
	}
	if len(repo.docs) != 0 || len(queue.ids) != 0 {
		t.Fatalf("foreign import docs=%#v queue=%#v", repo.docs, queue.ids)
	}
	owned, err := imports.Import(context.Background(), FeishuImportInput{UserID: ownerA.String(), KBID: kbA.String(), OAuthAccountID: accountA.String(), URL: safeURL.String()})
	if err != nil || len(queue.ids) != 1 {
		t.Fatalf("owned import doc=%#v queue=%#v err=%v", owned, queue.ids, err)
	}

	syncQueue := &twoOwnerFeishuQueue{}
	syncer := NewFeishuSync(repo, syncQueue)
	if _, err := syncer.Sync(context.Background(), ownerB.String(), accountB.String(), owned.ID); !isDocNotFound(err) {
		t.Fatalf("owner B Sync(A doc) error=%v", err)
	}
	if len(syncQueue.ids) != 0 {
		t.Fatalf("foreign sync enqueued work: %#v", syncQueue.ids)
	}
	if _, err := syncer.Sync(context.Background(), ownerA.String(), accountA.String(), owned.ID); err != nil || len(syncQueue.ids) != 1 {
		t.Fatalf("owned sync queue=%#v err=%v", syncQueue.ids, err)
	}
}

func isFeishuOwnership(err error) bool {
	var target *ErrFeishuImportOwnership
	return errors.As(err, &target)
}

type twoOwnerBootstrapRepo struct {
	owners map[string]*uuid.UUID
}

func (r *twoOwnerBootstrapRepo) CountOrphanOwnership(context.Context) (int64, error) {
	var count int64
	for _, owner := range r.owners {
		if owner == nil {
			count++
		}
	}
	return count, nil
}

func (r *twoOwnerBootstrapRepo) BootstrapOwner(_ context.Context, ownerID uuid.UUID, _ string) error {
	for key, owner := range r.owners {
		if owner == nil {
			assigned := ownerID
			r.owners[key] = &assigned
		}
	}
	return nil
}

func TestTwoOwnerBootstrapPreservesExistingOwnersAndAssignsOnlyOrphans(t *testing.T) {
	ownerA, ownerB := uuid.New(), uuid.New()
	repo := &twoOwnerBootstrapRepo{owners: map[string]*uuid.UUID{"kb-a": &ownerA, "kb-b": &ownerB, "legacy": nil}}
	if err := NewOwnershipBootstrap(repo).Run(context.Background(), "ou_legacy"); err != nil {
		t.Fatal(err)
	}
	wantLegacy := uuid.NewSHA1(uuid.NameSpaceOID, []byte("feishu:ou_legacy"))
	if *repo.owners["kb-a"] != ownerA || *repo.owners["kb-b"] != ownerB || repo.owners["legacy"] == nil || *repo.owners["legacy"] != wantLegacy {
		t.Fatalf("bootstrap owners=%#v", repo.owners)
	}
	if err := NewOwnershipBootstrap(repo).Run(context.Background(), ""); err != nil {
		t.Fatalf("idempotent bootstrap error=%v", err)
	}
}
