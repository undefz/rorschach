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
	return fmt.Sprintf("%02.0fm:%02.0fs", d.Minutes(), d.Seconds())
}

func sendKeyboard(chatId int64, text string, status PomoStatus) {
	msg := tgbotapi.NewMessage(chatId, text)

	var buttons []tgbotapi.KeyboardButton

	switch status {
	case empty:
		fallthrough
	case pomoEnded:
		fallthrough
	case breakEnded:
		buttons = append(buttons, tgbotapi.NewKeyboardButton("/start"))
		buttons = append(buttons, tgbotapi.NewKeyboardButton("/stop"))
	case pomoStarted:
		fallthrough
	case breakStarted:
		buttons = append(buttons, tgbotapi.NewKeyboardButton("/time"))
		buttons = append(buttons, tgbotapi.NewKeyboardButton("/stop"))
	}
	btnRow := tgbotapi.NewKeyboardButtonRow(buttons...)
	keyboard := tgbotapi.NewReplyKeyboard(btnRow)
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