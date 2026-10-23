package main

// REQ-383/M179：默认回环监听解析四象限（显式 ADDR 恒尊重 > LISTEN_ALL opt-in > 默认 127.0.0.1）。
import "testing"

func TestResolveListenAddr(t *testing.T) {
	cases := []struct {
		addrEnv, listenAll, want string
	}{
		{"", "", "127.0.0.1:8090"},           // 默认回环
		{"", "1", ":8090"},                   // opt-in 全接口
		{"", "true", ":8090"},                // opt-in true
		{":8090", "", ":8090"},               // 显式 ADDR 恒尊重（容器编排口）
		{"0.0.0.0:8090", "", "0.0.0.0:8090"}, // 显式非回环也尊重（运维自担，启动有告警日志）
	}
	for _, c := range cases {
		if got := resolveListenAddr(c.addrEnv, c.listenAll, "8090"); got != c.want {
			t.Errorf("resolveListenAddr(%q,%q) = %q, want %q", c.addrEnv, c.listenAll, got, c.want)
		}
	}
}
