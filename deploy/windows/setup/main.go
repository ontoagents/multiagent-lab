// MultiAgentLab SFX 自解压安装器（REQ-321）。
// 结构：setup.exe = 本程序 + 尾部附加 zip（build.ps1 拼接；zip 由 Compress-Archive 生成）。
// 行为：读自身字节 → 定位 EOCD 还原附加 zip → 解压到 %TEMP% 随机目录 → 隐终端执行 installer.cmd
// （真实安装逻辑全部在 installer.cmd：robocopy 落位/快捷方式/注册项/数据外移，见 deploy/windows/sfx/）。
// 诚实边界：解压窗口为控制台形态（进度按文件数计数）；MLAB_SILENT=1 透传给 installer.cmd 静默。
package main

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// CREATE_NO_WINDOW 不用于 setup 本体：控制台可见（双击有进度反馈）；installer.cmd 继承本控制台。
const createNoWindow = 0x08000000

func main() {
	exe, err := os.Executable()
	if err != nil {
		fail("无法定位自身: " + err.Error())
	}
	self, err := os.ReadFile(exe)
	if err != nil {
		fail("无法读取自身文件: " + err.Error())
	}
	zr, err := appendedZip(self)
	if err != nil {
		fail("定位附加 zip 失败（安装包不完整或被改动）: " + err.Error())
	}

	tmp, err := os.MkdirTemp("", "MultiAgentLab-setup-")
	if err != nil {
		fail("创建临时目录失败: " + err.Error())
	}
	defer os.RemoveAll(tmp)

	fmt.Printf("正在解压安装文件（%d 项）...\n", len(zr.File))
	for i, f := range zr.File {
		if err := extract(f, tmp); err != nil {
			fail(fmt.Sprintf("解压 %s 失败: %v", f.Name, err))
		}
		if i%50 == 0 {
			fmt.Printf("  %d/%d\n", i, len(zr.File))
		}
	}

	fmt.Println("启动安装程序...")
	cmd := exec.Command("cmd", "/d", "/c", "installer.cmd")
	cmd.Dir = tmp
	cmd.Env = os.Environ() // MLAB_SILENT 透传
	out, err := cmd.CombinedOutput()
	fmt.Print(strings.TrimRight(string(out), "\r\n"))
	if err != nil {
		fail(fmt.Sprintf("安装脚本失败: %v", err))
	}
}

// appendedZip 从自身字节尾部还原附加 zip：定位 EOCD（PK\x05\x06，无注释形态），
// 反推 archive 起点 = fileLen - 22 - cdSize - cdOffset。
func appendedZip(self []byte) (*zip.Reader, error) {
	const eocdSig = "PK\x05\x06"
	if len(self) < 22 {
		return nil, fmt.Errorf("文件过小")
	}
	eocd := -1
	floor := len(self) - 65536
	if floor < 0 {
		floor = 0
	}
	for i := len(self) - 22; i >= floor; i-- { // EOCD 在尾部 64KB 内
		if bytes.Equal(self[i:i+4], []byte(eocdSig)) {
			eocd = i
			break
		}
	}
	if eocd < 0 {
		return nil, fmt.Errorf("未找到 EOCD")
	}
	cdSize := binary.LittleEndian.Uint32(self[eocd+12 : eocd+16])
	cdOffset := binary.LittleEndian.Uint32(self[eocd+16 : eocd+20])
	start := len(self) - 22 - int(cdSize) - int(cdOffset)
	if start < 0 {
		return nil, fmt.Errorf("附加段越界")
	}
	return zip.NewReader(bytes.NewReader(self[start:]), int64(len(self)-start))
}

func extract(f *zip.File, dir string) error {
	// Compress-Archive（PS5.1）以反斜杠存条目名且目录条目不带 zip 规范的 / 结尾——
	// Go zip 据此误判为文件，会先创建出「payload\app」零字节文件使后续 mkdir 全线撞墙。
	// isDir 必须在 Clean 之前判定（Clean 会剥掉尾斜杠，反斜杠目录条目此时已无法识别）。
	name := strings.ReplaceAll(f.Name, "\\", "/")
	isDir := strings.HasSuffix(name, "/") || f.FileInfo().IsDir()
	name = filepath.Clean(name)
	if strings.HasPrefix(name, "..") || filepath.IsAbs(name) {
		return fmt.Errorf("非法路径 %s", f.Name)
	}
	target := filepath.Join(dir, name)
	if isDir {
		return os.MkdirAll(target, 0o755)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, rc)
	return err
}

func fail(msg string) {
	fmt.Fprintln(os.Stderr, "[setup] "+msg)
	fmt.Println("\n按回车键退出...")
	fmt.Scanln()
	os.Exit(1)
}
