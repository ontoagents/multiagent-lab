// OntoAgents Windows 桌面客户端入口（逻辑在 lib.rs，便于后续多平台复用）
#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

fn main() {
    ontoagents_desktop::run()
}
