package ui

import (
	"context"
	"fmt"
	"time"

	"github.com/fugue-labs/golem/internal/mission"
	"github.com/fugue-labs/golem/internal/ui/chat"
	"github.com/fugue-labs/gollem/core"
)

type missionPlanRun struct {
	MissionID      string
	PreviousStatus mission.MissionStatus
}

func (m *Model) completeMissionPlanRun(runErr error, messages []core.ModelMessage) {
	if m.missionPlanRun == nil {
		return
	}
	planRun := *m.missionPlanRun
	m.missionPlanRun = nil

	ctrl := m.missionController()
	if ctrl == nil {
		m.messages = append(m.messages, &chat.Message{Kind: chat.KindError, Content: "Mission store not available."})
		return
	}

	ctx := m.appCtx
	if runErr != nil {
		m.restoreMissionPlanStatus(ctx, ctrl, planRun)
		if isContextCanceled(runErr) {
			m.messages = append(m.messages, &chat.Message{Kind: chat.KindAssistant, Content: fmt.Sprintf("Mission planning for `%s` was cancelled.", planRun.MissionID)})
		}
		return
	}

	assistantText := lastAssistantModelText(messages)
	if assistantText == "" {
		assistantText = lastAssistantMessageContent(m.currentRunMessages)
	}
	if assistantText == "" {
		assistantText = lastAssistantMessageContent(m.messages)
	}
	if assistantText == "" {
		m.restoreMissionPlanStatus(ctx, ctrl, planRun)
		m.messages = append(m.messages, &chat.Message{Kind: chat.KindError, Content: "Mission planning completed without any assistant output to parse."})
		return
	}

	plan, err := mission.ParsePlanResult(assistantText)
	if err != nil {
		m.restoreMissionPlanStatus(ctx, ctrl, planRun)
		m.messages = append(m.messages, &chat.Message{Kind: chat.KindError, Content: fmt.Sprintf("Failed to parse mission plan: %v", err)})
		return
	}

	if err := ctrl.ApplyPlan(ctx, planRun.MissionID, plan); err != nil {
		m.restoreMissionPlanStatus(ctx, ctrl, planRun)
		m.messages = append(m.messages, &chat.Message{Kind: chat.KindError, Content: fmt.Sprintf("Failed to apply mission plan: %v", err)})
		return
	}

	m.messages = append(m.messages, &chat.Message{
		Kind: chat.KindAssistant,
		Content: fmt.Sprintf("Mission plan applied for `%s` with %d tasks and %d dependencies. Approval gate opened; run `/mission approve` to start execution.",
			planRun.MissionID, len(plan.Tasks), len(plan.Dependencies)),
	})
}

func (m *Model) restoreMissionPlanStatus(ctx context.Context, ctrl *mission.Controller, planRun missionPlanRun) {
	ms, err := ctrl.GetMission(ctx, planRun.MissionID)
	if err != nil || ms == nil {
		return
	}
	if ms.Status != mission.MissionPlanning {
		return
	}
	ms.Status = planRun.PreviousStatus
	ms.UpdatedAt = time.Now().UTC()
	_ = ctrl.Store().UpdateMission(ctx, ms)
}

func lastAssistantModelText(messages []core.ModelMessage) string {
	for i := len(messages) - 1; i >= 0; i-- {
		resp, ok := messages[i].(core.ModelResponse)
		if !ok {
			continue
		}
		if text := resp.TextContent(); text != "" {
			return text
		}
	}
	return ""
}

func lastAssistantMessageContent(messages []*chat.Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i] != nil && messages[i].Kind == chat.KindAssistant {
			return messages[i].Content
		}
	}
	return ""
}
