# pigo 如何对应 Pi

[English](design.md) · [界面](tui.zh-CN.md) · [说明](../README.zh-CN.md)

pigo 是一个用来阅读 [Pi](https://github.com/earendil-works/pi) 核心想法的小型 Go 程序。它保留同样的三层，不包含技能市场、扩展和完整 TUI。

## 三层

```mermaid
flowchart TD
    cli["internal/cli<br/>终端会话"]
    config["internal/config"]
    coding["internal/coding<br/>pi-coding-agent"]
    skills["internal/skills"]
    agent["internal/agent<br/>pi-agent-core"]
    ai["internal/ai<br/>pi-ai"]
    http["OpenAI<br/>HTTP"]
    cli --> config
    cli --> coding
    cli --> skills
    cli --> agent
    cli --> ai
    coding --> skills
    coding --> agent
    coding --> ai
    agent --> ai
    ai --> http
```

| 层 | 可以知道 | 不应知道 |
| --- | --- | --- |
| `internal/ai` | 消息、工具 schema、流式输出、HTTP | 执行工具、文件、循环 |
| `internal/agent` | 会话记录、询问再行动的循环、调用 `Tool.Execute` | HTTP、工作区、技能 |
| `internal/coding` | 工作区、文件和 shell 工具、系统提示 | SSE 如何解析 |
| `internal/skills` | `SKILL.md` 在哪里、提示词里的简短列表 | 循环和 HTTP |
| `internal/config` | 命令行参数、环境变量、`.env`、YAML | 循环、HTTP 和画面 |
| `internal/cli` | Cobra、Bubble Tea、`/skill:name`、调用 `config.Load` | 路径检查和线路格式 |

技能不会变成一种新的代理。编码会话发现技能后，要么把它们写进系统提示，要么在 `/skill:name` 时把文件贴进一条普通用户消息。`Agent.Prompt` 看到的仍然只是消息和工具。

## 会话记录和线路格式

会话记录是循环的记忆。一条消息有角色和文本；助手消息还可以带一组工具调用。工具结果是单独的角色 `toolResult`，并带上它回答的那次调用的 ID。

OpenAI chat completions 的形状不同：工具调用嵌在 `function` 里，工具结果的角色是 `tool`。`wire.go` 为每一层 JSON 对象起一个具名类型。`encodeRequest` 写出请求，累加器读入每个 `wireChunk`，两边都用 `json.Marshal` 和 `json.Unmarshal`。程序其余部分保持更平坦的会话记录。

系统提示在构造请求时附上。它不写入会话记录，所以 `Reset` 可以丢掉消息，同时留下提示、模型和工具。

## 代理循环

```mermaid
sequenceDiagram
    participant User as 用户
    participant Agent as 代理
    participant Model as 模型
    participant Tools as 工具
    User->>Agent: 提示
    loop 每圈一次模型调用
        Agent->>Model: 提示和工具
        Model-->>Agent: 消息
        alt 工具调用
            Agent->>Tools: 一次调用
            Tools-->>Agent: 结果
        end
    end
    Agent-->>User: 最终文本
```

`run` 的一圈就是一次模型调用。文本增量给界面看。`done` 事件才是写入会话记录的助手消息。

如果这条消息带有工具调用，每次调用都会执行，并配上一条复用该调用 ID 的 `toolResult`。下一圈会把两者一起发出去。被取消或未知的调用也会得到一条结果。助手消息里若有未配对的工具调用，下一次请求就会无效。

轮数上限是 25 次模型调用。

## 危险命令

代理核心不判断命令安不安全。模型要调用工具之后、`Execute` 之前，会先走 `BeforeToolCall`。钩子可以放行、拦截，或换掉参数。被拦截的调用仍然得到一条工具结果，这一轮继续，模型可以改做法。

```mermaid
sequenceDiagram
    participant Agent as 代理
    participant Hook as 钩子
    participant Tools as 工具
    participant Model as 模型
    Agent->>Model: 会话记录
    Model-->>Agent: 工具调用
    Agent->>Hook: BeforeToolCall
    alt 拦截
        Hook-->>Agent: 原因
        Agent-->>Model: 已拦截
    else 放行
        Hook-->>Agent: 放行
        Agent->>Tools: 执行
        Tools-->>Agent: 结果
        Agent-->>Model: 工具结果
    end
```

只有 `internal/coding` 列出危险的 shell 命令。它用一张短拒绝表匹配 `run_command`。选项可以写在路径后面（如 `rm ~ -rf`）；`--` 之后的词一律当操作数：

- 对 `/`、`~` 或 `$HOME` 执行 `rm -rf`
- `sudo`
- `mkfs`
- `dd` 写到磁盘设备（`of=/dev/sd*`，以及 `nvme`、`hd`、`mmcblk`、`xvd`、`vd`）
- `git push --force`，包括 `-f`
- `curl` 或 `wget` 通过管道送进 shell
- fork bomb
- `chmod -R 777 /`
- 重定向到这些磁盘设备

`rm -rf ./build` 和 `git push --force-with-lease` 不在表里。

交互聊天里，命中后工作行换成确认提示：

```text
! allow this command?
rm -rf /
recursive delete of / or home
enter allow · esc deny
```

Enter 或 `y` 执行。Esc 或 `n` 拒绝。工具结果是 `blocked:` 加上原因，这一轮继续。Ctrl-C 仍然中止这一轮。单次模式（`-p`）和行会话没有提示，所以直接拒绝。这两种模式在 Ctrl+C 时取消这一轮，而不是让信号直接杀掉进程。`--yolo`、配置文件里的 `yolo: true`，或 `PIGO_YOLO=1`，会跳过检查。聊天画面和行会话共用一张命令表：`/exit`、`exit`、`quit`、`/quit` 和 `/reset`。

这是一张模式拒绝表。脚本、编码、别名、`git -C`、`(rm -rf /)` 或 `$(…)` 这样的子 shell，或 `env rm` 这样的包装，都很容易绕过。它防的是模型失手，不是对手。真正的隔离需要沙箱：容器、macOS seatbelt，或 Linux landlock。

## 测试

`go test ./...` 跑单元测试和 `e2e` 里的本地端到端测试。这些测试启动一个 `httptest` 服务，按 OpenAI 兼容格式流出事件（含工具调用），并驱动 `coding.Session`、`pigo -p` 和 Bubble Tea 模型。`go test -tags e2e ./e2e/live`（`make e2e`）对真实模型做同类任务；没有 `OPENAI_API_KEY` 或 `OPENAI_BASE_URL` 时跳过。CI 在每次 push 和 pull request 上跑本地套件。真实模型任务在 `main`、手动触发，以及本仓库自己的 pull request 上跑；仓库密钥缺失时跳过。

## 配置

只有 `internal/config` 读取进程设置。koanf 把它们合并成一个扁平结构。`internal/agent`、`internal/ai`、`internal/coding` 都不引用它。`.env` 和技能前言各自去掉一对引号。`.env` 只填补进程里还没有的变量，给 shell 工具用，不会覆盖已经设置的变量。进程环境在 `.env` 之后再写入，所以进程里的值仍然胜过 `.env`。

从低到高的优先级：默认值、`~/.pigo/config.yaml`、`./pigo.yaml`、`.env`、环境变量（`PIGO_PROVIDER`、`OPENAI_API_KEY`、`OPENAI_BASE_URL`、`OPENAI_MODEL`、`PIGO_YOLO`），然后是命令行参数。空值会被跳过。内置模型是 `gpt-5.6-luna`。命令行用 Cobra：`pigo` 打开聊天，`-p` / `--print` 只跑一条提示，`--yolo` 跳过危险命令检查。

一次聊天请求只发送 `model`、`messages`、`tools` 和 `stream`。它不发送 `max_tokens`、`temperature` 或 `top_p`。GPT-5 推理模型会拒绝这些字段；不发送它们，其他兼容 OpenAI 的提供者也继续用自己的默认值。

聊天画面见 [tui.zh-CN.md](tui.zh-CN.md)。

## 工具和工作区

`Workspace.Resolve` 把路径从工作区根目录变成绝对路径，跟随符号链接，并拒绝根目录之外的目标。检查用的是解析之后的路径，所以项目内部的链接不能指向外面的文件。

`read_file` 多一条许可。工作区检查失败后，它仍可以打开已发现技能目录中的文件。`~/.pi/agent/skills` 里的技能就是这样被读到的。`write_file` 和 `edit_file` 没有这条许可。

`run_command` 以工作区为当前目录启动 `bash`。Unix 上需要安装 bash。它是普通进程。环境里的 `OPENAI_API_KEY` 会被去掉。[危险命令](#危险命令) 里的拒绝表可以在进程启动前拦住这条命令。

## 技能

```mermaid
flowchart TD
    start["开始"]
    userDir["用户技能<br/>~/.pi/agent/skills"]
    projectDir["项目技能<br/>.pi/skills"]
    list["同名<br/>留首"]
    prompt["系统<br/>提示"]
    read["read_file"]
    command["/skill:name"]
    start --> userDir --> projectDir --> list --> prompt
    prompt --> read
    prompt --> command
```

发现规则按教学所需的规模跟随 Pi：

- 先扫描用户技能，再扫描项目技能。同名时保留第一个，所以用户技能覆盖项目技能。
- 含有 `SKILL.md` 的目录是技能根，不再扫描子目录。
- 其他目录会递归扫描。以 `.` 开头的名字和 `node_modules` 会被跳过。
- 指向技能根之外的目录符号链接不会被继续遍历。如果该目录自身含有 `SKILL.md`，它仍然是一个技能。扫描会记住解析后的路径，所以符号链接环会停下来。指向本目录之外文件的 `SKILL.md` 会被跳过。
- 直接放在技能根目录、并且带有前置信息的 markdown 文件也算一个技能。
- 前置信息是夹在 `---` 之间的 `key: value`。描述为空则丢弃该文件。超过 1024 个字符的描述会被截到这个长度。名字为空则用目录名。名字由小写字母、数字和单个连字符组成，最长 64 个字符。
- `disable-model-invocation: true` 会把技能从提示词中去掉。`/skill:name` 仍然可以运行它。

提示词要求模型对技能路径调用 `read_file`。文件正文不会复制进提示词。技能里提到的相对路径要相对技能目录解析，工具收到的应是那个绝对路径。

`/skill:name` 以及可选的请求由 CLI 在 `Prompt` 之前处理。它读出 `SKILL.md`，发出一条已经包含文件和请求的用户消息。循环本身不增加技能命令。

Pi 还会遵守忽略文件、上层目录的 `.agents/skills`，以及包里的技能。pigo 不做这些，这样整段扫描可以一次读完。

## 留出的部分

完整的 Pi TUI、各家厂商的原生 API、技能市场、会话文件、子代理，以及遥测。这棵目录要说明的是分层：传输、循环、编码产品。
