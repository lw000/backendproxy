# 代理转发服务

基于 Go 实现的多端口代理转发服务，支持运行监控和日志切割。

## 功能特性

- **多端口监听**：可同时监听多个端口，每个端口转发到不同的目标服务
- **请求转发**：基于 `net/http/httputil.ReverseProxy` 实现高效转发
- **日志切割**：支持按大小切割日志，仅保留最近 N 个文件
- **运行监控**：基于 Gin 的监控页面，实时展示服务状态和请求/响应数据
- **服务标签**：每个代理服务可配置说明标签，便于识别

## 项目结构

```
backendproxy/
├── main.go              # 程序入口
├── config/
│   └── config.go        # 配置解析
├── proxy/
│   └── proxy.go         # 代理逻辑
├── logger/
│   └── logger.go        # 日志日志初始化
├── service/
│   └── service.go       # 服务封装
├── monitor/
│   └── handler.go       # Gin 监控接口
├── static/
│   └── index.html       # 监控页面
├── config.toml          # 配置文件
├── build.bat            # 编译脚本
├── release.bat          # 发布脚本
├── go.mod               # Go 模块
└── README.md            # 说明文档
```

## 配置说明

### config.toml

```toml
# 代理服务配置
[[proxies]]
port = 8080              # 监听端口
label = "用户服务"        # 服务说明
target = "http://localhost:3000"  # 目标地址

[[proxies]]
port = 8081
label = "订单服务"
target = "http://localhost:3001"

# 日志配置
[log]
dir = "./logs"          # 日志存放目录
level = "info"          # 日志级别
maxSize = 100           # 单文件最大大小(MB)
maxBackups = 100        # 保留文件数量

# 监控配置
[monitor]
enabled = true          # 是否启用监控
port = 9090             # 监控服务端口
```

## 编译运行

### 方式一：使用脚本（推荐）

#### 编译

```bash
.\build.bat
```

编译脚本会自动完成：
1. 检查 Go 环境
2. 清理旧的编译文件
3. 下载依赖
4. 编译程序
5. 创建日志目录

#### 发布

```bash
.\release.bat
```

发布脚本会自动完成：
1. 检查可执行文件
2. 生成时间戳目录
3. 拷贝程序、配置、静态文件到 `prod/backendproxy_YYYYMMDD-HHMMSS/`

### 方式二：手动编译

```bash
go build -o bin/backendproxy.exe main.go
```

### 运行

```bash
# 使用默认配置文件 config.toml
.\bin\backendproxy.exe

# 指定配置文件
.\bin\backendproxy.exe -config=path/to/config.toml
```

## 发布管理

每次发布会在 `prod` 目录下创建带时间戳的新版本：

```
prod/
├── backendproxy_20250317-143025/
│   ├── backendproxy.exe
│   ├── config.toml
│   └── static/
│       └── index.html
└── backendproxy_20250317-150030/
    ├── backendproxy.exe
    ├── config.toml
    └── static/
        └── index.html
```

## 监控页面

启动服务后，访问 `http://localhost:9090` 查看监控面板：

- **服务概览**：显示所有代理服务的运行状态
- **统计信息**：总请求数、成功数、失败数、平均延迟
- **实时日志**：显示最近的请求/响应数据流

## 依赖

- github.com/BurntSushi/toml - 配置解析
- github.com/gin-gonic/gin - 监控服务
- go.uber.org/zap - 结构化日志
- gopkg.in/natefinch/lumberjack.v2 - 日志切割

## 快速开始

1. **编译程序**
   ```bash
   .\build.bat
   ```

2. **修改配置**
   编辑 `config.toml`，设置代理服务、日志和监控配置

3. **运行程序**
   ```bash
   .\bin\backendproxy.exe
   ```

4. **访问监控**
   打开浏览器访问 `http://localhost:9090`

5. **发布版本**
   ```bash
   .\release.bat
   ```
