package opencode

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/StvnWthrsp/minecraft-server-agent/internal/bot"
)

var sessionID = regexp.MustCompile(`^ses_[A-Za-z0-9]+$`)

type Agent struct {
	Client    *Planner
	Directory string
}

func (a Agent) request(ctx context.Context, method, path string, body, out any) error {
	err := a.Client.requestLimit(ctx, method, path, body, out, false, 2*1024*1024)
	return agentError(ctx, err)
}

func agentError(ctx context.Context, err error) error {
	if err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("OpenCode agent request canceled: %w", ctx.Err())
		}
		return errors.New(strings.ReplaceAll(err.Error(), "; no action taken", "; inspect the OpenCode session and server state"))
	}
	return nil
}

func (a Agent) agentID() string {
	if a.Client.config.AgentID == "" {
		return "build"
	}
	return a.Client.config.AgentID
}

func (a Agent) model() map[string]string {
	return map[string]string{"id": a.Client.config.ModelID, "providerID": a.Client.config.ProviderID}
}

func (a Agent) Create(ctx context.Context, title string) (string, error) {
	if err := a.Client.Check(ctx); err != nil {
		return "", agentError(ctx, err)
	}
	var response struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	body := map[string]any{
		"title": title, "agent": a.agentID(), "model": a.model(),
		"location":    map[string]string{"directory": a.Directory},
		"permissions": []map[string]string{{"action": "*", "resource": "*", "effect": "allow"}},
	}
	if err := a.request(ctx, http.MethodPost, "/api/session", body, &response); err != nil {
		return "", err
	}
	if !sessionID.MatchString(response.Data.ID) || len(response.Data.ID) > 128 {
		return "", errors.New("OpenCode returned an invalid session ID")
	}
	return response.Data.ID, nil
}

func (a Agent) Interrupt(ctx context.Context, id string) error {
	if !sessionID.MatchString(id) || len(id) > 128 {
		return errors.New("invalid recorded OpenCode session ID")
	}
	if err := a.Client.Check(ctx); err != nil {
		return agentError(ctx, err)
	}
	var result struct {
		Interrupted bool `json:"interrupted"`
	}
	err := a.Client.request(ctx, http.MethodPost, "/api/session/"+id+"/interrupt", nil, &result, false)
	var status httpStatusError
	if errors.As(err, &status) && status.code == http.StatusNotFound {
		return bot.ErrAgentSessionGone
	}
	if err != nil {
		return agentError(ctx, err)
	}
	if err := a.request(ctx, http.MethodPost, "/api/experimental/session/"+id+"/wait", nil, nil); err != nil {
		return err
	}
	var pending struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := a.request(ctx, http.MethodGet, "/api/session/"+id+"/inbox", nil, &pending); err != nil {
		return err
	}
	for _, item := range pending.Data {
		if len(item.ID) > 128 || !regexp.MustCompile(`^msg[_A-Za-z0-9]+$`).MatchString(item.ID) {
			return errors.New("invalid pending OpenCode input ID")
		}
		if err := a.request(ctx, http.MethodDelete, "/api/session/"+id+"/inbox/"+url.PathEscape(item.ID), nil, nil); err != nil {
			return err
		}
	}
	return nil
}

type agentMessage struct {
	ID      string          `json:"id"`
	Type    string          `json:"type"`
	Outcome string          `json:"outcome"`
	Error   json.RawMessage `json:"error"`
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
}

func (a Agent) messages(ctx context.Context, id string) ([]agentMessage, error) {
	var response struct {
		Data []agentMessage `json:"data"`
	}
	err := a.request(ctx, http.MethodGet, "/api/session/"+id+"/message?limit=20&order=desc", nil, &response)
	return response.Data, err
}

func latestIdle(messages []agentMessage) string {
	for _, m := range messages {
		if m.Type == "idle" {
			return m.ID
		}
	}
	return ""
}

type agentForm struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Fields []struct {
		Key         string `json:"key"`
		Type        string `json:"type"`
		Title       string `json:"title"`
		Description string `json:"description"`
		Options     []struct {
			Value string `json:"value"`
			Label string `json:"label"`
		} `json:"options"`
	} `json:"fields"`
}

func (a Agent) forms(ctx context.Context, id string) ([]agentForm, error) {
	var response struct {
		Data []agentForm `json:"data"`
	}
	err := a.request(ctx, http.MethodGet, "/api/session/"+id+"/form", nil, &response)
	return response.Data, err
}

func describeForm(f agentForm) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n", f.Title)
	for _, field := range f.Fields {
		fmt.Fprintf(&b, "%s: %s %s\n", field.Key, field.Title, field.Description)
		for i, option := range field.Options {
			fmt.Fprintf(&b, "%d) %s\n", i+1, option.Label)
		}
	}
	if len(f.Fields) == 1 {
		b.WriteString("Reply with minecraft-server-agent followed by your answer (option number or text; comma-separated for multiple choices).")
	} else {
		b.WriteString(`Reply with minecraft-server-agent followed by a JSON object of answers, e.g. {"q0":"Yes","q1":["Option"]}.`)
	}
	return b.String()
}

func formAnswer(f agentForm, text string) (map[string]any, error) {
	if len(f.Fields) != 1 || strings.HasPrefix(strings.TrimSpace(text), "{") {
		var answer map[string]any
		if json.Unmarshal([]byte(text), &answer) != nil || answer == nil {
			return nil, errors.New("reply with a JSON object keyed by the question fields")
		}
		return answer, nil
	}
	field := f.Fields[0]
	option := func(text string) string {
		text = strings.TrimSpace(text)
		if i, err := strconv.Atoi(text); err == nil && i > 0 && i <= len(field.Options) {
			return field.Options[i-1].Value
		}
		for _, o := range field.Options {
			if strings.EqualFold(text, o.Label) {
				return o.Value
			}
		}
		return text
	}
	var value any
	switch field.Type {
	case "string":
		value = option(text)
	case "multiselect":
		values := []string{}
		for _, part := range strings.Split(text, ",") {
			values = append(values, option(part))
		}
		value = values
	case "boolean":
		switch strings.ToLower(strings.TrimSpace(text)) {
		case "yes", "true":
			value = true
		case "no", "false":
			value = false
		default:
			return nil, errors.New("answer yes or no")
		}
	case "number", "integer":
		var err error
		value, err = strconv.ParseFloat(text, 64)
		if err != nil {
			return nil, errors.New("answer with a number")
		}
	default:
		return nil, errors.New("answer this form in the OpenCode interface")
	}
	return map[string]any{field.Key: value}, nil
}

func (a Agent) Run(ctx context.Context, id, text string) (reply bot.AgentReply, runErr error) {
	if !sessionID.MatchString(id) || len(id) > 128 {
		return reply, errors.New("invalid OpenCode session ID")
	}
	if err := a.Client.Check(ctx); err != nil {
		return reply, agentError(ctx, err)
	}
	messages, err := a.messages(ctx, id)
	if err != nil {
		return reply, err
	}
	previousIdle := latestIdle(messages)
	forms, err := a.forms(ctx, id)
	if err != nil {
		return reply, err
	}
	if len(forms) == 0 {
		var active struct {
			Data map[string]json.RawMessage `json:"data"`
		}
		if err := a.request(ctx, http.MethodGet, "/api/session/active", nil, &active); err != nil {
			return reply, err
		}
		for attempt := 0; ; attempt++ {
			if _, running := active.Data[id]; !running {
				break
			}
			if attempt == 5 {
				return reply, errors.New("this OpenCode session is already running; use /stop or inspect it before sending another request")
			}
			select {
			case <-ctx.Done():
				return reply, ctx.Err()
			case <-time.After(100 * time.Millisecond):
			}
			if err := a.request(ctx, http.MethodGet, "/api/session/active", nil, &active); err != nil {
				return reply, err
			}
		}
		var inbox struct {
			Data []json.RawMessage `json:"data"`
		}
		if err := a.request(ctx, http.MethodGet, "/api/session/"+id+"/inbox", nil, &inbox); err != nil {
			return reply, err
		}
		if len(inbox.Data) > 0 {
			return reply, errors.New("this session has undelivered inputs; use /stop to clear them before a new request")
		}
	}
	started := false
	defer func() {
		if started && runErr != nil {
			stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := a.Interrupt(stopCtx, id); err != nil {
				runErr = errors.New("agent outcome unknown and interruption could not be confirmed; inspect the OpenCode session and server state")
			}
		}
	}()
	if len(forms) > 0 {
		answer, err := formAnswer(forms[0], text)
		if err != nil {
			return bot.AgentReply{Text: err.Error() + "\n" + describeForm(forms[0]), Waiting: true}, nil
		}
		started = true
		if err := a.request(ctx, http.MethodPost, "/api/session/"+id+"/form/"+url.PathEscape(forms[0].ID)+"/reply", map[string]any{"answer": answer}, nil); err != nil {
			return reply, err
		}
	} else {
		if err := a.request(ctx, http.MethodPost, "/api/session/"+id+"/model", map[string]any{"model": a.model()}, nil); err != nil {
			return reply, err
		}
		if err := a.request(ctx, http.MethodPost, "/api/session/"+id+"/agent", map[string]string{"agent": a.agentID()}, nil); err != nil {
			return reply, err
		}
		prompt := "You are administering the Minecraft server in " + a.Directory + " for its trusted OP administrator. Use your normal OpenCode tools to carry out their request, including system and web access. Verify actual outcomes. For changes, use appropriate backups and explain recovery if something fails. Ask for missing details rather than inventing startup commands or mod compatibility. Do not print credentials. Reply concisely for Minecraft chat. This is an executable administration request, not a request for a JSON proposal.\n\nAdministrator request:\n" + text
		var admitted json.RawMessage
		started = true
		if err := a.request(ctx, http.MethodPost, "/api/session/"+id+"/prompt", map[string]any{"text": prompt}, &admitted); err != nil {
			return reply, err
		}
	}
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		forms, err := a.forms(ctx, id)
		if err != nil {
			return reply, err
		}
		if len(forms) > 0 {
			return bot.AgentReply{Text: describeForm(forms[0]), Waiting: true}, nil
		}
		messages, err := a.messages(ctx, id)
		if err != nil {
			return reply, err
		}
		if idle := latestIdle(messages); idle != "" && idle != previousIdle {
			for _, m := range messages {
				if m.ID == previousIdle {
					break
				}
				if m.Type == "idle" && (m.Outcome != "succeeded" || (len(m.Error) > 0 && string(m.Error) != "null")) {
					return reply, errors.New("OpenCode agent did not complete successfully; inspect its session and server state")
				}
				if m.Type == "assistant" {
					if len(m.Error) > 0 && string(m.Error) != "null" {
						return reply, errors.New("OpenCode agent failed; inspect its session and server state")
					}
					var parts []string
					for _, content := range m.Content {
						if content.Type == "text" && content.Text != "" {
							parts = append(parts, content.Text)
						}
					}
					if len(parts) > 0 {
						return bot.AgentReply{Text: strings.Join(parts, "\n")}, nil
					}
				}
			}
			return reply, errors.New("OpenCode completed without a text response; inspect its session")
		}
		select {
		case <-ctx.Done():
			return reply, ctx.Err()
		case <-ticker.C:
		}
	}
}
