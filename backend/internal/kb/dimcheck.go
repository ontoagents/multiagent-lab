package kb

// KB-20/M292：embedding 维度漂移检测与告警——错误类型。
// 根治：embedding 连接切换后新旧维度向量混存、SQLite 路径 len(v)!=len(vec) 静默跳过丢召回；
// 改为库级 embed_dim 登记 + 写入端拒绝（DimMismatchError）+ 检索端可懂报错（含重建指引）。
// KB-O3（库级维度锁定）工程侧随本件落地：登记即锁定，解锁=整库重建（清空后首索引重登记）。

import "fmt"

// DimMismatchError 维度漂移错误（写入/检索双端；消息含重建指引——可懂报错不虚构结果）。
type DimMismatchError struct {
	KB   string
	Want int
	Have int
}

func (e DimMismatchError) Error() string {
	return fmt.Sprintf(
		"知识库 %q 已登记 embedding 维度 %d，当前模型输出 %d 维——embedding 模型已切换。请整库重建索引（删除文档后重新灌入，首索引将以新维度重新登记），或切回原 embedding 模型。",
		e.KB, e.Want, e.Have)
}
