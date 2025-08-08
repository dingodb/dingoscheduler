package dao

import (
	"database/sql"
	"fmt"
	"log"
	"testing"

	"dingoscheduler/internal/model"

	_ "github.com/go-sql-driver/mysql" // 导入 MySQL 驱动（下划线表示仅执行 init 函数）
)

func TestDb(t *testing.T) {
	// 数据库连接字符串（格式：用户名:密码@tcp(地址:端口)/数据库名?参数）
	dsn := "root:123123@tcp(172.30.14.123:3307)/dingo?charset=utf8mb4&parseTime=true&loc=Local"

	// 打开数据库连接（不会立即建立连接，而是验证参数格式）
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		log.Fatalf("无法打开数据库连接: %v", err)
	}
	defer db.Close() // 程序退出时关闭连接

	// 验证连接是否有效
	if err := db.Ping(); err != nil {
		log.Fatalf("无法连接到数据库: %v", err)
	}
	fmt.Println("数据库连接成功！")
	if _, err := GetEntity(db, "hs", true); err != nil {
		log.Fatalf("db err,%v", err)
	}
}

// 查询单行数据
func GetEntity(db *sql.DB, instanceId string, online bool) (*model.Dingospeed, error) {

	// var speed2 model.Dingospeed
	// sql := fmt.Sprintf("select * from dingospeed where instance_id = '%s' and online = %v limit 1", instanceId, online)
	rows, err := db.Query("select id from dingospeed where instance_id = ? and online = ? limit 1", instanceId, online)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var speed model.Dingospeed
		// 扫描当前行数据到结构体
		if err := rows.Scan(&speed.ID); err != nil {
			return nil, err
		}
	}

	// if err := .Scan(&speed); err != nil {
	// 	log.Fatalf("db err,%v", err)
	// 	return nil, err
	// }
	return nil, nil
	// if err := d.baseData.BizDB.Raw(sql).Scan(&speed).Error; err != nil { // [mysql] 2025/07/30 11:18:38 packets.go:68 [warn] unexpected sequence nr: expected 1, got 2
	// 	if errors.Is(err, gorm.ErrRecordNotFound) {
	// 		return nil, nil
	// 	}
	// 	return nil, err
	// }
	// // 1个？=》 {"level":"ERROR","time":"2025-07-30 11:25:53","caller":"service/manager_service.go:62","msg":"getEntity err.Error 1105 (HY000): not a literal: ?1"}
	// // 没有？=>{"level":"ERROR","time":"2025-07-30 11:30:21","caller":"service/manager_service.go:62","msg":"getEntity err.Error 1105 (HY000): not a literal: ?0"}
	// if err := d.baseData.BizDB.Table("dingospeed").Where(fmt.Sprintf("instance_id = '%s'", instanceId)).First(&speed2).Error; err != nil {
	// 	if errors.Is(err, gorm.ErrRecordNotFound) {
	// 		return nil, nil
	// 	}
	// 	return nil, err
	// }
	// return &speed, nil
}
