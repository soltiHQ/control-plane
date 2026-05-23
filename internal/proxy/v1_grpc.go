package proxy

import (
	"context"
	"fmt"
	"strings"

	genv1 "github.com/soltiHQ/control-plane/api/gen/v1"
	proxyv1 "github.com/soltiHQ/control-plane/api/proxy/v1"
	"google.golang.org/grpc"
)

// grpcProxyV1 implements AgentProxy over gRPC (solti.v1.SoltiApi).
type grpcProxyV1 struct {
	conn *grpc.ClientConn
}

func (p *grpcProxyV1) ListTasks(ctx context.Context, f TaskFilter) (*proxyv1.TaskListResponse, error) {
	client := genv1.NewSoltiApiClient(p.conn)

	req := &genv1.ListTasksRequest{
		Limit:  clampUint32(f.Limit),
		Offset: clampUint32(f.Offset),
	}
	if f.Slot != "" {
		req.Slot = &f.Slot
	}
	if f.Status != "" {
		if s, ok := parseV1TaskStatus(f.Status); ok {
			req.Status = &s
		}
	}

	resp, err := client.ListTasks(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrListTasks, err)
	}

	tasks := make([]proxyv1.Task, len(resp.GetTasks()))
	for i, t := range resp.GetTasks() {
		tasks[i] = taskDataToProxy(t)
	}

	return &proxyv1.TaskListResponse{
		Tasks: tasks,
		Total: int(resp.GetTotal()),
	}, nil
}

func (p *grpcProxyV1) SubmitTask(ctx context.Context, sub TaskSubmission) (string, error) {
	if sub.Spec == nil {
		return "", fmt.Errorf("%w: nil spec", ErrSubmitTask)
	}
	client := genv1.NewSoltiApiClient(p.conn)

	resp, err := client.SubmitTask(ctx, &genv1.SubmitTaskRequest{Spec: sub.Spec})
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrSubmitTask, err)
	}
	taskID := resp.GetTaskId()
	if taskID == "" {
		// SDK contract: on success the response must carry a non-empty
		// task id. Empty means the agent accepted the request but gave
		// us nothing to cancel/delete later — treat as a soft failure
		// so the sync runner retries instead of pretending it's synced.
		return "", fmt.Errorf("%w: agent returned empty task id", ErrSubmitTask)
	}
	return taskID, nil
}

func (p *grpcProxyV1) GetTask(ctx context.Context, taskID string) (*proxyv1.TaskStatusResponse, error) {
	client := genv1.NewSoltiApiClient(p.conn)

	resp, err := client.GetTaskStatus(ctx, &genv1.GetTaskStatusRequest{TaskId: taskID})
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrGetTask, err)
	}

	var task *proxyv1.Task
	if data := resp.GetTask(); data != nil {
		t := taskDataToProxy(data)
		task = &t
	}

	return &proxyv1.TaskStatusResponse{Info: task}, nil
}

func (p *grpcProxyV1) ListTaskRuns(ctx context.Context, taskID string) (*proxyv1.TaskRunListResponse, error) {
	client := genv1.NewSoltiApiClient(p.conn)

	resp, err := client.ListTaskRuns(ctx, &genv1.ListTaskRunsRequest{TaskId: taskID})
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrListTaskRuns, err)
	}

	runs := make([]proxyv1.TaskRun, len(resp.GetRuns()))
	for i, r := range resp.GetRuns() {
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

func (p *grpcProxyV1) DeleteTask(ctx context.Context, taskID string) error {
	client := genv1.NewSoltiApiClient(p.conn)

	_, err := client.DeleteTask(ctx, &genv1.DeleteTaskRequest{TaskId: taskID})
	if err != nil {
		return fmt.Errorf("%w: %v", ErrDeleteTask, err)
	}

	return nil
}

// StreamTaskLogs opens the agent's StreamTaskLogs server-stream, forwards
// each OutputEventProto into a channel, and closes it on EOF, ctx
// cancellation, or transport error. Channel buffer is 64 — typical chunk
// cadence (~10 lines/sec) is well below; bursts are absorbed.
func (p *grpcProxyV1) StreamTaskLogs(ctx context.Context, taskID string) (<-chan *genv1.OutputEventProto, error) {
	client := genv1.NewSoltiApiClient(p.conn)
	stream, err := client.StreamTaskLogs(ctx, &genv1.StreamTaskLogsRequest{TaskId: taskID})
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrStreamTaskLogs, err)
	}
	ch := make(chan *genv1.OutputEventProto, 64)
	go func() {
		defer close(ch)
		for {
			ev, err := stream.Recv()
			if err != nil {
				return
			}
			select {
			case ch <- ev:
			case <-ctx.Done():
				return
			}
		}
	}()
	return ch, nil
}

// taskDataToProxy converts a proto TaskData (nested metadata + spec + status)
// into the flat proxy-level Task type consumed by podium's own REST/UI.
func taskDataToProxy(t *genv1.TaskData) proxyv1.Task {
	meta := t.GetMetadata()
	st := t.GetStatus()
	spec := t.GetSpec()

	task := proxyv1.Task{
		ID:              meta.GetId(),
		Slot:            spec.GetSlot(),
		Status:          v1TaskStatusString(st.GetPhase()),
		Attempt:         int(st.GetAttempt()),
		CreatedAt:       meta.GetCreatedAt(),
		UpdatedAt:       meta.GetUpdatedAt(),
		Error:           st.GetError(),
		ResourceVersion: meta.GetResourceVersion(),
	}
	if st.ExitCode != nil {
		task.ExitCode = st.ExitCode
	}
	return task
}

// v1TaskStatusString converts a v1 proto TaskStatus enum to a lowercase string.
//
//	TASK_STATUS_RUNNING → "running"
func v1TaskStatusString(s genv1.TaskStatus) string {
	name := s.String()
	name = strings.TrimPrefix(name, "TASK_STATUS_")
	return strings.ToLower(name)
}

// parseV1TaskStatus converts a lowercase status string to the v1 proto enum.
//
//	"running" → TASK_STATUS_RUNNING
func parseV1TaskStatus(s string) (genv1.TaskStatus, bool) {
	key := "TASK_STATUS_" + strings.ToUpper(s)
	v, ok := genv1.TaskStatus_value[key]
	if !ok {
		return genv1.TaskStatus_TASK_STATUS_UNSPECIFIED, false
	}
	return genv1.TaskStatus(v), true
}

func clampUint32(v int) uint32 {
	if v <= 0 {
		return 0
	}
	return uint32(v)
}
