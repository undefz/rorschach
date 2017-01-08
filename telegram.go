package main

import (
	"fmt"
	"gopkg.in/telegram-bot-api.v4"
	"time"
)

func formatDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	return fmt.Sprintf("%02.0fm:%02.0fs", d.Minutes(), (d % time.Minute).Seconds())
}

func sendKeyboard(chatId int64, text string, status ChatStatus) {
	msg := tgbotapi.NewMessage(chatId, text)

	var rows [][]tgbotapi.KeyboardButton

	switch status {
	case Idle:
		rows = append(rows, []tgbotapi.KeyboardButton{
			tgbotapi.NewKeyboardButton("/start"),
			tgbotapi.NewKeyboardButton("/stop"),
		})
		rows = append(rows, []tgbotapi.KeyboardButton{
			tgbotapi.NewKeyboardButton("/tasks add"),
			tgbotapi.NewKeyboardButton("/tasks set"),
			tgbotapi.NewKeyboardButton("/tasks delete"),
		})
	case Counter:
		rows = append(rows, []tgbotapi.KeyboardButton{
			tgbotapi.NewKeyboardButton("/stop"),
		})
	}
	keyboard := tgbotapi.NewReplyKeyboard(rows...)
	keyboard.OneTimeKeyboard = true

	msg.ReplyMarkup = keyboard

	bot.Send(msg)
}

func sendMessage(chatId int64, text string) int {
	msg := tgbotapi.NewMessage(chatId, text)
	sent, err := bot.Send(msg)
	if err != nil {
		return 0
	}
	return sent.MessageID
}
