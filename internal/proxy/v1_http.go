package proxy

import (
	"bufio"
	"context"
	jsonStd "encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	genv1 "github.com/soltiHQ/control-plane/api/gen/v1"
	proxyv1 "github.com/soltiHQ/control-plane/api/proxy/v1"
)

const (
	v1PathTasks = "/api/v1/tasks"
)

// httpV1Unmarshal decodes canonical proto-JSON responses from the SDK.
// DiscardUnknown keeps forward compatibility when newer agents add fields.
var httpV1Unmarshal = protojson.UnmarshalOptions{DiscardUnknown: true}

// httpProxyV1 implements AgentProxy over HTTP for API v1.
type httpProxyV1 struct {
	endpoint string
	client   httpClient
}

func (p *httpProxyV1) ListTasks(ctx context.Context, f TaskFilter) (*proxyv1.TaskListResponse, error) {
	u, err := url.Parse(p.endpoint + v1PathTasks)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBadEndpointURL, err)
	}

	q := u.Query()
	if f.Slot != "" {
		q.Set("slot", f.Slot)
	}
	if f.Status != "" {
		q.Set("status", f.Status)
	}
	if f.Limit > 0 {
		q.Set("limit", strconv.Itoa(f.Limit))
	}
	if f.Offset > 0 {
		q.Set("offset", strconv.Itoa(f.Offset))
	}
	u.RawQuery = q.Encode()

	var out genv1.ListTasksResponse
	if err := doProtoJSONGet(ctx, p.client, u.String(), &out); err != nil {
		return nil, err
	}

	tasks := make([]proxyv1.Task, len(out.GetTasks()))
	for i, t := range out.GetTasks() {
		tasks[i] = taskDataToProxy(t)
	}

	return &proxyv1.TaskListResponse{
		Tasks: tasks,
		Total: int(out.GetTotal()),
	}, nil
}

func (p *httpProxyV1) SubmitTask(ctx context.Context, sub TaskSubmission) (string, error) {
	if sub.Spec == nil {
		return "", fmt.Errorf("%w: nil spec", ErrSubmitTask)
	}
	u, err := url.Parse(p.endpoint + v1PathTasks)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrBadEndpointURL, err)
	}

	// SubmitTaskResponse carries the TaskId the agent assigned. Without
	// it CP cannot later DeleteTask / GetTaskStatus for this exact run,
	// which breaks the update and uninstall flows.
	var out genv1.SubmitTaskResponse
	if err := doProtoJSONPostDecoding(ctx, p.client, u.String(), &genv1.SubmitTaskRequest{Spec: sub.Spec}, &out); err != nil {
		return "", err
	}
	taskID := out.GetTaskId()
	if taskID == "" {
		return "", fmt.Errorf("%w: agent returned empty task id", ErrSubmitTask)
	}
	return taskID, nil
}

func (p *httpProxyV1) GetTask(ctx context.Context, taskID string) (*proxyv1.TaskStatusResponse, error) {
	u, err := url.Parse(fmt.Sprintf("%s%s/%s", p.endpoint, v1PathTasks, taskID))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBadEndpointURL, err)
	}

	var out genv1.GetTaskStatusResponse
	if err := doProtoJSONGet(ctx, p.client, u.String(), &out); err != nil {
		return nil, err
	}

	var task *proxyv1.Task
	if data := out.GetTask(); data != nil {
		t := taskDataToProxy(data)
		task = &t
	}
	return &proxyv1.TaskStatusResponse{Info: task}, nil
}

func (p *httpProxyV1) ListTaskRuns(ctx context.Context, taskID string) (*proxyv1.TaskRunListResponse, error) {
	u, err := url.Parse(fmt.Sprintf("%s%s/%s/runs", p.endpoint, v1PathTasks, taskID))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBadEndpointURL, err)
	}

	var out genv1.ListTaskRunsResponse
	if err := doProtoJSONGet(ctx, p.client, u.String(), &out); err != nil {
		return nil, err
	}

	runs := make([]proxyv1.TaskRun, len(out.GetRuns()))
	for i, r := range out.GetRuns() {
		run := proxyv1.TaskRun{
			Attempt:   int(r.GetAttempt()),
			Status:    v1TaskStatusString(r.GetStatus()),
			StartedAt: r.GetStartedAt(),
		}
		if r.FinishedAt != nil {
			run.FinishedAt = *r.FinishedAt
		}
		if r.Error != nil {
			run.Error = *r.Error
		}
		if r.ExitCode != nil {
			run.ExitCode = r.ExitCode
		}
		runs[i] = run
	}
	return &proxyv1.TaskRunListResponse{Runs: runs}, nil
}

func (p *httpProxyV1) DeleteTask(ctx context.Context, taskID string) error {
	u, err := url.Parse(fmt.Sprintf("%s%s/%s", p.endpoint, v1PathTasks, taskID))
	if err != nil {
		return fmt.Errorf("%w: %v", ErrBadEndpointURL, err)
	}

	return doDelete(ctx, p.client, u.String())
}

// StreamTaskLogs consumes the agent's SSE stream at
// /api/v1/tasks/{id}/logs/stream and translates each event to the proto
// shape used cluster-wide.
//
// Agent wire format is a custom flat JSON with a `type` discriminator
// (see agentLogEvent), not canonical proto-JSON. We translate locally so
// downstream consumers can rely on a single OutputEventProto shape
// regardless of the source transport.
func (p *httpProxyV1) StreamTaskLogs(ctx context.Context, taskID string) (<-chan *genv1.OutputEventProto, error) {
	u, err := url.Parse(fmt.Sprintf("%s%s/%s/logs", p.endpoint, v1PathTasks, taskID))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBadEndpointURL, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCreateRequest, err)
	}
	req.Header.Set("Accept", "text/event-stream")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrStreamTaskLogs, err)
	}
	if resp.StatusCode != http.StatusOK {
		err := formatUnexpectedStatus(resp)
		_ = resp.Body.Close()
		return nil, fmt.Errorf("%w: %v", ErrStreamTaskLogs, err)
	}

	ch := make(chan *genv1.OutputEventProto, 64)
	go func() {
		defer close(ch)
		defer resp.Body.Close()

		reader := bufio.NewReader(resp.Body)
		var dataLines []string
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				return
			}
			line = strings.TrimRight(line, "\r\n")

			// Blank line = frame terminator. Concatenate accumulated
			// `data:` lines (per SSE spec multiple data lines join with
			// newline) and dispatch.
			if line == "" {
				if len(dataLines) > 0 {
					ev := parseAgentLogEvent(strings.Join(dataLines, "\n"))
					dataLines = dataLines[:0]
					if ev == nil {
						continue
					}
					select {
					case ch <- ev:
					case <-ctx.Done():
						return
					}
				}
				continue
			}
			if strings.HasPrefix(line, "data:") {
				dataLines = append(dataLines, strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
			}
			// `event:` and other SSE fields are ignored — the type
			// discriminator inside the data payload is authoritative.
		}
	}()
	return ch, nil
}

// agentLogEvent matches the flat JSON the agent emits per SSE frame.
type agentLogEvent struct {
	Type     string `json:"type"`
	Attempt  uint32 `json:"attempt,omitempty"`
	Stream   string `json:"stream,omitempty"`
	Seq      uint64 `json:"seq,omitempty"`
	Ts       int64  `json:"ts,omitempty"`
	Line     string `json:"line,omitempty"`
	Started  int64  `json:"startedAt,omitempty"`
	Finished int64  `json:"finishedAt,omitempty"`
	ExitCode *int32 `json:"exitCode,omitempty"`
	Skipped  uint64 `json:"skipped,omitempty"`
}

// parseAgentLogEvent decodes one agent SSE payload and converts it to the
// proto shape. Returns nil on unknown or malformed events; callers should
// skip silently — the stream continues.
func parseAgentLogEvent(payload string) *genv1.OutputEventProto {
	payload = strings.TrimSpace(payload)
	if payload == "" {
		return nil
	}
	var e agentLogEvent
	if err := jsonStd.Unmarshal([]byte(payload), &e); err != nil {
		return nil
	}
	switch e.Type {
	case "chunk":
		return &genv1.OutputEventProto{Kind: &genv1.OutputEventProto_Chunk{Chunk: &genv1.OutputChunkProto{
			Attempt: e.Attempt,
			Stream:  agentStreamToProto(e.Stream),
			Seq:     e.Seq,
			Ts:      e.Ts,
			Line:    []byte(e.Line),
		}}}
	case "runStarted":
		return &genv1.OutputEventProto{Kind: &genv1.OutputEventProto_RunStarted{RunStarted: &genv1.RunStartedProto{
			Attempt:   e.Attempt,
			StartedAt: e.Started,
		}}}
	case "runFinished":
		return &genv1.OutputEventProto{Kind: &genv1.OutputEventProto_RunFinished{RunFinished: &genv1.RunFinishedProto{
			Attempt:    e.Attempt,
			ExitCode:   e.ExitCode,
			FinishedAt: e.Finished,
		}}}
	case "lagged":
		return &genv1.OutputEventProto{Kind: &genv1.OutputEventProto_Lagged{Lagged: &genv1.LaggedProto{
			Skipped: e.Skipped,
		}}}
	}
	return nil
}

func agentStreamToProto(s string) genv1.OutputStreamKind {
	switch s {
	case "stdout":
		return genv1.OutputStreamKind_OUTPUT_STREAM_KIND_STDOUT
	case "stderr":
		return genv1.OutputStreamKind_OUTPUT_STREAM_KIND_STDERR
	}
	return genv1.OutputStreamKind_OUTPUT_STREAM_KIND_UNSPECIFIED
}

// doProtoJSONGet performs a GET and decodes the response as proto-JSON.
// On non-200 responses the SDK error envelope (`{"error","message"}`) is
// surfaced via formatUnexpectedStatus instead of being silently discarded.
func doProtoJSONGet(ctx context.Context, client httpClient, url string, out proto.Message) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrCreateRequest, err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrRequest, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return formatUnexpectedStatus(resp)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrDecode, err)
	}
	if err := httpV1Unmarshal.Unmarshal(body, out); err != nil {
		return fmt.Errorf("%w: %v", ErrDecode, err)
	}
	return nil
}
