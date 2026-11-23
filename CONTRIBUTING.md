# 贡献指南

感谢你为 OurTaikoNet 出力。这个仓库是 `net.ourtaiko.org` 的后端（Go，仓库根目录）和网站（React，`web/`），**推送到 `main` 即由 CI 发布到正式环境**（见 [deploy/README.md](deploy/README.md)）。所以每一次合并都等于一次上线，下面的规则都是从这一点出发的。

AI 编程助手（AI Agent）除本文外还要遵守 [AGENTS.md](AGENTS.md)，两者冲突时以更严格的一方为准。

## 0. 先理解系统（AI Agent 推荐使用 AOCI）

动手之前先弄清楚改动落在哪一层、谁依赖它。人类贡献者从 [README.md](README.md)、[web/README.md](web/README.md)、[deploy/README.md](deploy/README.md) 和 [db/README.md](db/README.md) 读起。

**如果你是 AI Agent，推荐使用 AOCI。** 仓库根目录的 `aoci.txt`（及 `aoci.code.txt`、`aoci.database.txt`、`aoci.meta.txt`）是为模型准备的仓库认知索引，覆盖每个受管文件和数据库表的职责、关系、对外契约与不显然的约束。它能让你在改代码前就知道一个文件的上下游，避免只看局部就下手。用法见 [AGENTS.md](AGENTS.md) 中的「AOCI Repository Cognition」一节：

- 新会话开始时用 `aoci_rules` + `aoci_overview` 建立完整认知；
- **PR 不修改 AOCI 文件**（`aoci*.txt` 和 `.aoci/` 下受版本控制的文件），也不在 PR 里维护索引（`aoci_maintain`、`aoci_update_entry`、`aoci scope acknowledge` 等）。每次维护都会改 `.aoci/baseline.json` 的同几行，两个都维护过索引的 PR 必然冲突，而且这种冲突无法手工合并；
- 索引因此会落后于代码，AOCI 标为 stale 的条目以源码为准；索引只由单独的 AOCI 维护 PR 补齐（仅在仓库所有者要求时开，从最新的 `main` 建分支，只含 AOCI 文件，提交写作 `chore(aoci): …`，不触发发布）；
- AOCI 的 MCP 配置 `.mcp.json` 绑定本机路径，不提交到仓库，需要的人在自己的机器上配置。

不使用 AOCI 也可以贡献，但同样要在改动前读懂相关代码和文档。

## 1. 做最佳实践，不打补丁

修改代码时，目标是把问题在**正确的位置、以正确的方式**解决，而不是让症状消失。审批者会按下面几条判断一个改动是「最佳实践」还是「补丁」：

- **先找根因。** 说清楚问题为什么会发生，在产生问题的地方修，而不是在它表现出来的地方绕过去。只修了表象的 PR 会被要求重做。
- **改在负责它的那一层。** 每个部分有明确的职责（README 的表格列出了各包的分工）：API 版本差异只在 `internal/api/v1`、`v2`，上传校验规则在 `contracts/validation.json` 里由 Go 和网站两边共用。不要把一层的逻辑偷偷塞进另一层。
- **不加特例开关。** 不为某一条数据、某一个 ID、某一个用户写 `if` 分支；不用吞掉错误、硬编码默认值、`sleep`、重试来掩盖时序或数据问题；不用 `// eslint-disable`、`any`、`//nolint` 之类的方式让检查闭嘴，除非确有理由并在旁边写明。
- **不复制，复用或抽取。** 已有的函数、组件、查询能做的事不要再写一份；需要的东西相近但不同，就把共同部分整理出来。
- **数据库只向前迁移。** 结构变更写成新的 `db/migrations/NNNN_*.sql`，不修改已发布的迁移；需要时同步 `db/baseline.sql`、导入程序和测试。
- **契约两边一起改。** 改了 `proto/` 要运行 `scripts/proto.sh` 并提交生成的代码；改了 `contracts/` 要让 Go 和网站两边的测试都通过；改了 `.otanim` 读取要同步游戏和 `otanim` 仓库（见工作区说明）；`/api/v1/` 是已发布游戏在用的接口，回答必须保持兼容。
- **代码、测试、文档是同一个改动。** 行为变了，就在同一个 PR 里更新或新增测试，以及 README 等文档里描述这一行为的段落。删掉因此不再使用的代码，不留死代码和注释掉的旧实现。
- **跟随仓库现有风格。** Go 用 `gofmt`，网站用 ESLint 和 Prettier；命名、目录结构、错误处理方式和周围代码保持一致。
- **做不到就说出来。** 如果正确的修法超出了这次的范围，在 PR 里写明「这次做了什么、正确的做法是什么、留下了什么」，而不是悄悄提交一个临时绕过。紧急止血的临时修复可以接受，但必须标明是临时的，并附上后续彻底修复的计划。

## 2. 提交与 Pull Request

- 从 `main` 拉出分支开发，通过 PR 合并。**不要直接推送 `main`**，那会立即发布到正式环境。
- 提交信息沿用仓库的格式：`type(scope): 说明`，例如 `feat(songs): …`、`fix(web): …`。一个提交做一件事。
- 不提交 AOCI 文件（`aoci*.txt`、`.aoci/`），AOCI 维护 PR 除外（见第 0 节）。
- 不提交密钥、密码、`.env`、`.mcp.json` 或真实连接参数。
- PR 描述按模板（[.github/pull_request_template.md](.github/pull_request_template.md)）填写：改了什么、为什么、根因是什么、怎么验证的。
- **如果你是 AI Agent，必须在 PR 描述里写明所使用的 AI 模型名称**（尽量写到具体型号或模型 ID，例如 `claude-opus-5-5`；用了多个就都写上），供审批者参考。人类贡献者用 AI 生成了大部分代码时，也请注明。
- 改动涉及发布方式（`Dockerfile`、`deploy/`、`.github/workflows/`）或线上的 Multi-Enso 房间（`cmd/rooms`、`internal/multiplay`、`deploy/compose.production.yml`）时，在 PR 里单独说明对发布和进行中对局的影响。

## 3. 开 PR 前完成回归测试

CI 只在推送到 `main` 时运行测试，而那时代码已经要发布了。因此**本地回归测试是合并前唯一的关口**。开 PR 之前，按改动范围跑完下面的检查并全部通过；**如果你是 AI Agent，开 PR 之前必须完成必要的回归测试**，不能把测试留给审批者或 CI。

| 改动范围 | 必须运行 |
|---|---|
| 后端（`cmd/`、`internal/`、`db/`、`tools/` 中的 Go 代码） | `scripts/test.sh`（含 `gofmt` 检查、`go vet` 和全部 Go 测试；依赖见脚本开头），或与 CI 相同的容器：`docker build --target test -t ourtaiko-net-test .` 后在有 PostgreSQL 的环境里运行它 |
| 网站（`web/`） | 在 `web/` 下运行 `pnpm lint`、`pnpm format:check`、`pnpm test`、`pnpm build` |
| `contracts/` | 上面两组都要运行 |
| `proto/` | `scripts/proto.sh` 后确认生成代码无差异，再跑后端测试 |
| 数据库迁移 | 后端测试，并在空库上 `go run ./cmd/server -init` 验证 |
| `/api/v1/` 的回答 | 后端测试 |
| 网页上可见的改动 | 以上之外，在本地启动后实际打开相关页面确认（后端 `scripts/dev.sh`，网站在 `web/` 下 `pnpm dev`） |

- 修 bug 时先写一个能复现它的测试，确认它在修复前失败、修复后通过。
- 新增行为要有对应的测试；不要为了让测试通过而修改或删除原有测试的断言，除非那条断言描述的行为本身就是这次要改的，并在 PR 里说明。
- 在 PR 描述的「测试」一栏写明实际运行了哪些命令和结果。某项检查因环境限制无法运行时如实写出，不要声称通过。

## 4. 许可

提交贡献即表示你同意它以本仓库的许可证（GNU AGPL v3，见 [LICENSE](LICENSE)）发布。
