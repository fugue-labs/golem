package ui

import (
	"context"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/fugue-labs/gollem/ext/codetool"
)

func makeAskUserFunc(runID int, ch chan askUserRequest) codetool.AskUserFunc {
	return func(ctx context.Context, questions []codetool.AskUserQuestion) ([]codetool.AskUserAnswer, error) {
		resp := make(chan []codetool.AskUserAnswer, 1)
		select {
		case ch <- askUserRequest{runID: runID, questions: questions, response: resp}:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		select {
		case answers := <-resp:
			return answers, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

func (m *Model) waitForAskUser() tea.Cmd {
	return func() tea.Msg {
		select {
		case req := <-m.askUserCh:
			if req.response == nil {
				return askUserShutdownMsg{}
			}
			return req
		case <-m.askDone:
			return askUserShutdownMsg{}
		}
	}
}

func (m *Model) shutdownAskLoop() {
	select {
	case <-m.askDone:
	default:
		close(m.askDone)
	}
}

func (m *Model) beginAskMode(req askUserRequest) {
	m.askMode = true
	m.askQuestions = append([]codetool.AskUserQuestion(nil), req.questions...)
	m.askAnswers = nil
	m.askCurrent = 0
	m.askRespCh = req.response
	m.input.Reset()
	m.input.SetHeight(1)
	m.input.Placeholder = askModePlaceholder(m.askQuestions, 0)
}

func (m *Model) resetAskState() {
	m.askMode = false
	m.askQuestions = nil
	m.askAnswers = nil
	m.askCurrent = 0
	m.askRespCh = nil
	m.input.Reset()
	m.input.SetHeight(1)
	m.input.Placeholder = "Ask anything… /help for commands"
}

func (m *Model) currentInputHeight() int {
	if m.askMode {
		return 6
	}
	inputHeight := strings.Count(m.input.Value(), "\n") + 2
	if inputHeight > 6 {
		inputHeight = 6
	}
	if m.busy {
		inputHeight++
	}
	return inputHeight
}

func (m *Model) handleAskKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	switch key {
	case "escape", "ctrl+c":
		if m.cancel != nil {
			m.cancel()
		}
		m.resetAskState()
		return m, m.input.Focus()
	case "shift+enter":
		m.input.InsertString("\n")
		return m, nil
	case "enter":
		answer := strings.TrimSpace(m.input.Value())
		if answer == "" {
			return m, nil
		}
		if idx, err := strconv.Atoi(answer); err == nil {
			q := m.askQuestions[m.askCurrent]
			if idx >= 1 && idx <= len(q.Options) {
				answer = q.Options[idx-1]
			}
		}
		m.askAnswers = append(m.askAnswers, codetool.AskUserAnswer{QuestionIndex: m.askCurrent, Selected: answer})
		m.askCurrent++
		if m.askCurrent >= len(m.askQuestions) {
			if m.askRespCh != nil {
				select {
				case m.askRespCh <- append([]codetool.AskUserAnswer(nil), m.askAnswers...):
				default:
				}
			}
			m.resetAskState()
			return m, m.input.Focus()
		}
		m.input.Reset()
		m.input.SetHeight(1)
		m.input.Placeholder = askModePlaceholder(m.askQuestions, m.askCurrent)
		return m, m.input.Focus()
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m *Model) renderAskInput() string {
	if len(m.askQuestions) == 0 || m.askCurrent >= len(m.askQuestions) {
		return m.renderInputBusyOrIdle()
	}
	q := m.askQuestions[m.askCurrent]
	var b strings.Builder
	b.WriteString("Question ")
	b.WriteString(strconv.Itoa(m.askCurrent + 1))
	b.WriteString("/")
	b.WriteString(strconv.Itoa(len(m.askQuestions)))
	b.WriteString("\n")
	b.WriteString(strings.TrimSpace(q.Text))
	b.WriteString("\n")
	for i, opt := range q.Options {
		b.WriteString("  ")
		b.WriteString(strconv.Itoa(i + 1))
		b.WriteString(". ")
		b.WriteString(opt)
		b.WriteString("\n")
	}
	prompt := m.sty.Input.Prompt.Render(" > ")
	b.WriteString(prompt)
	b.WriteString(m.input.View())
	return b.String()
}

func askModePlaceholder(_ []codetool.AskUserQuestion, _ int) string {
	return "Type a number or your own answer"
}
