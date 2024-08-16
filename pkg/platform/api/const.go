package api

type MergeRequestState string

const (
	MergeRequestStateOpen   MergeRequestState = "open"
	MergeRequestStateClosed MergeRequestState = "closed"
)

type PipelineState string

const (
	PipelineStateUnknown            PipelineState = "unknown"
	PipelineStateCreated            PipelineState = "created"
	PipelineStateWaitingForResource PipelineState = "waiting_for_resource"
	PipelineStatePreparing          PipelineState = "preparing"
	PipelineStatePending            PipelineState = "pending"
	PipelineStateRunning            PipelineState = "running"
	PipelineStateSuccess            PipelineState = "success"
	PipelineStateFailed             PipelineState = "failed"
	PipelineStateCanceled           PipelineState = "canceled"
	PipelineStateSkipped            PipelineState = "skipped"
	PipelineStateManual             PipelineState = "manual"
	PipelineStateScheduled          PipelineState = "scheduled"
)

type UserType string

const (
	UserTypeUser UserType = "user"
	UserTypeBot  UserType = "bot"
)

type UserState string

const (
	UserStateActive    UserState = "active"
	UserStateSuspended UserState = "suspended"
)
