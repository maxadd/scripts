巡检脚本，依托于 golang 强大的并发能力，为每一个 ip 开启一个 goroutine（协程），每个 goroutine ssh 登录之后执行巡检的命令。

一旦巡检有异常，那么会将异常信息发送到一个 channel（队列）中，有一个 goroutine 会从这个 channel 中读数据，读完之后，将巡检结果汇总。详细的巡检结果写入 CSV 文件中一份，这个 CSV 会文件会发送到一个 http 服务器，以便通过钉钉消息的 URL 进行下载。总的简略的巡检结果（包括 CSV 文件的 URL）会发送到钉钉群机器人。

目前脚本还很粗糙，限制很死，只支持从 zabbix 的模板中读取 ip 列表。并且巡检项很少，一旦添加巡检项还要自己写代码实现巡检逻辑。每个配置文件中的巡检项对应 plugins 下面的一个插件。每写一个插件，需要在 `init` 函数中注册。

CSV 文件要发送的 http 服务器是写死的，没有可配置的地方，主要是没时间实现。
