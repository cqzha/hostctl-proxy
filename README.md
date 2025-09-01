# hostctl_proxy

hostctl_proxy是基于[atx-agent](https://github.com/openatx/atx-agent)魔改
对原来的Http代理模块做了改造，核心控制模块cmdctrl只做了小幅修改
为适应笔者的项目，Http代理新增了动态路由，重写了request处理逻辑；新增了Config模块，实现控制对象可配置

**本项目旨在将测试环境的底层控制功能集成到统一平台，并通过标准化的 API 供各类测试脚本调用**
※需要配合Http客户端及具体的控制对象

```
+-------------+             +---------------+                 +---------------+
|             |             |               |                 |               |
|             |             |               |    控制/        |   target      |
| Http Client |<---http---->| hostctl_proxy |<---socket通信-->|  DUT/         |
|             |             |               |                 |  application/ |
|             |             |               |                 |  device etc   |
+-------------+             +---------------+                 +---------------+
                           \________________测试环境____________________________/
```

## Features

*   **HTTP API 驱动**：通过标准化 HTTP 接口暴露功能，各类脚本（Python/Shell等）可轻松集成，**消除底层控制的重复编码**。
*   **插件化架构**：控制对象（设备/模块）以插件形式管理，**增删配置化**，实现快速部署与灵活扩展。

## Quick Start

*   **下载代码及依赖**
```bash
git clone https://github.com/cqzha/hostctl_proxy.git
cd hostctl_proxy
go tidy mod
```

*   **Build**
```bash
go build -o bin/hostctl_proxy.exe # for windows
go build -o bin/hostctl_proxy # for linux
```

*   **Run**
※ 提前准备配置文件config.json，与执行文件同一路径
```bash
./hostctl_proxy.exe server # for windows
./hostctl_proxy server # for linux
```
