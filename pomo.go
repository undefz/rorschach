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

	sessions := make(map[int64]*UserSession)

	go progressUpdater(sessions)
	go pomoReminder(sessions)

	readMessages(updates, sessions, config.AllowedChatId)
}

func progressUpdater(sessions map[int64]*UserSession) {
	for t := range time.Tick(1 * time.Second) {
		for _, v := range sessions {
			s := v.state
			switch s.status {
			case pomoStarted:
				remaining := pomoTime - t.Sub(s.started)
				sendRemainingTime(v, remaining)
			case breakStarted:
				remaining := breakTime - t.Sub(s.started)
				sendRemainingTime(v, remaining)
			}
		}
	}
}

func pomoReminder(sessions map[int64]*UserSession) {
	tickerPeriod := 1 * time.Second
	for t := range time.Tick(tickerPeriod) {
		for _, v := range sessions {
			s := v.state
			switch s.status {
			case pomoEnded:
				spent := t.Sub(s.started)
				if spent%reminderTime < tickerPeriod {
					sendMessage(v.user.chatId, "Time for break?")
				}
			case breakEnded:
				spent := t.Sub(s.started)
				if spent%reminderTime < tickerPeriod {
					sendMessage(v.user.chatId, "Time for pomo?")
				}
			default:
			}
		}
	}
}

func readMessages(updates <-chan tgbotapi.Update, sessions map[int64]*UserSession, allowedChatId int64) {
	for update := range updates {
		message := update.Message
		if message == nil || message.Chat.ID != allowedChatId {
			continue
		}

		chat := message.Chat

		from := message.From.UserName
		text := message.Text

		log.Printf("[%+v-%s] %s", chat.ID, from, text)

		session, ok := sessions[chat.ID]
		if !ok {
			session = &UserSession{
				user: User{
					chatId: chat.ID,
				},
			}
			sessions[chat.ID] = session
		}
		log.Printf("Session before action %s: %+v", text, session)
		switch text {
		case "/start":
			switch session.state.status {
			case empty:
				fallthrough
			case breakEnded:
				startPomo(session)
			case pomoStarted:
			//send error and how many time left
			case pomoEnded:
				startBreak(session)
			case breakStarted:
				//send error and how many time left
			}
		case "/stop":
			switch session.state.status {
			case pomoStarted:
				endPomo(session)
			case breakStarted:
				endBreak(session)
			default:
				resetState(session)
			}
		default:
			sendKeyboard(chat.ID, "Unknown command", session.state.status)
		}
		log.Printf("Session after action %s: %+v", text, session)
	}
}

func resetState(session *UserSession) {
	session.mutex.Lock()
	defer session.mutex.Unlock()

	session.state = State{
		status: empty,
	}
	sendKeyboard(session.user.chatId, "Timer reset", empty)
}

func startPomo(session *UserSession) {
	session.mutex.Lock()
	defer session.mutex.Unlock()

	chatId := session.user.chatId

	sendKeyboard(chatId, "Pomodoro started", pomoStarted)
	timeMsgId := sendMessage(chatId, formatDuration(pomoTime))

	pomoId := insertPomo(chatId)

	timer := time.AfterFunc(pomoTime, func() {
		endPomo(session)
	})
	session.state = State{
		status:    pomoStarted,
		timer:     timer,
		messageId: timeMsgId,
		started:   time.Now(),
		pomoId:    pomoId,
	}
}

func endPomo(session *UserSession) {
	session.mutex.Lock()
	defer session.mutex.Unlock()

	if session.state.timer != nil {
		session.state.timer.Stop()
	}

	markFinished(session.state.pomoId)

	sendKeyboard(session.user.chatId, "Pomodoro ended", pomoEnded)
	session.state = State{
		status:  pomoEnded,
		started: time.Now(),
	}
}

func startBreak(session *UserSession) {
	session.mutex.Lock()
	defer session.mutex.Unlock()

	chatId := session.user.chatId

	timer := time.AfterFunc(breakTime, func() {
		endBreak(session)
	})
	sendKeyboard(chatId, "Break started", breakStarted)
	timeMsgId := sendMessage(chatId, formatDuration(breakTime))
	session.state = State{
		status:    breakStarted,
		timer:     timer,
		messageId: timeMsgId,
		started:   time.Now(),
	}
}

func endBreak(session *UserSession) {
	session.mutex.Lock()
	defer session.mutex.Unlock()

	if session.state.timer != nil {
		session.state.timer.Stop()
	}

	sendKeyboard(session.user.chatId, "Break ended", breakEnded)
	session.state = State{
		status:  breakEnded,
		started: time.Now(),
	}
}

func sendRemainingTime(s *UserSession, remaining time.Duration) {
	changed := tgbotapi.NewEditMessageText(s.user.chatId, s.state.messageId, formatDuration(remaining))
	bot.Send(changed)
}
