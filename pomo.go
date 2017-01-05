package main

import (
	"log"
	"time"

	"gopkg.in/telegram-bot-api.v4"

	_ "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"os/user"
)

var (
	bot *tgbotapi.BotAPI
	db  *sqlx.DB
)

type Config struct {
	TelegramToken      string `yaml:"TelegramToken"`
	MysqlConnectString string `yaml:"MysqlConnectString"`
	AllowedChatId      int64  `yaml:"AllowedChatId"`
}

func loadConfig() (Config, error) {
	usr, err := user.Current()
	if err != nil {
		return Config{}, err
	}
	file, err := ioutil.ReadFile(usr.HomeDir + "/.config/rorschach.yaml")
	if err != nil {
		return Config{}, err
	}
	config := Config{}
	err = yaml.Unmarshal(file, &config)
	if err != nil {
		return Config{}, err
	}

	return config, nil
}

func main() {
	config, err := loadConfig()
	if err != nil {
		log.Fatalln(err)
	}

	bot, err = tgbotapi.NewBotAPI(config.TelegramToken)
	if err != nil {
		panic(err)
	}

	bot.Debug = false

	log.Printf("Authorized on account %s", bot.Self.UserName)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates, err := bot.GetUpdatesChan(u)

	db, err = sqlx.Connect("mysql", config.MysqlConnectString)
	if err != nil {
		log.Fatalln(err)
	}

	sessions := make(map[int64]UserSession)
	sessionChannel := make(chan UserSession)
	go func() {
		for status := range sessionChannel {
			sessions[status.chatId] = status
		}
	}()
	tickerPeriod := 1 * time.Second
	ticker := time.NewTicker(tickerPeriod)
	go func() {
		for t := range ticker.C {
			for _, v := range sessions {
				switch v.status {
				case pomoStarted:
					remaining := pomoTime - t.Sub(v.started)
					sendRemainingTime(v, remaining)
				case breakStarted:
					remaining := breakTime - t.Sub(v.started)
					sendRemainingTime(v, remaining)
				case pomoEnded:
					spent := t.Sub(v.started)
					if spent%reminderTime < tickerPeriod {
						sendMessage(v.chatId, "Time for break?")
					}
				case breakEnded:
					spent := t.Sub(v.started)
					if spent%reminderTime < tickerPeriod {
						sendMessage(v.chatId, "Time for pomo?")
					}
				default:
				}
			}
		}
	}()

	for update := range updates {
		message := update.Message
		if message == nil || message.Chat.ID != config.AllowedChatId {
			continue
		}

		chat := message.Chat

		from := message.From.UserName
		text := message.Text

		log.Printf("[%+v-%s] %s", chat.ID, from, text)

		session := sessions[chat.ID]
		switch text {
		case "/start":
			switch session.status {
			case empty:
				fallthrough
			case breakEnded:
				startPomo(chat.ID, sessionChannel)
			case pomoStarted:
			//send error and how many time left
			case pomoEnded:
				startBreak(chat.ID, sessionChannel)
			case breakStarted:
				//send error and how many time left
			}
		case "/stop":
			switch session.status {
			case pomoStarted:
				session.timer.Stop()
				endPomo(chat.ID, session.pomoId, sessionChannel)
			case breakStarted:
				session.timer.Stop()
				endBreak(chat.ID, sessionChannel)
			default:
				resetState(chat.ID, sessionChannel)
			}
		default:
			sendKeyboard(chat.ID, "Unknown command", session.status)
		}
	}
}

func resetState(chatId int64, sessionChannel chan<- UserSession) {
	sessionChannel <- UserSession{chatId: chatId, status: empty}
	sendKeyboard(chatId, "Timer reset", empty)
}

func startPomo(chatId int64, sessionChannel chan<- UserSession) {
	sendKeyboard(chatId, "Pomodoro started", pomoStarted)
	timeMsgId := sendMessage(chatId, formatDuration(pomoTime))

	pomoId := insertPomo(chatId)

	timer := time.AfterFunc(pomoTime, func() {
		endPomo(chatId, pomoId, sessionChannel)
	})
	sessionChannel <- UserSession{
		chatId:    chatId,
		status:    pomoStarted,
		timer:     timer,
		messageId: timeMsgId,
		started:   time.Now(),
		pomoId:    pomoId,
	}
}

func endPomo(chatId int64, pomoId int64, sessionChannel chan<- UserSession) {
	markFinished(pomoId)

	sendKeyboard(chatId, "Pomodoro ended", pomoEnded)
	sessionChannel <- UserSession{chatId: chatId, status: pomoEnded, started: time.Now()}
}

func startBreak(chatId int64, sessionChannel chan<- UserSession) {
	timer := time.AfterFunc(breakTime, func() {
		endBreak(chatId, sessionChannel)
	})
	sendKeyboard(chatId, "Break started", breakStarted)
	timeMsgId := sendMessage(chatId, formatDuration(breakTime))
	sessionChannel <- UserSession{chatId: chatId, status: breakStarted, timer: timer, messageId: timeMsgId, started: time.Now()}
}

func endBreak(chatId int64, sessionChannel chan<- UserSession) {
	sendKeyboard(chatId, "Break ended", breakEnded)
	sessionChannel <- UserSession{chatId: chatId, status: breakEnded, started: time.Now()}
}

func sendRemainingTime(s UserSession, remaining time.Duration) {
	changed := tgbotapi.NewEditMessageText(s.chatId, s.messageId, formatDuration(remaining))
	bot.Send(changed)
}
