package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/zenith-wang/it-wiki/backend/internal/domain"
	"github.com/zenith-wang/it-wiki/backend/internal/repo/generated"
)

type recordingOwnerChatQueries struct {
	create     generated.CreateConversationForOwnerParams
	get        generated.GetConversationForOwnerParams
	list       generated.ListConversationsByKBForOwnerParams
	count      generated.CountConversationsByKBForOwnerParams
	update     generated.UpdateConversationModeForOwnerParams
	listMsgs   generated.ListMessagesByConversationForOwnerParams
	countMsgs  generated.CountMessagesByConversationForOwnerParams
	createMsg  generated.CreateMessageForOwnerParams
	recentMsgs generated.ListRecentMessagesByConversationForOwnerParams
	touch      generated.TouchConversationForOwnerParams
	conv       generated.Conversation
	getKBErr   error
}

func (q *recordingOwnerChatQueries) GetKnowledgeBaseForOwner(context.Context, generated.GetKnowledgeBaseForOwnerParams) (generated.KnowledgeBase, error) {
	return generated.KnowledgeBase{ID: q.conv.KbID}, q.getKBErr
}

func (q *recordingOwnerChatQueries) CreateConversationForOwner(_ context.Context, arg generated.CreateConversationForOwnerParams) (generated.Conversation, error) {
	q.create = arg
	return q.conv, nil
}

func TestConversationListReturnsNotFoundForForeignKnowledgeBase(t *testing.T) {
	kbID := uuid.New()
	q := &recordingOwnerChatQueries{conv: generated.Conversation{KbID: kbID}, getKBErr: pgx.ErrNoRows}
	_, _, err := NewChat(q, nil, nil, "", 0, nil, nil).ListConversations(context.Background(), uuid.NewString(), kbID.String(), 20, 0)
	var notFound *ErrKBNotFound
	if !errors.As(err, &notFound) || q.list.OwnerUserID.Valid {
		t.Fatalf("ListConversations() error=%v listCalled=%v", err, q.list.OwnerUserID.Valid)
	}
}
func (q *recordingOwnerChatQueries) GetConversationForOwner(_ context.Context, arg generated.GetConversationForOwnerParams) (generated.Conversation, error) {
	q.get = arg
	return q.conv, nil
}
func (q *recordingOwnerChatQueries) ListConversationsByKBForOwner(_ context.Context, arg generated.ListConversationsByKBForOwnerParams) ([]generated.Conversation, error) {
	q.list = arg
	return []generated.Conversation{q.conv}, nil
}
func (q *recordingOwnerChatQueries) CountConversationsByKBForOwner(_ context.Context, arg generated.CountConversationsByKBForOwnerParams) (int64, error) {
	q.count = arg
	return 1, nil
}
func (q *recordingOwnerChatQueries) UpdateConversationModeForOwner(_ context.Context, arg generated.UpdateConversationModeForOwnerParams) (generated.Conversation, error) {
	q.update = arg
	return q.conv, nil
}
func (q *recordingOwnerChatQueries) ListMessagesByConversationForOwner(_ context.Context, arg generated.ListMessagesByConversationForOwnerParams) ([]generated.Message, error) {
	q.listMsgs = arg
	return nil, nil
}
func (q *recordingOwnerChatQueries) CountMessagesByConversationForOwner(_ context.Context, arg generated.CountMessagesByConversationForOwnerParams) (int64, error) {
	q.countMsgs = arg
	return 0, nil
}
func (q *recordingOwnerChatQueries) CreateMessageForOwner(_ context.Context, arg generated.CreateMessageForOwnerParams) (generated.Message, error) {
	q.createMsg = arg
	return generated.Message{ID: uuid.New(), ConversationID: arg.ConversationID, Citations: []byte("[]"), ToolCalls: []byte("[]"), TokenUsage: []byte("{}")}, nil
}
func (q *recordingOwnerChatQueries) ListRecentMessagesByConversationForOwner(_ context.Context, arg generated.ListRecentMessagesByConversationForOwnerParams) ([]generated.Message, error) {
	q.recentMsgs = arg
	return nil, nil
}
func (q *recordingOwnerChatQueries) TouchConversationForOwner(_ context.Context, arg generated.TouchConversationForOwnerParams) error {
	q.touch = arg
	return nil
}

func TestChatOwnerIDReachesConversationAndMessageReadQueries(t *testing.T) {
	ownerID, kbID, convID := uuid.New(), uuid.New(), uuid.New()
	q := &recordingOwnerChatQueries{conv: generated.Conversation{ID: convID, KbID: kbID, Mode: domain.ConversationModeRAG}}
	svc := NewChat(q, nil, nil, "", 0, nil, nil)
	if _, err := svc.CreateConversation(context.Background(), ownerID.String(), kbID.String(), ""); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.UpdateMode(context.Background(), ownerID.String(), convID.String(), domain.ConversationModeRAG); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.ListConversations(context.Background(), ownerID.String(), kbID.String(), 20, 0); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.ListMessages(context.Background(), ownerID.String(), convID.String(), 20, 0); err != nil {
		t.Fatal(err)
	}
	if q.create.OwnerUserID.Bytes != ownerID || q.update.OwnerUserID.Bytes != ownerID || q.list.OwnerUserID.Bytes != ownerID || q.get.OwnerUserID.Bytes != ownerID || q.listMsgs.OwnerUserID.Bytes != ownerID || q.countMsgs.OwnerUserID.Bytes != ownerID {
		t.Fatalf("chat owner propagation mismatch: %#v", q)
	}
	if q.create.AgentID != "knowledge-rag" {
		t.Fatalf("default agent = %q", q.create.AgentID)
	}
}
