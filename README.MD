# l7dump

应用层抓包工具


## 原理

1. 通过 tcp syn 追踪连接
2. 根据头部监测应用层连接类型


## 特性

1. 支持 http, mysql, kafka, mogodb 等应用层协议
2. 支持插件 (lua script)
3. 支持高级过滤
