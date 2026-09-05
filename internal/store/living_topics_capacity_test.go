package store

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func TestLivingTopicAcceptsThirtyMembersAndRejectsThirtyFirst(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	topic, err := state.CreateLivingTopic(ctx, "Capacity")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 31; i++ {
		item, err := state.CreateMemoryRecallStub(ctx, libraryInput(fmt.Sprintf("capacity-%d", i), domain.SourceX, "Capacity evidence", "Source", "2026-09-05T00:00:00Z"))
		if err != nil {
			t.Fatal(err)
		}
		_, err = state.AddLivingTopicMember(ctx, topic.ID, item.ID)
		if i < 30 && err != nil {
			t.Fatalf("member %d: %v", i+1, err)
		}
		if i == 30 && !errors.Is(err, ErrLivingTopicMemberMax) {
			t.Fatalf("31st member must fail, got %v", err)
		}
	}
	detail, err := state.LivingTopicDetail(ctx, topic.ID)
	if err != nil || len(detail.Members) != 30 {
		t.Fatalf("members=%d err=%v", len(detail.Members), err)
	}
}
