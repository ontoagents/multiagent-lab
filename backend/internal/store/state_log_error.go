package store

import "log"

// logStateWriteErr 留痕写入失败仅日志（审计旁路不阻断业务写）；独立函数便于测试注入拦截。
func logStateWriteErr(field string, err error) {
	log.Printf("[state_log] %s 留痕写入失败: %v", field, err)
}
