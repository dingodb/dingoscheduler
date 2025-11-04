/*
* Copyright (c) 2022 FengTaiSEC Corporation.
* @brief
* @author     刘懿辉<liuyihui@fengtaisec.com>
* @date       2024/5/10 9:37:04
* @history
 */

package util

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

var (
	timeFormat   = "2006-01-02 15:04:05"
	timeLayout   = "2006/1/2 15:04:05"
	timeWithZone = "2006-01-02T15:04:05-07:00"

	timeTemplate1 = "2006-01-02 15:04:05"  // 常规类型
	timeTemplate2 = "2006/01/02 15:04:05"  // 其他类型
	timeTemplate3 = "2006 Jan 2 15:04:05"  // 其他类型
	timeTemplate7 = "Jan  2 2006 15:04:05" // 其他类型
	timeTemplate6 = "2006 Jane 2 15:04:05" // 其他类型

	timeTemplate4 = "15:04:05"
	timeTemplate5 = "2023-04-11T11:50:26.657Z"
	TimeParses    = make([]string, 0)
)

func init() {
	TimeParses = append(TimeParses,
		timeTemplate1,
		timeTemplate2,
		timeTemplate3,
		timeTemplate4,
		timeTemplate5,
		timeTemplate6,
		timeTemplate7)
}

const (
	day    = 24 * 60 * 60
	hour   = 60 * 60
	minute = 60
)

func GetCurrentTimeStr() string {
	now := time.Now()
	timeStr1 := now.Format("2006-01-02 15:04:05")
	return timeStr1
}

// FormatUptime 将运行时长（秒）转为 xH x分 x秒
func FormatUptime(uptime int64) string {
	days := uptime / day
	hours := (uptime - days*day) / hour
	minutes := (uptime - days*day - hours*hour) / minute
	seconds := uptime % minute
	return fmt.Sprintf("%d天 %02d时%02d分%02d秒", days, hours, minutes, seconds)
}

func ParseDuration(durationStr string) string {
	parts := strings.Split(durationStr, "h")
	if len(parts) != 2 {
		return "未知"
	}
	hours, err := strconv.Atoi(parts[0])
	if err != nil {
		return "未知"
	}

	parts = strings.Split(parts[1], "m")
	if len(parts) != 2 {
		return "未知"
	}
	minutes, err := strconv.Atoi(parts[0])
	if err != nil {
		return "未知"
	}
	parts = strings.Split(parts[1], "s")
	if len(parts) != 2 {
		return "未知"
	}
	parts = strings.Split(parts[0], ".")
	if len(parts) != 2 {
		return "未知"
	}
	seconds, err := strconv.Atoi(parts[0])
	if err != nil {
		return "未知"
	}
	days := hours / day
	hours = (hours - days*day) / hour
	return fmt.Sprintf("%d天 %02d时%02d分%02d秒", days, hours, minutes, seconds)

}

// TimestampUnixTLimitHourMinSecond  秒级时间戳转为字符串 只包含时分秒
func TimestampUnixTLimitHourMinSecond(timestamp int64) string {
	if timestamp == 0 {
		return ""
	}
	utcTime := time.Unix(timestamp, 0).UTC()
	// 创建一个表示东八区的时区对象
	zone := time.FixedZone("CST", 8*60*60) // 东八区，偏移量为8小时

	// 将时间对象转换为东八区时区
	easternTime := utcTime.In(zone)
	return easternTime.Format("15:04:05")
}

// 将时间转为字符串,只包含时分秒
func TimeTLimitHourMinSecond(t time.Time) string {
	// 创建一个表示东八区的时区对象
	zone := time.FixedZone("CST", 8*60*60) // 东八区，偏移量为8小时

	// 将时间对象转换为东八区时区
	easternTime := t.In(zone)
	return easternTime.Format(timeFormat)
}

// TimestampUnixTDateTime 秒级时间戳转为字符串 包含年月日时分秒
func TimestampUnixTDateTime(timestamp int64) string {
	if timestamp == 0 {
		return ""
	}
	utcTime := time.Unix(timestamp, 0).UTC()
	// 创建一个表示东八区的时区对象
	zone := time.FixedZone("CST", 8*60*60) // 东八区，偏移量为8小时

	// 将时间对象转换为东八区时区
	easternTime := utcTime.In(zone)
	return easternTime.Format(timeFormat)
}

// TimestampUnixUnixMilliTDateTime 毫秒级时间戳转为字符串 包含年月日时分秒
func TimestampUnixUnixMilliTDateTime(timestamp int64) string {
	if timestamp == 0 {
		return ""
	}
	utcTime := time.UnixMilli(timestamp).UTC()
	// 创建一个表示东八区的时区对象
	zone := time.FixedZone("CST", 8*60*60) // 东八区，偏移量为8小时

	// 将时间对象转换为东八区时区
	easternTime := utcTime.In(zone)
	return easternTime.Format(timeFormat)
}

func TimeFormat(time time.Time) string {
	if TimeToUnix(time) == 0 {
		return ""
	}
	return time.Format(timeFormat)
}

// 将时间字符串转为时间格式
func StringToTime(str string) *time.Time {
	// 解析日期时间字符串
	dt, err := time.Parse(timeFormat, str)
	if err != nil {
		str += ":00"
		dt, err = time.Parse(timeLayout, str)
		if err != nil {
			return nil
		}
	}
	return &dt
}

func TimeToUnix(time time.Time) int64 {
	if time.Unix() > 0 {
		return time.Unix()
	}
	return 0
}

func TimeAddrToUnix(time *time.Time) int64 {
	if time == nil {
		return 0
	}
	if time.Unix() > 0 {
		return time.Unix()
	}
	return 0
}

// TimestampTDateTime 毫秒级时间戳转为字符串
func TimestampTDateTime(timestamp int64) string {
	if timestamp == 0 {
		return ""
	}
	utcTime := time.UnixMilli(timestamp).UTC()
	// 创建一个表示东八区的时区对象
	zone := time.FixedZone("CST", 8*60*60) // 东八区，偏移量为8小时

	// 将时间对象转换为东八区时区
	easternTime := utcTime.In(zone)
	return easternTime.Format(timeFormat)
}

// 秒级时间戳转字符串

func UnixSecondTDateTime(second int64) string {
	if second == 0 {
		return ""
	}
	utcTime := time.Unix(second, 0).UTC()
	// 创建一个表示东八区的时区对象
	zone := time.FixedZone("CST", 8*60*60) // 东八区，偏移量为8小时

	// 将时间对象转换为东八区时区
	easternTime := utcTime.In(zone)
	return easternTime.Format(timeFormat)
}

// TimeStrWithZoneToString 带时区的时间字符串转为简洁的字符串
func TimeStrWithZoneToString(timeStr string) string {
	desiredLayout := "2006-01-02 15:04:05"
	t, err := time.Parse(timeWithZone, timeStr)
	if err != nil {
		fmt.Println("解析时间戳出错:", err)
		return ""
	}
	formattedTime := t.Format(desiredLayout)
	return formattedTime
}

func TimestampToDateTimeWithZone(timestamp int64) time.Time {
	location, _ := time.LoadLocation("Asia/Shanghai")

	t := time.Unix(timestamp, 0).In(location)
	return t
}

func TimeInterfaceToString(val interface{}) string {
	var t int64
	switch val.(type) {
	case int, int8, int16, int32, int64:
		t = val.(int64)
	case string:
		parseInt, err := strconv.Atoi(val.(string))
		if err != nil {
			t = TimeParse(val.(string))
		} else {
			t = int64(parseInt)
			if len(val.(string)) == 13 {
				t = t / 1000
			}
		}
	default:
		t = time.Now().Unix()
	}
	return TimestampUnixTDateTime(t)
}

func TimeParse(val string) int64 {
	// 遍历当前存储的日期时间
	for i := 0; i < len(TimeParses); i++ {
		ts, err := time.Parse(TimeParses[i], val)
		// 若出现错误 继续执行
		if err != nil {
			continue
		} else {
			// 若没有错误 表示找到匹配类型 返回
			unix := ts.Unix()
			return unix - 28800
		}
	}
	// if val == "" {
	//	return time.Now().Unix(), true
	// }
	// 标准timezone
	ts, err := time.Parse(time.RFC3339, val)
	// 若标准时间有错误 返回false
	if err != nil {
		fmt.Println("解析日期时间出错：", err)
		return 0
	} else {
		unix := ts.Unix()
		return unix
	}
}

func ZeroForTime() time.Time {
	now := time.Now()
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
}
