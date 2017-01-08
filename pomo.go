package main

import (
	"log"
	"time"

	"gopkg.in/telegram-bot-api.v4"

	_ "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"os"
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

// using env variables because user.Current() doesn't work with cross-compilation
func homeDir() string {
	if home := os.Getenv("HOME"); home != "" {
		return home
	}
	// For Windows.
	return os.Getenv("UserProfile")
}

func loadConfig() (Config, error) {
	file, err := ioutil.ReadFile(homeDir() + "/.config/rorschach.yaml")
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
	for t := range time.Tick(10 * time.Second) {
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

func sendRemainingTime(s *UserSession, remaining time.Duration) {
	changed := tgbotapi.NewEditMessageText(s.user.chatId, s.chatState.counterId, formatDuration(remaining))
	bot.Send(changed)
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

		chatId := message.Chat.ID

		from := message.From.UserName
		text := message.Text

		log.Printf("[%+v-%s] %s", chatId, from, text)

		session, ok := sessions[chatId]
		if !ok {
			session = &UserSession{
				user: User{
					chatId: chatId,
				},
			}
			sessions[chatId] = session
		}
		log.Printf("Session before action %s: %+v", text, session)
		switch session.chatState.status {
		case Idle:
			switch text {
			case "/start":
				switch session.state.status {
				case empty:
					fallthrough
				case breakEnded:
					startPomo(session)
				case pomoEnded:
					startBreak(session)
				}
			case "/stop":
				resetState(session)
			case "/tasks add":
				sendMessage(chatId, "Enter task name")
				session.chatState = ChatState{status: AddingTask}
			case "/tasks set":
				listTasks(session)
				sendMessage(chatId, "Enter task name")
				session.chatState = ChatState{status: SelectingTask}
			case "/tasks delete":
				listTasks(session)
				sendMessage(chatId, "Enter task name")
				session.chatState = ChatState{status: DeletingTask}
			default:
				sendKeyboard(chatId, "Unknown command", session.chatState.status)
			}
		case Counter:
			switch text {
			case "/stop":
				switch session.state.status {
				case pomoStarted:
					endPomo(session)
				case breakStarted:
					endBreak(session)
				default:
					sendKeyboard(chatId, "Unknown command", session.chatState.status)
				}
			}
		case AddingTask:
			addTask(session, text)
		case DeletingTask:
			deleteTask(session, text)
		case SelectingTask:
			setTask(session, text)
		default:
			sendKeyboard(chatId, "Unknown command", session.chatState.status)
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
	session.chatState = ChatState{}
	sendKeyboard(session.user.chatId, "Timer reset", Idle)
}

func startPomo(session *UserSession) {
	session.mutex.Lock()
	defer session.mutex.Unlock()

	chatId := session.user.chatId

	sendKeyboard(chatId, "Pomodoro started", Counter)
	counterId := sendMessage(chatId, formatDuration(pomoTime))

	pomoId := insertPomo(chatId, session.user.taskId)

	timer := time.AfterFunc(pomoTime, func() {
		endPomo(session)
	})
	session.state = State{
		status:  pomoStarted,
		timer:   timer,
		started: time.Now(),
		pomoId:  pomoId,
	}
	session.chatState = ChatState{
		status:    Counter,
		counterId: counterId,
	}
}

func endPomo(session *UserSession) {
	session.mutex.Lock()
	defer session.mutex.Unlock()

	if session.state.timer != nil {
		session.state.timer.Stop()
	}

	markFinished(session.state.pomoId)

	sendKeyboard(session.user.chatId, "Pomodoro ended", Idle)
	session.state = State{
		status:  pomoEnded,
		started: time.Now(),
	}
	session.chatState = ChatState{
		status: Idle,
	}
}

func startBreak(session *UserSession) {
	session.mutex.Lock()
	defer session.mutex.Unlock()

	chatId := session.user.chatId

	timer := time.AfterFunc(breakTime, func() {
		endBreak(session)
	})
	sendKeyboard(chatId, "Break started", Counter)
	counterId := sendMessage(chatId, formatDuration(breakTime))
	session.state = State{
		status:  breakStarted,
		timer:   timer,
		started: time.Now(),
	}
	session.chatState = ChatState{
		status:    Counter,
		counterId: counterId,
	}
}

func endBreak(session *UserSession) {
	session.mutex.Lock()
	defer session.mutex.Unlock()

	if session.state.timer != nil {
		session.state.timer.Stop()
	}

	sendKeyboard(session.user.chatId, "Break ended", Idle)
	session.state = State{
		status:  breakEnded,
		started: time.Now(),
	}
	session.chatState = ChatState{
		status: Idle,
	}
}

func listTasks(session *UserSession) error {
	tasks, err := loadTasks(db, session.user.chatId)
	if err != nil {
		log.Printf("Error while loading tasks: %s\n", err)
		return err
	}

	var rows [][]tgbotapi.KeyboardButton
	for _, task := range tasks {
		rows = append(rows, []tgbotapi.KeyboardButton{
			tgbotapi.NewKeyboardButton(task.Name),
		})
	}

	msg := tgbotapi.NewMessage(session.user.chatId, "Select task")
	keyboard := tgbotapi.NewReplyKeyboard(rows...)
	keyboard.OneTimeKeyboard = true
	msg.ReplyMarkup = keyboard
	bot.Send(msg)
	return nil
}

func addTask(session *UserSession, taskName string) error {
	session.mutex.Lock()
	defer session.mutex.Unlock()

	_, err := insertTask(db, session.user.chatId, taskName)
	if err != nil {
		return err
	}

	session.chatState = ChatState{}
	sendKeyboard(session.user.chatId, "Task inserted", Idle)
	return nil
}

func deleteTask(session *UserSession, taskName string) error {
	session.mutex.Lock()
	defer session.mutex.Unlock()

	err := deleteTaskByName(db, session.user.chatId, taskName)
	if err != nil {
		log.Printf("Error while deleting task %s - %s", taskName, err)
		return err
	}
	session.chatState = ChatState{}
	sendKeyboard(session.user.chatId, "Task "+taskName+" deleted", Idle)
	return nil
}

func setTask(session *UserSession, taskName string) error {
	session.mutex.Lock()
	defer session.mutex.Unlock()

	session.chatState = ChatState{}

	tasks, err := loadTasks(db, session.user.chatId)
	if err != nil {
		log.Printf("Error while loading tasks: %s\n", err)
	}
	for _, task := range tasks {
		if task.Name == taskName {
			session.user.taskId = task.Id
			sendKeyboard(session.user.chatId, "Changed task to "+taskName, Idle)
			return nil
		}
	}
	sendKeyboard(session.user.chatId, "Task not found!", Idle)
	return nil
}
