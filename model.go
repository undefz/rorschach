package main

import (
	"sync"
	"time"
)

const (
	pomoTime     = 25 * time.Minute
	breakTime    = 5 * time.Minute
	reminderTime = 1 * time.Minute
)

type PomoStatus int64

const (
	empty = PomoStatus(iota)
	pomoStarted
	pomoEnded
	breakStarted
	breakEnded
)

type ChatStatus int64

const (
	Idle = ChatStatus(iota)
	Counter
	AddingTask
	DeletingTask
	SelectingTask
)

type UserSession struct {
	user      User
	state     State
	chatState ChatState

	mutex sync.RWMutex
}

type User struct {
	chatId int64
	taskId int64
}

type ChatState struct {
	status    ChatStatus
	counterId int
}

type State struct {
	status  PomoStatus
	timer   *time.Timer
	started time.Time
	pomoId  int64
}

type Task struct {
	Id   int64  `db:"id"`
	Name string `db:"name"`
}
