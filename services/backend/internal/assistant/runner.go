package assistant

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"eva/services/backend/internal/llm"
	"eva/services/backend/internal/observability"
	"eva/services/backend/internal/repository"
	"eva/services/backend/internal/search"
)

type Runner struct {
	LLM    *llm.Client
	Search *search.Client
	Store  *repository.Store
}

type ToolExecInfo struct {
	ToolName string
	Status   string
}

// RunTurn executes assistant with tool loop; returns assistant text and optional sources JSON.
func (r *Runner) RunTurn(ctx context.Context, userID, convID uuid.UUID, userText string) (reply string, sourcesJSON []byte, tools []ToolExecInfo, err error) {
	if _, err := r.Store.AddMessage(ctx, convID, "user", userText, nil); err != nil {
		return "", nil, nil, err
	}
	msgs, err := r.Store.ListMessages(ctx, userID, convID)
	if err != nil {
		return "", nil, nil, err
	}
	lmMsgs := make([]llm.Message, 0, len(msgs))
	for _, m := range msgs {
		if m.Role == "tool" {
			lmMsgs = append(lmMsgs, llm.Message{Role: "tool", Content: m.Content, Name: "web_search"})
			continue
		}
		lmMsgs = append(lmMsgs, llm.Message{Role: m.Role, Content: m.Content})
	}
	toolDefs := []llm.ToolDef{llm.WebSearchTool()}
	var sources []repositorySource
	const maxToolIters = 5
	for iter := 0; iter < maxToolIters; iter++ {
		text, calls, err := r.LLM.Complete(ctx, lmMsgs, toolDefs)
		if err != nil {
			return "", nil, tools, err
		}
		if len(calls) == 0 {
			reply = text
			break
		}
		lmMsgs = append(lmMsgs, llm.Message{Role: "assistant", Content: text, ToolCalls: calls})
		for _, tc := range calls {
			if tc.Function.Name != "web_search" {
				continue
			}
			var args struct {
				Query string `json:"query"`
			}
			_ = json.Unmarshal([]byte(tc.Function.Arguments), &args)
			in, _ := json.Marshal(args)
			tools = append(tools, ToolExecInfo{tc.Function.Name, "started"})
			hits, serr := r.Search.Search(ctx, args.Query, 5)
			var out []byte
			status := "ok"
			var errMsg *string
			if serr != nil {
				status = "error"
				obsTool(status)
				observability.SearchRequests.WithLabelValues("error").Inc()
				em := serr.Error()
				errMsg = &em
				out, _ = json.Marshal(map[string]string{"error": em})
				_ = r.Store.RecordToolExecution(ctx, convID, tc.Function.Name, "failed", in, out, errMsg)
			} else {
				obsTool("ok")
				observability.SearchRequests.WithLabelValues("ok").Inc()
				sources = append(sources, hitsToSources(hits)...)
				out, _ = json.Marshal(hits)
				_ = r.Store.RecordToolExecution(ctx, convID, tc.Function.Name, "ok", in, out, nil)
			}
			tools[len(tools)-1].Status = status
			toolContent := search.FormatHits(hits)
			if serr != nil {
				toolContent = fmt.Sprintf("Search failed: %v", serr)
			}
			lmMsgs = append(lmMsgs, llm.Message{
				Role:       "tool",
				ToolCallID: tc.ID,
				Name:       tc.Function.Name,
				Content:    toolContent,
			})
			if _, err := r.Store.AddMessage(ctx, convID, "tool", toolContent, nil); err != nil {
				return "", nil, tools, err
			}
		}
	}
	if reply == "" {
		reply = "(empty response)"
	}
	if len(sources) > 0 {
		b, _ := json.Marshal(sources)
		sourcesJSON = b
	}
	if _, err := r.Store.AddMessage(ctx, convID, "assistant", reply, sourcesJSON); err != nil {
		return "", nil, tools, err
	}
	return reply, sourcesJSON, tools, nil
}

type repositorySource struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet,omitempty"`
}

func hitsToSources(hits []search.Hit) []repositorySource {
	out := make([]repositorySource, 0, len(hits))
	for _, h := range hits {
		out = append(out, repositorySource{Title: h.Title, URL: h.URL, Snippet: h.Snippet})
	}
	return out
}

// StreamFake streams assistant text over SSE after full turn (tools included in non-streamed phase).
func (r *Runner) RunTurnStreamSSE(ctx context.Context, userID, convID uuid.UUID, userText string, emit func(event string, data []byte) error) error {
	reply, _, _, err := r.RunTurn(ctx, userID, convID, userText)
	if err != nil {
		return err
	}
	chunk := 0
	const size = 8
	for i := 0; i < len(reply); i += size {
		end := i + size
		if end > len(reply) {
			end = len(reply)
		}
		part := reply[i:end]
		d, _ := json.Marshal(map[string]any{"type": "delta", "text": part})
		if err := emit("message", d); err != nil {
			return err
		}
		chunk++
	}
	fin, _ := json.Marshal(map[string]any{"type": "done", "chunks": chunk})
	return emit("message", fin)
}

// ChunkText splits reply for UI streaming simulation.
func ChunkText(s string) []string {
	if s == "" {
		return nil
	}
	var out []string
	const size = 12
	for i := 0; i < len(s); i += size {
		end := i + size
		if end > len(s) {
			end = len(s)
		}
		out = append(out, s[i:end])
	}
	return out
}

func JoinChunks(chunks []string) string { return strings.Join(chunks, "") }

func obsTool(result string) {
	observability.ToolCalls.WithLabelValues("web_search", result).Inc()
}
