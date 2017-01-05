package main

import "time"

const (
	pomoTime     = 10 * time.Second
	breakTime    = 5 * time.Second
	reminderTime = 3 * time.Second
)

type PomoStatus int64

const (
	empty = PomoStatus(iota)
	pomoStarted
	pomoEnded
	breakStarted
	breakEnded
)

type UserSession struct {
	chatId    int64
	status    PomoStatus
	timer     *time.Timer
	started   time.Time
	messageId int
	pomoId    int64
}
