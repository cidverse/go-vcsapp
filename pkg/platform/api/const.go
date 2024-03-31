package api

type MergeRequestState string

const (
	MergeRequestStateOpen   MergeRequestState = "open"
	MergeRequestStateClosed MergeRequestState = "closed"
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
