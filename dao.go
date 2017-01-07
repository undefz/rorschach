package main

import (
	"github.com/jmoiron/sqlx"
	"log"
)

func insertPomo(chatId int64) int64 {
	result := db.MustExec("insert into pomo_history(user_id, task_id, started, ended, finished) "+
		"values (?, null, now(), null, 0)",
		chatId)
	inserted, err := result.LastInsertId()
	if err != nil {
		log.Println("Could not insert new pomo")
		return 0
	}
	return inserted
}

func markFinished(pomoId int64) {
	db.MustExec("update pomo_history set ended = now(), finished = 1 where id = ?", pomoId)
}

func loadTasks(db *sqlx.DB, userId int64) ([]Task, error) {
	tasks := []Task{}
	err := db.Select(tasks, "select * from tasks where user_id = ?", userId)
	return tasks, err
}

func addTask(db *sqlx.DB, userId int64, name string) (int64, error) {
	result, err := db.Exec("insert into tasks (user_id, name) values (?, ?)", userId, name)
	if err != nil {
		return 0, err
	}
	insertedId, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}
	return insertedId, nil
}

func deleteTask(db *sqlx.DB, taskId int64) error {
	_, err := db.Exec("delete from tasks where id = ?", taskId)
	return err
}
