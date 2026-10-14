//go:build windows

package inference

import "os"

// quitSignal 关闭阶梯第二档信号（Windows 无 SIGTERM）：返回 Interrupt 以满足签名；
// Windows 进程不支持 interrupt 投递，Signal 调用报错由调用方忽略（streaming.go `_ =`），
// 第三档 Kill 兜底——与 signal_unix.go 的阶梯语义对齐，Windows 下第二档为空操作。
func quitSignal() os.Signal { return os.Interrupt }
