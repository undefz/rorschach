package main

import "log"

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
