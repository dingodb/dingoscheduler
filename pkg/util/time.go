package util

import (
	"time"
)

func GetCurrentTimeStr() string {
	now := time.Now()
	timeStr1 := now.Format("2006-01-02 15:04:05")
	return timeStr1
}
