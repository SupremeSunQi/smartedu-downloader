# 智教教材下载器

一个免安装的 Windows 桌面程序，用于浏览、筛选并下载国家中小学智慧教育平台的教材 PDF。本项目是非官方工具，与平台运营方无关联；请遵守平台使用规则和教材版权要求。

## 功能

- 按学段、学科、版本和年级浏览教材，查看封面与基础信息。
- 在 Chrome 或 Microsoft Edge 完成官网登录后，自动识别登录状态；不支持自动识别时可手动输入 Token。
- 单本教材下载，支持同时下载数量、同名文件处理和下载目录设置。
- 下载任务支持暂停、继续、取消、重试；异常退出后可恢复未完成任务。
- 教材目录缓存、任务记录、日志和下载文件集中保存在 EXE 所在目录，便于备份与清理。

## 运行环境

普通用户只需要 Windows 10/11 x64、网络连接和智慧教育平台账号。Windows 通常已包含 Microsoft Edge WebView2 Runtime；若启动时提示缺少，请从微软官方渠道安装。自动读取浏览器登录状态支持 Chrome 和 Microsoft Edge，其他浏览器可使用程序中的手动 Token 功能。**运行 EXE 不需要安装 Go、Node.js 或 Python。**

从[最新发布页](https://github.com/SupremeSunQi/smartedu-downloader/releases/latest)下载 `SmartEduDownloader.exe`，放入可写文件夹后双击运行，不建议放在 `Program Files`。首次启动若 Windows 显示安全提醒，请先核对下载来源。登录完成后返回程序，程序会自动检测；未进入教材列表时可点击“重新检测”。

## 从源码构建

开发与打包环境：Windows 10/11 x64、Go 1.25.x、Node.js 22.x（含 npm）、Wails CLI 2.10.2，以及 Wails 所需的 64 位 C 编译器。将 `go.exe`、`npm.cmd`、`wails.exe` 加入系统 `PATH`。完整的 Go 竞态检测脚本还需要 `clang.exe` 和 `clang++.exe`（例如 LLVM-MinGW）。

```powershell
# 安装与 go.mod 匹配的 Wails CLI；已经安装相同版本可跳过
go install github.com/wailsapp/wails/v2/cmd/wails@v2.10.2

# 在仓库根目录执行：安装前端依赖、运行测试、生成图标并打包
.\scripts\build.ps1

# 输出：build\bin\SmartEduDownloader.exe
```

单独运行完整测试与打包烟雾测试：

```powershell
npm --prefix frontend ci
.\scripts\test.ps1
.\scripts\smoke.ps1 -Executable .\build\bin\SmartEduDownloader.exe
```

`build.ps1` 会执行前端测试与构建、Go 测试与 `go vet`，再生成 Windows 图标并调用 Wails。`smoke.ps1` 会在临时目录验证单实例、正常退出和数据目录位置，结束后清理临时文件。

## 实现原理

- **界面与后端：** React + TypeScript 构建桌面界面，Wails 将界面和 Go 后端打包为单个 Windows EXE。`frontend/` 管理界面，`internal/` 按认证、目录、资源、下载、存储等职责划分。
- **登录：** 程序打开官方登录页，在 Chrome/Edge 的 LocalStorage 中只提取 HTTPS `smartedu.cn` 及其子域名下符合平台 SDK 格式的 Token 候选。浏览器存储被复制到有大小限制的内存快照；候选 Token 逐个请求受保护 PDF 的少量字节进行验证，通过后才保存在当前程序进程内。程序不读取浏览器密码，不保存 Token 到配置、缓存、任务记录或日志。浏览器 SDK 存储格式若发生变化，自动识别可能需要更新。
- **教材目录：** Go 后端同步平台目录，前端按所选条件查询；目录缓存放在程序数据目录中。刷新失败时可继续查看已有缓存。
- **下载：** 后端解析教材资源并从可用地址下载 PDF。任务写入 `.part` 临时文件，支持续传、镜像切换和完成后校验；退出时保存不含 Token 的任务进度。

默认可变文件放在 EXE 旁边的 `downloads/` 和 `.smartedu-data/` 中。设置里可更改下载目录；下载目录、并发数量和同名文件策略在重启程序后生效。程序不会安装服务或留下常驻后台进程。

## 安全与反馈

Token 等同于临时登录凭证，不要将它粘贴到 Issue、日志或截图中。提交问题时可使用设置中的“导出诊断信息”，并在分享前检查其中是否包含个人信息。
